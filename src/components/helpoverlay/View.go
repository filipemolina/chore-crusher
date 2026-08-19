package helpoverlay

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/components/chrome"
	"github.com/filipemolina/farol/src/keys"
	"github.com/sahilm/fuzzy"
)

// contentWidth is the column the overlay's hints wrap to: the terminal minus
// the modal chrome and a margin, capped.
func (m Model) contentWidth() int {
	return max(24, min(helpOverlayMaxWidth, m.termWidth-16))
}

// renderScope renders one scope as a title line over one line per entry —
// each key/description pair gets its own row rather than being packed
// side-by-side and wrapped, which is what made a scope with several keys
// read as a run-on paragraph instead of a list. Unavailable rows are dimmed
// whole; available rows get the footer's treatment (key bold, description
// muted). Keys are padded to the widest key in the scope so the descriptions
// line up in a column.
func renderScope(scope keys.Scope, width int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(appstyles.Active.Accent)
	dimStyle := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextDim)
	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(appstyles.Active.TextPrimary)
	descStyle := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextMuted)

	keyWidth := 0
	for _, entry := range scope.Entries {
		keyWidth = max(keyWidth, lipgloss.Width(entry.Binding.Help().Key))
	}

	lines := []string{titleStyle.Render(scope.Title)}
	for _, entry := range scope.Entries {
		help := entry.Binding.Help()
		key := lipgloss.NewStyle().Width(keyWidth).Render(help.Key)

		var row string
		if entry.Available {
			row = keyStyle.Render(key) + descStyle.Render("  "+help.Desc)
		} else {
			row = dimStyle.Render(key + "  " + help.Desc)
		}
		lines = append(lines, lipgloss.NewStyle().Width(width).Render(row))
	}

	if scope.Note != "" {
		// Behaviour a key/description pair cannot carry — see keys.Scope.Note.
		lines = append(lines, dimStyle.Width(width).Render(scope.Note))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// filteredScope narrows scope to the entries whose key or description
// fuzzy-matches query, in their original order. ok is false when nothing in
// the scope matched, so the caller can drop the scope (and its title)
// entirely rather than show a heading over nothing — the same "no orphaned
// section" rule the task tree's own filter follows.
func filteredScope(scope keys.Scope, query string) (result keys.Scope, ok bool) {
	if query == "" {
		return scope, true
	}

	texts := make([]string, len(scope.Entries))
	for i, entry := range scope.Entries {
		help := entry.Binding.Help()
		texts[i] = help.Key + " " + help.Desc
	}

	matches := fuzzy.Find(query, texts)
	if len(matches) == 0 {
		return keys.Scope{}, false
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Index < matches[j].Index })

	original := scope.Entries
	scope.Entries = make([]keys.Entry, len(matches))
	for i, match := range matches {
		scope.Entries[i] = original[match.Index]
	}
	return scope, true
}

// filteredCatalog is the catalog narrowed by the current filter query, scope
// by scope. An empty query returns the catalog unchanged.
func (m Model) filteredCatalog() []keys.Scope {
	query := strings.TrimSpace(m.filterQuery)
	out := make([]keys.Scope, 0, len(m.catalog))
	for _, scope := range m.catalog {
		if filtered, ok := filteredScope(scope, query); ok {
			out = append(out, filtered)
		}
	}
	return out
}

// contentLines renders every scope in the filtered catalog and flattens the
// result to one list of already-wrapped lines, with a blank line between
// scopes — the section separation the catalog has always had, preserved
// across filtering. Flattening before windowing is what lets a scope taller
// than the window still be read: the window cuts between lines, not between
// scopes.
func (m Model) contentLines() []string {
	width := m.contentWidth()
	catalog := m.filteredCatalog()

	if len(catalog) == 0 {
		return []string{lipgloss.NewStyle().
			Foreground(appstyles.Active.TextDim).
			Width(width).
			Render(fmt.Sprintf("No shortcuts match %q.", strings.TrimSpace(m.filterQuery)))}
	}

	var lines []string
	for i, scope := range catalog {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, strings.Split(renderScope(scope, width), "\n")...)
	}
	return lines
}

// maxOffset is the furthest the content can scroll: enough to bring the last
// line into view, never past it.
func (m Model) maxOffset() int {
	return max(0, len(m.contentLines())-m.contentRows())
}

