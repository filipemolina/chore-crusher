package helpoverlay

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/farol/src/keys"
)

// helpOverlayMaxWidth caps the content column so a long description wraps to
// a couple of lines on wide terminals rather than stretching into one
// unreadable line. Each entry is already one key/description row, so this
// only wraps within a single row, never across rows.
const helpOverlayMaxWidth = 64

// Model is the ? overlay: every key in the app, grouped by scope and
// rendered from keys.Catalog, so what it says is what the handlers do.
// Rows the user could not press in the screen it was opened from are dimmed.
//
// The catalog lists the whole app rather than the current screen, which is
// more than one terminal's worth of lines, so the scope content is windowed
// and scrolls with ↑/↓ (keys.Overlay.Navigation), and narrowable with a
// `/`-fuzzy filter (keys.Global.Filter) over each entry's key and
// description — the same affordance the task tree's own filter uses.
// Without windowing the tail of the catalog — Details and Overlays — would
// simply be unreachable on an 80x24 terminal, which is the same "bound but
// undiscoverable" failure the completeness rule exists to prevent.
type Model struct {
	catalog    []keys.Scope
	termWidth  int
	termHeight int
	// offset is the first visible content line. Scope content is one flat
	// list of already-wrapped lines by the time it is windowed, so scrolling
	// is per line rather than per scope: a scope taller than the window is
	// still readable.
	offset int

	// filterInput is the `/`-filter's text box. filterTyping is true while it
	// owns the keyboard (every keystroke but enter/esc lands in it);
	// filterApplied is true once enter locks the query in and the input
	// blurs, matching keys.Overlay.Navigation back to scrolling. filterQuery
	// mirrors filterInput's value live, so the catalog narrows as the user
	// types rather than waiting for enter (the same live-preview behaviour
	// as the task tree's own filter).
	filterInput   textinput.Model
	filterTyping  bool
	filterApplied bool
	filterQuery   string
}

func (m Model) Init() tea.Cmd { return nil }

// New builds the help overlay for the screen described by ctx (which keys
// are pressable) and the terminal size for wrapping and windowing.
func New(ctx keys.Context, termWidth, termHeight int) tea.Model {
	fi := textinput.New()
	// The bubbles default prompt is a hardcoded ANSI-white "> ", which would
	// render between the bar's "/" and the query — see the task tree's own
	// filter input for the same fix.
	fi.Prompt = ""
	return Model{
		catalog:     keys.Catalog(ctx),
		termWidth:   termWidth,
		termHeight:  termHeight,
		filterInput: fi,
	}
}

// contentRows is how many lines of scope content fit on this terminal: the
// terminal minus everything the overlay spends on chrome — the modal border
// and padding, the title, the overflow counts, the legend and the hint line,
// plus the blank line between each.
//
// It is MEASURED rather than counted from a constant, by assembling the
// overlay around a single content line and subtracting it. Several of those
// pieces wrap at some widths and not others (the legend is two lines at 64
// columns, one at 80), so a hardcoded number is right at one size and wrong at
// the next — and being wrong here means the overlay renders taller than the
// terminal, which is the whole failure this windowing exists to prevent.
func (m Model) contentRows() int {
	// A worst-case overflow label, so the measurement never comes out short
	// of what the real one occupies. It must not be computed from the real
	// window: overflowLabel needs contentRows, and that would recurse.
	chrome := lipgloss.Height(m.assemble("x", "999 above · 999 below")) - 1
	return max(1, m.termHeight-chrome)
}
