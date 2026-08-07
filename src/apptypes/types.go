// Package apptypes holds the shapes components pass around: List, Task,
// Status, ProgressKind, and the list-item wrappers bubbles/list.Item needs.
// These are converted from src/store's row types at the store boundary —
// apptypes does not import database/sql. See docs/DESIGN.md §10.
package apptypes

import "github.com/filipemolina/chore-crusher/src/store"

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
	ID            string
	Name          string
	CreatedAt     int64
	Position      int
	Collaborative bool
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
		ID:            l.ID,
		Name:          l.Name,
		CreatedAt:     l.CreatedAt,
		Position:      l.Position,
		Collaborative: l.Collaborative,
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

// AgentActivity is a live claim that an agent is working on an entity.
// It mirrors store.AgentActivity — the TUI reads this from the store
// via the cmds layer and uses it to render a spinner on claimed rows
// (docs/plan/mcp-server-enhancement.md §3.4).
type AgentActivity struct {
	ID         string
	EntityType string // "task" | "list"
	EntityID   string
	AgentID    string
	Kind       string // "working" | "inspecting"
	AcquiredAt int64
}

// Comment is a task's append-only v1 comment thread (docs/plan/task-comments.md
// §1): a note authored by a user (OS username) or agent (CRUSH_AGENT
// identity), with no edit or delete in v1. It mirrors store.Comment — the
// conversion boundary keeps the TUI layer from depending on store's row shape.
type Comment struct {
	ID        string
	TaskID    string
	Author    string
	Note      string
	CreatedAt int64
}

// FromStoreComment converts one store.Comment into the component-facing shape.
func FromStoreComment(c store.Comment) Comment {
	return Comment{
		ID:        c.ID,
		TaskID:    c.TaskID,
		Author:    c.Author,
		Note:      c.Note,
		CreatedAt: c.CreatedAt,
	}
}

// FromStoreComments converts a slice of store comments in one pass.
func FromStoreComments(cs []store.Comment) []Comment {
	out := make([]Comment, len(cs))
	for i, c := range cs {
		out[i] = FromStoreComment(c)
	}
	return out
}

// FromStoreActivity converts one store.AgentActivity into the
// component-facing shape.
func FromStoreActivity(a store.AgentActivity) AgentActivity {
	return AgentActivity{
		ID:         a.ID,
		EntityType: a.EntityType,
		EntityID:   a.EntityID,
		AgentID:    a.AgentID,
		Kind:       string(a.Kind),
		AcquiredAt: a.AcquiredAt,
	}
}

// FromStoreActivities converts a slice of store activity rows in one pass.
func FromStoreActivities(as []store.AgentActivity) []AgentActivity {
	out := make([]AgentActivity, len(as))
	for i, a := range as {
		out[i] = FromStoreActivity(a)
	}
	return out
}
