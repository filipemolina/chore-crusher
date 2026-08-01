// Package apptypes holds the shapes components pass around: List, Task,
// Status, ProgressKind, and the list-item wrappers bubbles/list.Item needs.
// These are converted from src/store's row types at the store boundary —
// apptypes does not import database/sql. See docs/DESIGN.md §10.
package apptypes

import "github.com/filipemolina/chore-completer/src/store"

// Status is a task's lifecycle state, mirroring store.Status. The values are
// the literal strings stored in the database; docs/DESIGN.md §3 is the
// authority on which transitions are allowed.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusComplete   Status = "complete"
)

// ProgressKind describes how an in-progress task reports progress, mirroring
// store.ProgressKind. It only has meaning while Status is StatusInProgress;
// the store keeps it 'none' for pending and complete tasks.
type ProgressKind string

const (
	ProgressNone       ProgressKind = "none"
	ProgressSimple     ProgressKind = "simple"
	ProgressSubtasks   ProgressKind = "subtasks"
	ProgressPercentage ProgressKind = "percentage"
)

// Task is a task the components render, mirroring store.Task's row shape.
// ParentID is nil for a root-level task; ProgressPct is set only when
// ProgressKind is ProgressPercentage; CompletedAt is set only when Status is
// StatusComplete. Components never hold store.Task — FromStore converts at
// the boundary, so the TUI layer cannot come to depend on store's SQL-flavored
// reading of a row (docs/DESIGN.md §10).
type Task struct {
	ID           string
	ListID       string
	ParentID     *string
	Title        string
	Notes        string
	Status       Status
	ProgressKind ProgressKind
	ProgressPct  *int
	Position     int
	CreatedAt    int64
	UpdatedAt    int64
	CompletedAt  *int64
}

// List is a list the components render, mirroring store.List.
type List struct {
	ID        string
	Name      string
	CreatedAt int64
	Position  int
}

// ListSummary is a List plus its task counts, mirroring store.ListSummary.
// PendingCount counts every task whose status is not complete (pending and
// in-progress alike); CompleteCount counts status = complete.
type ListSummary struct {
	List          List
	PendingCount  int
	CompleteCount int
}

// FilterValue satisfies list.Item.
func (l ListSummary) FilterValue() string { return l.List.Name }

// FromStore converts one store.Task into the component-facing shape. The
// conversion is a function, not a type alias, so a field added to one side
// cannot silently leak to the other.
func FromStore(t store.Task) Task {
	return Task{
		ID:           t.ID,
		ListID:       t.ListID,
		ParentID:     t.ParentID,
		Title:        t.Title,
		Notes:        t.Notes,
		Status:       Status(t.Status),
		ProgressKind: ProgressKind(t.ProgressKind),
		ProgressPct:  t.ProgressPct,
		Position:     t.Position,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
		CompletedAt:  t.CompletedAt,
	}
}

// FromStoreTasks converts a slice of store rows in one pass.
func FromStoreTasks(ts []store.Task) []Task {
	out := make([]Task, len(ts))
	for i, t := range ts {
		out[i] = FromStore(t)
	}
	return out
}

// FromStoreList converts one store.List into the component-facing shape.
func FromStoreList(l store.List) List {
	return List{
		ID:        l.ID,
		Name:      l.Name,
		CreatedAt: l.CreatedAt,
		Position:  l.Position,
	}
}

// FromStoreLists converts store list summaries in one pass.
func FromStoreLists(ls []store.ListSummary) []ListSummary {
	out := make([]ListSummary, len(ls))
	for i, l := range ls {
		out[i] = ListSummary{
			List:          FromStoreList(l.List),
			PendingCount:  l.PendingCount,
			CompleteCount: l.CompleteCount,
		}
	}
	return out
}
