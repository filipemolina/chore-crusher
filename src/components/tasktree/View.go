package tasktree

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
)

// focusedZoneID is the zone id this component answers to
// (constants.COMPONENT_TASK_TREE). The value is part of the focus protocol,
// so it is written out here rather than imported — a zone must not
// accidentally answer to another zone's id.
const focusedZoneID = 1

// View renders raw content for Bubble Tea's Model contract. Taskspanel calls
// ViewInPanel with the exact inner Tasks dimensions during normal composition.
func (m Model) View() tea.View {
	return tea.NewView(m.ViewInPanel(chrome.PanelBodyWidth(m.body.MainWidth), chrome.PanelBodyHeight(m.body.Height), chrome.PanelBg(m.focused)))
}

// panelLine is one display line of the task-tree body. taskID names the task a
// row line belongs to (empty for chrome: section headers, blank spacers, the
// section rule, the filter bar, the inline create row, empty-state cards), so
// the scroll logic can find the selected row's line without re-deriving the
// header and spacing counts the renderer already knows (Commit 5 step 2).
type panelLine struct {
	taskID  string
	content string
	// section is non-empty for section header lines, set to "Pending" or
	// "Complete" so the renderer can pin the current section's header at the
	// top of the viewport and render its overflow suffix (docs/DESIGN.md §12).
	section string
}

// sectionLine creates a section-header panelLine tagged with its section name,
// so renderWindow can identify and pin it without inspecting rendered text.
func sectionLine(name string, count int) panelLine {
	return panelLine{content: sectionHeader(name, count), section: name}
}

// findSectionHeader scans the plan backwards from idx to find the section
// header ("Pending" or "Complete") that precedes it, returning the header's
// index in the plan, or -1 when idx is not under any section header
// (e.g. the filtered view, which has no section headers).
func findSectionHeader(plan []panelLine, idx int) int {
	for i := idx; i >= 0; i-- {
		if plan[i].section != "" {
			return i
		}
	}
	return -1
}

// countTaskRows counts plan lines that are task rows (non-empty taskID that is
// not the create sentinel) within [start, end). Used for the overflow suffix
// so the count reflects tasks, not chrome lines.
func countTaskRows(plan []panelLine, start, end int) int {
	n := 0
	for i := start; i < end && i < len(plan); i++ {
		if plan[i].taskID != "" && plan[i].taskID != createLineID {
			n++
		}
	}
	return n
}

// createLineID is the sentinel taskID the inline create row carries, so the
// scroll target can be the create row (where the cursor's input sits) rather
// than the anchor task while creating. Real task ids are ULIDs, so this can
// never collide with one.
const createLineID = "\x00create"

func chromeLine(content string) panelLine { return panelLine{content: content} }

// ViewInPanel renders the task tree as raw Tasks-surface content, windowed to
// height so a list taller than the panel stays reachable by scrolling.
// Taskspanel owns the enclosing frame, title, elevation, and footer.
func (m Model) ViewInPanel(width, height int, bg color.Color) string {
	m.filterInput.SetWidth(max(0, width-6))

	switch {
	case !m.activeList:
		return appstyles.FillBackground(bg, lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Render("Add a task to get started"))
	case len(m.rows) == 0 && !m.creating:
		// An empty list after an esc cancel: the standard recessed empty-state
		// card. The input is no longer the empty state — single-press esc
		// removes it (docs/plan/task-row-cards-and-status.md).
		return appstyles.FillBackground(bg, chrome.EmptyStateCard("No tasks yet.\nPress n to create one.", width, height))
	case m.filterActive() && len(filterMatches(m.rows, m.filterQuery)) == 0:
		// Filtered to nothing: the filter bar over a recessed "no match" card.
		// There are no rows to scroll, so this renders directly rather than
		// through the line-plan window.
		// No rows survived, so there are no direct matches either: the bar
		// reports 0 over the card.
		body := lipgloss.JoinVertical(lipgloss.Top, m.renderFilterBar(0), chrome.EmptyStateCard("No tasks match", width, 3))
		return appstyles.FillBackground(bg, fillToHeight(body, height, width, bg))
	default:
		plan := m.linePlan(width, bg)
		return appstyles.FillBackground(bg, m.renderWindow(plan, height, width, bg))
	}
}

// linePlan builds the one task-tree line plan the renderer and the scroll math
// share. It mirrors ViewInPanel's state precedence: inline creation and the
// unfiltered view both render the Pending/Complete sections (creation splices
// its input row in), and an active filter renders the flat filtered list.
func (m *Model) linePlan(width int, bg color.Color) []panelLine {
	switch {
	case m.creating:
		pending, complete := m.splitSections()
		return m.planSections(pending, complete, width, bg)
	case m.filterActive():
		return m.planFiltered(width, bg)
	default:
		pending, complete := m.splitSections()
		return m.planSections(pending, complete, width, bg)
	}
}

