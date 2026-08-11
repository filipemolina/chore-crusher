package cli

import "github.com/filipemolina/farol/src/store"

// notesBudget caps the total inlined body bytes in one `farol tasks
// --include` response. A row's body is never cut mid-text: a row that would
// push the response past the budget keeps its (has_notes, notes_len) flags
// but its body stays out, and its id is reported in `elided` — it comes back
// whole or not at all. Ported verbatim from the MCP list_tasks handler so the
// two surfaces stay byte-for-byte compatible.
const notesBudget = 40000

// inlineBodyBudget inlines notes and comments into rows under the byte
// budget, walking rows in preorder and accumulating len(notes) +
// sum(len(comment.note)). Once a row's body would push the running total
// past notesBudget, that row and every later row keep has_notes/notes_len but
// get no inlined body, and their ids are returned in elided — never cut
// mid-text. Skeleton rows (context_only) never take from the budget: their
// bodies are never inlined. budgetExceeded reports whether the budget was hit
// at all, which is exactly len(elided) > 0.
//
// Only rows that actually have a body are charged to the budget or named in
// elided. elided exists so the caller can re-fetch the dropped bodies with
// `farol show`, so listing a row with no notes and no comments would buy a
// round-trip that returns nothing — a cost worth removing.
//
// Comment presence comes from ONE store.TaskIDsWithComments query for the
// whole list, not a ListComments per row: that helper exists for this exact
// N+1 — a per-request read stays per-request. Only rows the set says are
// commented are read.
func inlineBodyBudget(s *store.Store, listID string, rows []taskRowJSON, tasks []store.Task, includeNotes, includeComments bool) (elided []string, budgetExceeded bool, err error) {
	byID := make(map[string]string, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t.Notes
	}
	commented := map[string]bool{}
	if includeComments {
		commented, err = s.TaskIDsWithComments(listID)
		if err != nil {
			return nil, false, err
		}
	}
	// hasBody is what makes a row eligible to be inlined — and therefore to be
	// elided when the budget runs out.
	hasBody := func(r *taskRowJSON) bool {
		if r.ContextOnly {
			return false
		}
		return (includeNotes && len(byID[r.ID]) > 0) || (includeComments && commented[r.ID])
	}
	used := 0
	for i := range rows {
		row := &rows[i]
		if !hasBody(row) {
			continue
		}
		cost := 0
		if includeNotes {
			cost += len(byID[row.ID])
		}
		var comments []store.Comment
		if includeComments && commented[row.ID] {
			comments, err = s.ListComments(row.ID)
			if err != nil {
				return nil, false, err
			}
			for _, c := range comments {
				cost += len(c.Note)
			}
		}
		if used+cost > notesBudget {
			// This row and every later one keep has_notes/notes_len but get no
			// body; rows with nothing to inline are not "dropped" and so are
			// not named (skeletons among them — they never had a body).
			for j := i; j < len(rows); j++ {
				if !hasBody(&rows[j]) {
					continue
				}
				elided = append(elided, rows[j].ID)
			}
			return elided, true, nil
		}
		used += cost
		if includeNotes {
			row.Notes = byID[row.ID]
		}
		if len(comments) > 0 {
			row.Comments = commentsJSON(comments)
		}
	}
	return elided, false, nil
}

// commentsJSON converts store comments into the taskRowJSON Comment shape.
func commentsJSON(comments []store.Comment) []commentJSON {
	out := make([]commentJSON, 0, len(comments))
	for _, c := range comments {
		out = append(out, commentJSON{
			ID:        c.ID,
			Author:    c.Author,
			Note:      c.Note,
			CreatedAt: c.CreatedAt,
		})
	}
	return out
}