// renderFilterBar renders the `/`-filter row: "/" plus the live query (or the
// focused input while typing) plus a hint. It renders empty when no filter is
// active, so assemble can splice it in unconditionally and contentRows —
// which measures assemble at the model's current state — picks up its height
// automatically.
func (m Model) renderFilterBar() string {
	if !m.filterTyping && !m.filterApplied {
		return ""
	}

	slash := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Bold(true).Render("/")
	hint := lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Render("esc to clear")

	if m.filterTyping {
		return slash + " " + m.filterInput.View() + "  " + hint
	}
	query := lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted).Render(m.filterQuery)
	return slash + " " + query + "  " + hint
}

// assemble wraps a body of content lines in the overlay's chrome: the title
// above, the filter bar when active, then the overflow counts, the dimming
// legend and the hint line below. contentRows measures itself against this,
// so the two can never disagree about what the chrome costs.
func (m Model) assemble(body, overflow string) string {
	windowed := overflow != ""
	width := m.contentWidth()
	dim := lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Width(width)

	sections := []string{chrome.ModalTitle("Keyboard shortcuts")}
	if bar := m.renderFilterBar(); bar != "" {
		sections = append(sections, bar)
	}
	sections = append(sections, body)

	if windowed {
		// Hidden-line counts, in the same "N above / N below" shape the task
		// tree's pinned section headers use for the same problem
		// (docs/DESIGN.md §12). Without it a windowed catalog looks complete.
		sections = append(sections, dim.Render(overflow))
	}

	// The overlay lists every key in the app on every screen, so it has to say
	// what the dimming means — otherwise a dimmed row reads as "removed" or
	// "broken" rather than "not on this screen".
	sections = append(sections, dim.Render("Dimmed keys exist but are not available on the screen you opened this from."))

	// The overlay's own keys, built from the same bindings as everything
	// else: it owns the keyboard while open, and an overlay advertises its
	// own keys because there is no footer beneath it. Scrolling and closing
	// are only advertised while the filter input is not eating every
	// keystroke — its own bar already explains esc while typing, and ? types
	// a literal character rather than closing.
	var hints []chrome.KeyHint
	if m.filterTyping {
		hints = append(hints, chrome.HintAs(keys.Overlay.Submit, "apply"))
	} else {
		if windowed {
			hints = append(hints, chrome.HintAs(keys.Overlay.Navigation, "scroll"))
		}
		hints = append(hints, chrome.HintAs(keys.Global.Filter, "filter"))
		hints = append(hints,
			chrome.HintAs(keys.Global.Help, "close"),
			chrome.HintAs(keys.Overlay.Cancel, "close"),
		)
	}
	sections = append(sections, chrome.RenderKeyHints(hints, appstyles.Active.TextMuted))

	return chrome.ModalSurface(appstyles.Active.ModalBg, strings.Join(sections, "\n\n"))
}

// overflowLabel reports how many content lines are hidden above and below the
// current window.
func (m Model) overflowLabel() string {
	lines := m.contentLines()
	offset := min(m.offset, m.maxOffset())
	end := min(offset+m.contentRows(), len(lines))

	var parts []string
	if offset > 0 {
		parts = append(parts, fmt.Sprintf("%d above", offset))
	}
	if below := len(lines) - end; below > 0 {
		parts = append(parts, fmt.Sprintf("%d below", below))
	}
	return strings.Join(parts, " · ")
}

func (m Model) View() tea.View {
	m.filterInput.SetWidth(max(0, m.contentWidth()-6))
	// Seal the filter input onto the modal surface every render, the same as
	// the task tree's own filter bar: the bubbles textinput default carries
	// no foreground on its focused Text and a hardcoded white on the
	// blurred one, which vanishes against the modal background.
	chrome.SealInput(&m.filterInput, appstyles.Active.ModalBg, appstyles.Active.ModalBg)

	lines := m.contentLines()
	windowed := m.maxOffset() > 0
	offset := min(m.offset, m.maxOffset())
	end := min(offset+m.contentRows(), len(lines))

	overflow := ""
	if windowed {
		overflow = m.overflowLabel()
	}
	return tea.NewView(m.assemble(strings.Join(lines[offset:end], "\n"), overflow))
}