// renderWindow renders exactly the height display lines starting at the
// selection-driven scroll offset, then pads any short remainder to height with
// the panel background so the window always paints its full box (no bleed on a
// short tail). The offset is re-derived from the stored scrollOffset here so a
// render never mutates persistent state; Update is what advances scrollOffset.
//
// The section header for the section the cursor is in is pinned to the top of
// the viewport (docs/DESIGN.md §12): when the header has scrolled past, it
// stays visible as a fixed line rather than disappearing. The pinned header
// also carries the overflow suffix ("N above . N below") that tells the user
// how many task rows are hidden above and/or below the window.
func (m Model) renderWindow(plan []panelLine, height, width int, bg color.Color) string {
	if height <= 0 {
		return ""
	}
	selIdx := m.selectedLineIndex(plan)
	off := clampScroll(len(plan), selIdx, height, m.scrollOffset)

	// Determine whether a section header needs pinning: find the header for
	// the section the selection lives in, and check if it is above the window.
	headerIdx := findSectionHeader(plan, selIdx)
	pinned := false
	if headerIdx >= 0 && headerIdx < off {
		// Header is above the window — pin it at the top and shrink the
		// content area by one line so the selection stays visible.
		off = clampScroll(len(plan), selIdx, height-1, m.scrollOffset)
		pinned = true
	}

	end := min(len(plan), off+height)
	if pinned {
		end = min(len(plan), off+height-1) // make room for the pinned header line
	}

	lines := make([]string, 0, height)

	// Prepend the pinned header (with overflow suffix) if applicable.
	if pinned {
		header := plan[headerIdx]
		overhead := overflowSuffix(plan, off, end, height)
		lines = append(lines, header.content+overhead)
	}

	for i := off; i < end; i++ {
		lines = append(lines, plan[i].content)
	}
	for len(lines) < height {
		lines = append(lines, lipgloss.NewStyle().Background(bg).Width(max(0, width)).Render(""))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// overflowSuffix computes the "N above" / "N below" text suffix for the pinned
// section header, rendered in TextDim (docs/DESIGN.md §12). When there are
// more task rows hidden above the viewport it reports "N above"; below, "N
// below"; both as "N above . N below". When everything fits, the suffix is
// empty.
func overflowSuffix(plan []panelLine, off, end, height int) string {
	above := countTaskRows(plan, 0, off)
	below := countTaskRows(plan, end, len(plan))
	if above == 0 && below == 0 {
		return ""
	}
	dim := lipgloss.NewStyle().Foreground(appstyles.Active.TextDim)
	parts := make([]string, 0, 2)
	if above > 0 {
		parts = append(parts, dim.Render(fmt.Sprintf("%d above", above)))
	}
	if below > 0 {
		parts = append(parts, dim.Render(fmt.Sprintf("%d below", below)))
	}
	return "  " + strings.Join(parts, " . ")
}

// fillToHeight clips or pads a rendered block to exactly height lines, padding
// with background-painted blanks so a short block still seals its full box.
func fillToHeight(block string, height, width int, bg color.Color) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(block, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, lipgloss.NewStyle().Background(bg).Width(max(0, width)).Render(""))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// splitSections splits displayed rows into Pending and Complete based on root
// task status. displayedRows is used (not visibleRows) so the sections match
// the on-screen order the cursor actually navigates (phase B step 3).
func (m *Model) splitSections() (pending, complete []apptypes.Row) {
	for _, row := range m.displayedRows() {
		if row.Depth == 0 {
			if row.Task.Status == apptypes.StatusComplete {
				complete = append(complete, row)
			} else {
				pending = append(pending, row)
			}
		} else {
			// Non-root: follow parent
			if row.Task.ParentID != nil {
				parent := m.findRow(*row.Task.ParentID)
				if parent != nil && parent.Task.Status == apptypes.StatusComplete {
					complete = append(complete, row)
				} else {
					pending = append(pending, row)
				}
			} else {
				pending = append(pending, row)
			}
		}
	}
	return
}

// countByStatusNotComplete and countByStatusComplete count rows by their own
// Task.Status rather than by section membership. A section's row count (the
// slice length) and its status count can disagree: a completed subtask of a
// pending parent is deliberately displayed inside the Pending section (see
// splitSections), so that section's rows are not all pending. The Lists
// panel counts statuses (apptypes.ListSummary.PendingCount/CompleteCount),
// so the tree's section headers must too, or the same list shows two
// different "pending" numbers on one screen (bug: the same list shows two
// different counts on one screen). Rows are already in memory here; this
// does not re-query the store, which could disagree with what is drawn.
func countByStatusNotComplete(rows []apptypes.Row) int {
	n := 0
	for _, row := range rows {
		if row.Task.Status != apptypes.StatusComplete {
			n++
		}
	}
	return n
}

func countByStatusComplete(rows []apptypes.Row) int {
	n := 0
	for _, row := range rows {
		if row.Task.Status == apptypes.StatusComplete {
			n++
		}
	}
	return n
}

// planSections builds the line plan for the Pending and Complete sections,
// splicing the inline create row in immediately after the reference task (its
// insertion point) within whichever section that task lives. The create row is
// never shown as a "no tasks yet" card: on an empty list it is the only row,
// placed at the end. Each task row line carries its task id; headers, spacers,
// the rule, and the create row are chrome lines (empty id).
func (m *Model) planSections(pending, complete []apptypes.Row, width int, bg color.Color) []panelLine {
	var plan []panelLine
	placedCreate := false

	// An empty list's create row opens under the Pending header: the input
	// creates a pending task (store.CreateTask inserts status 'pending'), so
	// the card belongs to the Pending section even while that section has
	// nothing in it yet (docs/plan/task-row-cards-and-status.md).
	if m.creating && len(pending) == 0 && len(complete) == 0 {
		plan = append(plan, sectionLine("Pending", 0))
		plan = append(plan, chromeLine(""))
		plan = append(plan, m.createLine(width, bg))
		return plan
	}

	if len(pending) > 0 {
		plan = append(plan, sectionLine("Pending", countByStatusNotComplete(pending)))
		// One blank line below each section title, and one below the last
		// pending row, so the sections read as blocks with air around them
		// (docs/DESIGN.md §6).
		plan = append(plan, chromeLine(""))
		plan, placedCreate = m.appendSectionPlan(plan, pending, width, bg, placedCreate)
		if len(complete) > 0 {
			plan = append(plan, chromeLine(""))
		}
	}

	if len(pending) > 0 && len(complete) > 0 {
		plan = append(plan, chromeLine(chrome.PanelRule(width)))
	}

	if len(complete) > 0 {
		plan = append(plan, sectionLine("Complete", countByStatusComplete(complete)))
		plan = append(plan, chromeLine(""))
		plan, placedCreate = m.appendSectionPlan(plan, complete, width, bg, placedCreate)
	}

	if m.creating && !placedCreate {
		plan = append(plan, m.createLine(width, bg))
	}

	return plan
}

// createLine renders the inline create row as a plan line tagged with the
// create sentinel, so the scroll target follows the input.
func (m *Model) createLine(width int, bg color.Color) panelLine {
	return panelLine{taskID: createLineID, content: m.renderCreateRow(width, bg)}
}

// createRenderAnchorID returns the id of the row after which the inline
// create row should be rendered in the given visible slice: the anchor's
// last visible descendant, or the anchor itself if it has no visible
// descendants, or "" when there is no anchor (append at end). This keeps the
// create card visually after the selected task's entire visible subtree,
// matching where the committed task lands (resolveCreateLocation inserts the
// sibling after the anchor's subtree): selecting a parent-with-children and
// pressing n must render (and create) the new sibling after that subtree,
// not between the parent and its first child (bug 5).
func createRenderAnchorID(rows []apptypes.Row, beforeID string) string {
	if beforeID == "" {
		return ""
	}
	anchorIdx := -1
	anchorDepth := -1
	for i, r := range rows {
		if r.Task.ID == beforeID {
			anchorIdx = i
			anchorDepth = r.Depth
			break
		}
	}
	if anchorIdx < 0 {
		return beforeID
	}
	// In depth-first preorder, descendants form a contiguous run after the
	// anchor with depth > anchorDepth; the last one is the render anchor. If
	// none exist (the anchor has no visible children, e.g. a collapsed
	// subtree), the anchor itself is the render point.
	lastDesc := anchorIdx
	for i := anchorIdx + 1; i < len(rows); i++ {
		if rows[i].Depth <= anchorDepth {
			break
		}
		lastDesc = i
	}
	return rows[lastDesc].Task.ID
}

// appendSectionPlan appends each row of a section as a task line, inserting the
// create row immediately after the anchor's last visible descendant when in
// creating mode (createRenderAnchorID), so a parent-with-children does not split
// from its subtree around the input (bug 5).
func (m *Model) appendSectionPlan(plan []panelLine, rows []apptypes.Row, width int, bg color.Color, placed bool) ([]panelLine, bool) {
	anchor := ""
	if m.creating {
		anchor = createRenderAnchorID(rows, m.createBeforeID)
	}
	for _, row := range rows {
		plan = append(plan, panelLine{taskID: row.Task.ID, content: m.renderRow(row, width, bg, nil)})
		if m.creating && !placed && anchor != "" && row.Task.ID == anchor {
			plan = append(plan, m.createLine(width, bg))
			placed = true
		}
	}
	return plan, placed
}

// sectionHeader renders a bold TextPrimary name followed by a dimmed count,
// the same "name, then a muted count" shape the lists panel uses for a list
// row (docs/DESIGN.md §12).
func sectionHeader(name string, count int) string {
	return primary(true).Render(name) + " " + muted().Render("("+strconv.Itoa(count)+")")
}

// planFiltered builds the line plan for the /-filter view: the filter bar over
// the flat filtered row list. The Pending/Complete section headers are
// suppressed while filtering — there is no honest way to split a half-filtered
// set into them (docs/plans/phase-8-search.md step 1). The empty-match case is
// handled directly in ViewInPanel, so this is only reached with rows to show.
func (m *Model) planFiltered(width int, bg color.Color) []panelLine {
	rows, matched := matchVisible(m.rows, m.filterQuery)

	plan := []panelLine{chromeLine(m.renderFilterBar(len(matched)))}
	for _, row := range rows {
		// Only dim ancestors of a real match; when the query is empty (the
		// input is open but nothing typed yet) nothing is dimmed.
		_, isMatch := matched[row.Task.ID]
		dimmed := m.filterQuery != "" && !isMatch
		plan = append(plan, panelLine{taskID: row.Task.ID, content: m.renderFilterRow(row, width, dimmed, matched[row.Task.ID], bg)})
	}

	return plan
}

// renderFilterBar shows the live input while typing ([/ query]) or, once a
// query is applied, a dimmed summary of the same query. Both states carry the
// same affordances — the match count and "esc to clear" — so the filter reads
// identically from the first keystroke onward and never implies that enter is
// what makes it work. matches is the number of directly-matched rows, which is
// what the user is counting; the elided ancestors kept for tree context are
// not matches and are excluded by the caller.
func (m *Model) renderFilterBar(matches int) string {
	slash := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Bold(true).Render("/")
	// The suffix sits in the footer hint tier (TextDim), one step below the
	// query itself (TextMuted), so it reads as chrome (docs/DESIGN.md §12).
	hint := dim().Render
	suffix := hint("esc to clear")
	// With nothing typed yet the filter matches nothing in particular, so a
	// count would be noise; esc still works and still says so.
	if m.filterQuery != "" {
		unit := "matches"
		if matches == 1 {
			unit = "match"
		}
		suffix = hint(fmt.Sprintf("%d %s", matches, unit)) + "  " + suffix
	}
	if m.filterTyping {
		return slash + " " + m.filterInput.View() + "  " + suffix
	}
	return slash + " " + muted().Render(m.filterQuery) + "  " + suffix
}

// renderFilterRow renders one filtered row. A directly-matched row renders like
// a normal task row; an ancestor that only stays visible to anchor a match
// renders dimmed so the two are distinguishable (docs/plans/phase-8-search.md
// step 1's unmatched styling).
func (m *Model) renderFilterRow(row apptypes.Row, width int, dimmed bool, matchedIndexes []int, bg color.Color) string {
	if dimmed {
		cardIndent := strings.Repeat(" ", 2*row.Depth)
		cardWidth := max(0, width-len(cardIndent))
		content := dim().Render(chrome.Truncate("[…] "+row.Task.Title, max(1, cardWidth-cardInset)))
		return cardIndent + renderTaskCard(cardWidth, bg, appstyles.Active.TextMuted, content)
	}
	// Highlight matched substring inside matching rows, using
	// sahilm/fuzzy's MatchedIndexes — same fuzzy matching used
	// by the F picker (searchpicker) and matchVisible. The
	// fuzzy.Find call in matchVisible already computed these
	// indexes, reusing the same data path.
	return m.renderRow(row, width, bg, matchedIndexes)
}

// renderRow renders one task row as a full-width card: a ▌ bar column whose
// color is accent when the row is selected and the row's own status color
// otherwise, then the columns (checkbox, title) and the right-aligned
// progress+status block. The expand/collapse marker (▾/▸) sits at the end of
// the title, and each level of depth indents the whole card by two columns,
// so a subtask's bar steps right and the row reads at its real depth
// (docs/DESIGN.md §12).
func (m *Model) renderRow(row apptypes.Row, width int, bg color.Color, matchedIndexes []int) string {
	cardIndent := strings.Repeat(" ", 2*row.Depth)
	cardWidth := max(0, width-len(cardIndent))

	// Text-presentation checkbox glyphs (no emoji presentation, single cell):
	// ◻ pending and in progress, ◼ complete. In progress shares the pending
	// square — the IN PROGRESS label and bar colour set it apart.
	checkbox := "◻"
	checkboxFg := appstyles.Active.TextMuted
	textFg := appstyles.Active.TextPrimary
	if row.Task.Status == apptypes.StatusComplete {
		checkbox = "◼"
		checkboxFg = appstyles.Active.StatusComplete
		textFg = appstyles.Active.TextMuted
	}

	// The expand/collapse marker is part of the title, not a leading column,
	// so a parent's title starts at its own depth (docs/DESIGN.md §12).
	trailing := ""
	if row.HasChildren {
		if m.collapsed[row.Task.ID] {
			trailing = "▸"
		} else {
			trailing = "▾"
		}
	}

	// A filtered row arrives with the offsets its query matched; highlighting
	// paints the whole title (matched runs in accent, the rest in textFg), so
	// it subsumes the complete-row dimming rather than nesting inside it.
	title := row.Task.Title
	switch {
	case len(matchedIndexes) > 0:
		title = highlightMatch(title, matchedIndexes, textFg)
	case row.Task.Status == apptypes.StatusComplete:
		title = lipgloss.NewStyle().Foreground(textFg).Render(title)
	}

	isSelected := row.Task.ID == m.selectedID
	rowBg := chrome.ListRowBg(isSelected, m.focused)

	checkboxColored := lipgloss.NewStyle().Foreground(checkboxFg).Render(checkbox)
	notesGlyph := ""
	if row.Task.Notes != "" {
		notesGlyph = detailsIcon
	}
	commentsGlyph := ""
	if row.HasComments {
		commentsGlyph = commentsIcon
	}
	// Compose both glyphs into the fixed two-cell trailing column: an absent
	// glyph becomes a single space, so the combined string is always exactly
	// two cells (notes left, comments right) regardless of which indicators
	// the row carries.
	detailsGlyph := padRightGlyph(notesGlyph) + padRightGlyph(commentsGlyph)
	// Build the agent spinner text: "⠙ agentA" when claimed, "" otherwise.
	agentSpinner := ""
	if a, ok := m.work[row.Task.ID]; ok {
		spinner := chrome.Spinner(m.animFrame)
		agent := a.AgentID
		if len(agent) > 6 {
			agent = agent[:6]
		}
		agentSpinner = spinner + " " + agent
	}

	content := buildRowContent(checkboxColored, title, trailing,
		statusLabel(row.Task.Status), progressLabel(row, m.rows), detailsGlyph,
		agentSpinner, 1,
		cardWidth-cardInset, statusFg(row.Task.Status), spinnerFg(isSelected))

	return cardIndent + renderTaskCard(cardWidth, rowBg, barFgFor(row.Task.Status, isSelected), content)
}

// renderCreateRow renders the inline "new task" row as a card styled like a
// selected task row: ModalBg background, accent bar, Padding(0,1,0,0), the level
// glyph (-/+/^) in accent where the checkbox would sit, and the placeholder
// or typed text. There is no → prompt — the card chrome marks the row as
// active. The whole card is indented to the depth the new task will land at,
// matching task rows (docs/DESIGN.md §12).
func (m *Model) renderCreateRow(width int, bg color.Color) string {
	glyph := "-"
	switch m.createLevelOffset {
	case +1:
		glyph = "+"
	case -1:
		glyph = "^"
	}

	selectedDepth := 0
	if m.selectedID != "" {
		if row := m.findRow(m.selectedID); row != nil {
			selectedDepth = row.Depth
		}
	}
	cardIndent := strings.Repeat(" ", max(0, 2*(selectedDepth+m.createLevelOffset)))
	cardWidth := max(0, width-len(cardIndent))

	// prefix = glyph + space
	prefixWidth := 2
	m.createInput.SetWidth(max(1, cardWidth-cardInset-prefixWidth))

	glyphColored := lipgloss.NewStyle().Foreground(appstyles.Active.Accent).Render(glyph)

	var title string
	if m.createInput.Value() == "" {
		title = lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Render(m.createInput.Placeholder)
	} else {
		title = m.createInput.View()
	}

	content := lipgloss.JoinHorizontal(lipgloss.Left, glyphColored, " ", title)
	return cardIndent + renderTaskCard(cardWidth, appstyles.Active.ModalBg, appstyles.Active.Accent, content)
}

// taskRowCols describes the computed width of each column in a task row.
type taskRowCols struct {
	checkbox     int
	title        int
	status       int
	progress     int
	agentSpinner int
	details      int
}

// statusColWidth is the fixed width of the status column: the longest status
// label, "IN PROGRESS" (11 runes). Every status label is right-aligned inside
// this width so PENDING / IN PROGRESS / COMPLETE all end at the same column and
// the trailing icon column begins at the same offset on every row (decision 2,
// docs/plan/ui-improvements.md; docs/DESIGN.md §12).
const statusColWidth = 11

// detailsColWidth is the fixed width of the trailing icon column — two display
// cells: the document glyph (notes) on the left and the comments glyph on the
// right, both right-aligned within the column. Each glyph is one cell; an
// absent glyph is a single space, so the combined string is always exactly two
// cells and every row's right edge stays aligned (docs/DESIGN.md §12;
// docs/plan/task-comments.md §6, Commit 4).
const detailsColWidth = 2

// computeTaskRowCols distributes tableWidth among the task row's columns.
// checkbox is never dropped; title is never dropped; progress, agentSpinner,
// and the status+icon block are dropped whole (in that order) when the table
// is too narrow. The status column is a fixed statusColWidth allocation and the
// trailing icon column a fixed detailsColWidth allocation — reserved together
// regardless of whether the row has notes — so the status label and the glyph
// (or its blank cell) align across rows. Drop order matches
// docs/plan/mcp-server-enhancement.md §3.7; the fixed trailing block is
// decision 2 of docs/plan/ui-improvements.md.
func computeTaskRowCols(tableWidth, checkboxWidth int, status, progress, agentSpinner string) taskRowCols {
	cols := taskRowCols{checkbox: checkboxWidth}

	statusW := 0
	detailsW := 0
	if status != "" {
		statusW = statusColWidth + 1   // +1 leading gap before the status column
		detailsW = detailsColWidth + 1 // +1 gap between the status and icon columns
	}
	progressW := 0
	if progress != "" {
		progressW = len(progress) + 1 // +1 for trailing gap
	}
	agentW := 0
	if agentSpinner != "" {
		agentW = len(agentSpinner) + 1 // +1 for trailing gap
	}

	// The status label and its trailing icon column are one atomic right block:
	// they are reserved together and shed together, so a narrow row never shows
	// a partial icon or a status fragment.
	rightBlock := statusW + detailsW

	// Drop order: progress first, then agent-spinner, then the whole right block.
	if progressW > 0 && tableWidth-rightBlock-agentW-progressW < 1 {
		progressW = 0
	}
	if agentW > 0 && tableWidth-rightBlock-agentW < 1 {
		agentW = 0
	}
	if rightBlock > 0 && tableWidth-rightBlock < 1 {
		statusW, detailsW, rightBlock = 0, 0, 0
	}

	cols.status = statusW
	cols.details = detailsW
	cols.progress = progressW
	cols.agentSpinner = agentW
	cols.title = max(1, tableWidth-rightBlock-agentW-progressW)

	return cols
}

// statusLabel returns the display label for a task status, all caps
// (docs/plan/task-row-cards-and-status.md).
func statusLabel(status apptypes.Status) string {
	switch status {
	case apptypes.StatusPending:
		return "PENDING"
	case apptypes.StatusInProgress:
		return "IN PROGRESS"
	case apptypes.StatusComplete:
		return "COMPLETE"
	}
	return ""
}

// statusFg returns the color the status label (and, unselected, the row's
// bar column) draws with for a task's status: muted grey for pending, the
// theme's warning amber for in progress, and its success green for complete.
// All three are active-theme tokens — no hardcoded colors
// (docs/plan/task-row-cards-and-status.md).
func statusFg(status apptypes.Status) color.Color {
	switch status {
	case apptypes.StatusInProgress:
		return appstyles.Active.StatusInProgress
	case apptypes.StatusComplete:
		return appstyles.Active.StatusComplete
	default:
		return appstyles.Active.TextMuted
	}
}

// barFgFor is the bar-column rule: accent on the selected row, otherwise the
// row's own status color (docs/plan/task-row-cards-and-status.md).
func barFgFor(status apptypes.Status, isSelected bool) color.Color {
	if isSelected {
		return appstyles.Active.Accent
	}
	return statusFg(status)
}

// spinnerFg is the agent-spinner color rule (docs/plan/mcp-server-enhancement.md
// §3.7): accent on the selected row, TextDim otherwise — the same selected-row
// rule as the bar column, so a claimed selected row reads accent all the way
// across.
func spinnerFg(isSelected bool) color.Color {
	if isSelected {
		return appstyles.Active.Accent
	}
	return appstyles.Active.TextDim
}

// progressLabel returns the display label for an in-progress task's progress,
// or "" when the task has no progress to show.
func progressLabel(row apptypes.Row, rows []apptypes.Row) string {
	if row.Task.Status != apptypes.StatusInProgress {
		return ""
	}
	switch row.Task.ProgressKind {
	case apptypes.ProgressPercentage:
		if row.Task.ProgressPct != nil {
			return fmt.Sprintf("%d%%", *row.Task.ProgressPct)
		}
	case apptypes.ProgressSubtasks:
		pct, displayAsSimple := apptypes.DerivedPercent(rows, row.Task.ID)
		if !displayAsSimple {
			return fmt.Sprintf("%d%%", pct)
		}
	}
	return ""
}

// cardInset is the card chrome a task row spends before its content: the ▌
// bar column plus the card's horizontal padding (Padding(0,1,0,0) = right 1).
// buildRowContent and the create row budget against width-cardInset, which is
// the card's real inner width, so the status cell ends flush at the right
// padding (docs/DESIGN.md §12).
const cardInset = 2

// detailsIcon marks a task whose notes are non-empty — the details screen
// (enter) has something to show. It is right-aligned in the fixed trailing icon
// column, immediately right of the status column, in TextMuted (docs/DESIGN.md
// §12's glyph table). U+1F5CE DOCUMENT (🗎) measures one cell in go-runewidth,
// like the rest of the vocabulary, though emoji-capable fonts may render it
// wider (docs/DESIGN.md §12 records the caveat).
const detailsIcon = "🗎"

// commentsIcon marks a task that has at least one comment. It pairs with
// detailsIcon in the fixed two-cell trailing icon column: notes on the left,
// comments on the right (docs/plan/task-comments.md §6, Commit 4). U+1F4AC
// (💬) measures two cells in go-runewidth, so — unlike the one-cell 🗎 — it
// cannot share a one-cell slot in the column. U+1F5E8 LEFT SPEECH BUBBLE
// (🗨) is the one-cell form (verified via go-runewidth v0.0.23, 2026-08-06)
// and is used instead, accepting the same emoji-font widening caveat the
// document glyph already carries.
const commentsIcon = "🗨"

// padRightGlyph returns glyph padded to exactly one display cell, so two
// one-cell glyphs compose into a fixed two-cell string: an absent glyph
// becomes a single space, a glyph already one cell is returned unchanged.
// Both detailsIcon and commentsIcon are go-runewidth one-cell by construction
// (see their constant docs), so this is the only normalization needed.
func padRightGlyph(glyph string) string {
	if glyph == "" {
		return " "
	}
	return glyph
}

// buildRowContent renders a task row's columns — checkbox, title (plus the
// optional trailing expand/collapse marker), and the right-aligned
// progress+agent-spinner+status block — to fit a card's inner content width.
// cols.title absorbs the remaining budget after the progress, agent-spinner,
// and status columns, so the status cell ends flush at the card's right
// padding: that is the right-alignment ("status at the end of the line").
// Drop order under narrowness: progress sheds first, then agent-spinner,
// then status, all whole (docs/plan/mcp-server-enhancement.md §3.7).
func buildRowContent(checkbox, title, trailing, status, progress, detailsGlyph, agentSpinner string,
	checkboxWidth, contentWidth int, statusColor, spinnerColor color.Color) string {
	prefixWidth := checkboxWidth + 1
	tableWidth := contentWidth - prefixWidth
	if tableWidth < 1 {
		tableWidth = 1
	}

	cols := computeTaskRowCols(tableWidth, checkboxWidth, status, progress, agentSpinner)

	checkboxCell := lipgloss.NewStyle().Width(cols.checkbox).Render(checkbox)

	// The trailing marker (" ▾") is only rendered when it fits: at the
	// narrowest widths the title alone claims the column and the marker is
	// shed rather than pushed past the cell.
	trailingW := 0
	if trailing != "" {
		need := 1 + lipgloss.Width(trailing)
		if cols.title > need {
			trailingW = need
		}
	}
	titleText := chrome.Truncate(title, max(1, cols.title-trailingW))
	if trailingW > 0 {
		titleText += " " + trailing
	}
	titleCell := lipgloss.NewStyle().Width(cols.title).Render(titleText)

	var progressCell, agentSpinnerCell, statusCell, detailsCell string
	if cols.progress > 0 && progress != "" {
		progressCell = lipgloss.NewStyle().
			Foreground(appstyles.Active.TextDim).
			Width(cols.progress).
			Render(chrome.Truncate(progress, max(1, cols.progress-1)))
	}
	if cols.agentSpinner > 0 && agentSpinner != "" {
		agentSpinnerCell = lipgloss.NewStyle().
			Foreground(spinnerColor).
			Width(cols.agentSpinner).
			Render(chrome.Truncate(agentSpinner, max(1, cols.agentSpinner-1)))
	}
	if cols.status > 0 && status != "" {
		// Fixed-width status column: the label is right-aligned so PENDING /
		// IN PROGRESS / COMPLETE all end at the same column, and the trailing
		// icon column begins at the same offset on every row.
		statusCell = lipgloss.NewStyle().
			Foreground(statusColor).
			Width(cols.status).
			Align(lipgloss.Right).
			Render(chrome.Truncate(status, statusColWidth))
		// Fixed trailing icon column: the document glyph (or, for a row with no
		// notes, a blank cell of the same width) right-aligned as the row's last
		// cell — so noted and un-noted rows keep the same right edge.
		detailsCell = lipgloss.NewStyle().
			Foreground(appstyles.Active.TextMuted).
			Width(cols.details).
			Align(lipgloss.Right).
			Render(detailsGlyph)
	}

	parts := []string{checkboxCell, " ", titleCell, progressCell, agentSpinnerCell, statusCell, detailsCell}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

// renderTaskCard wraps row content in the shared row-card chrome: the ▌ bar
// column, Padding(0,1,0,0), and the row's background, spanning the full panel
// body width. No vertical padding — a one-line title makes a one-line card
// (the "thinner" ask; the lists' vertical Padding(1) is what makes their rows
// 4 lines tall). Ported from listspanel's delegate (docs/DESIGN.md §12). Bar +
// wrapper sum to exactly width — which is what makes a selected row's
// ModalBg read as a full-width band rather than a highlight behind the text.
func renderTaskCard(width int, bg, barFg color.Color, content string) string {
	wrapper := lipgloss.NewStyle().
		Width(max(0, width-1)).
		Padding(0, 1, 0, 0).
		Background(bg).
		Render(content)

	return appstyles.FillBackground(bg,
		lipgloss.JoinHorizontal(lipgloss.Left, chrome.BarColumn(barFg, bg, wrapper), wrapper))
}

func primary(bold bool) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)
	if bold {
		s = s.Bold(true)
	}
	return s
}

func muted() lipgloss.Style { return lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted) }
func dim() lipgloss.Style   { return lipgloss.NewStyle().Foreground(appstyles.Active.TextDim) }

// highlightMatch styles the query's matched characters inside a title so the
// user can see why a row survived the filter. matchedIndexes are byte offsets
// into title, as returned by sahilm/fuzzy's MatchedIndexes — the very slice
// matchVisible got from the Find call that selected this row, so the highlight
// can never disagree with the match.
//
// Only the matched runs are restyled (accent, bold); the rest of the title
// keeps baseFg, the colour the row would have used anyway. Styling the whole
// title here rather than letting the caller wrap the result keeps the ANSI
// flat: a nested Render would reset colour at the first inner span and lose
// the outer one for the remainder of the line.
func highlightMatch(title string, matchedIndexes []int, baseFg color.Color) string {
	base := lipgloss.NewStyle().Foreground(baseFg)
	if len(matchedIndexes) == 0 {
		return base.Render(title)
	}

	matched := make(map[int]bool, len(matchedIndexes))
	for _, i := range matchedIndexes {
		matched[i] = true
	}
	accent := lipgloss.NewStyle().Foreground(appstyles.Active.Accent).Bold(true)

	// Walk runes, flushing a run whenever it flips between matched and not, so
	// "gard" in "Plan the garden" emits one accent span, not four.
	var out strings.Builder
	var run strings.Builder
	runMatched := false
	flush := func() {
		if run.Len() == 0 {
			return
		}
		if runMatched {
			out.WriteString(accent.Render(run.String()))
		} else {
			out.WriteString(base.Render(run.String()))
		}
		run.Reset()
	}
	for i, r := range title {
		if isMatch := matched[i]; isMatch != runMatched {
			flush()
			runMatched = isMatch
		}
		run.WriteRune(r)
	}
	flush()
	return out.String()
}
