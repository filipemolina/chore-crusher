package store

import (
	"fmt"
	"time"
)

// ExportVersion is the current export document format version. Bump it when
// the document shape changes so old files can be rejected or migrated.
const ExportVersion = 1

// ExportDocument is the top-level export/import payload. It is versioned so a
// future format change is detected rather than mis-parsed.
type ExportDocument struct {
	Version int          `json:"version"`
	Lists   []ExportList `json:"lists"`
}

// ExportList is one list with all of its tasks (and their comments /
// attachments) — the same preorder grouping ListTasks returns.
type ExportList struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	CreatedBy     string       `json:"created_by"`
	Collaborative bool         `json:"collaborative"`
	CreatedAt     int64        `json:"created_at"`
	Position      int          `json:"position"`
	ArchivedAt    *int64       `json:"archived_at,omitempty"`
	Tasks         []ExportTask `json:"tasks"`
}

// ExportTask carries every Task column (§2) plus the comment and attachment
// rows the task owns. ParentID references the *exported* task id; import
// rewrites it through its old->new id map.
type ExportTask struct {
	ID           string             `json:"id"`
	ParentID     *string            `json:"parent_id"`
	Title        string             `json:"title"`
	Notes        string             `json:"notes"`
	Status       string             `json:"status"`
	ProgressKind string             `json:"progress_kind"`
	ProgressPct  *int               `json:"progress_pct"`
	Position     int                `json:"position"`
	CreatedAt    int64              `json:"created_at"`
	UpdatedAt    int64              `json:"updated_at"`
	CompletedAt  *int64             `json:"completed_at"`
	Assignee     string             `json:"assignee"`
	AssignedAt   *int64             `json:"assigned_at"`
	Priority     string             `json:"priority"`
	Comments     []ExportComment    `json:"comments"`
	Attachments  []ExportAttachment `json:"attachments"`
}

type ExportComment struct {
	Author    string `json:"author"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"created_at"`
}

type ExportAttachment struct {
	Path      string `json:"path"`
	CreatedAt int64  `json:"created_at"`
}

// Export builds an ExportDocument. When listID is nil, every list in the
// store is included; when non-nil, only that list. Rows within a list come
// back in ListTasks' depth-first preorder, so every parent precedes its
// children and ImportList can recreate them in the same order.
func (s *Store) Export(listID *string) (ExportDocument, error) {
	doc := ExportDocument{Version: ExportVersion}
	var lists []ListSummary
	if listID == nil {
		// A whole-store export must capture archived lists too — this is
		// the explicit include-archived path, kept separate from ListLists'
		// default exclusion so archiving a list never loses its data on
		// export (docs/DESIGN.md §2).
		all, err := s.ListAllLists()
		if err != nil {
			return doc, err
		}
		lists = all
	} else {
		l, err := s.GetList(*listID)
		if err != nil {
			return doc, err
		}
		lists = []ListSummary{{List: l}}
	}
	for _, ls := range lists {
		el, err := s.exportList(ls.ID)
		if err != nil {
			return doc, err
		}
		doc.Lists = append(doc.Lists, el)
	}
	return doc, nil
}

func (s *Store) exportList(id string) (ExportList, error) {
	l, err := s.GetList(id)
	if err != nil {
		return ExportList{}, err
	}
	tasks, err := s.ListTasks(id)
	if err != nil {
		return ExportList{}, err
	}
	el := ExportList{
		ID:            l.ID,
		Name:          l.Name,
		CreatedBy:     l.CreatedBy,
		Collaborative: l.Collaborative,
		CreatedAt:     l.CreatedAt,
		Position:      l.Position,
		ArchivedAt:    l.ArchivedAt,
	}
	for _, t := range tasks {
		et := ExportTask{
			ID:           t.ID,
			ParentID:     t.ParentID,
			Title:        t.Title,
			Notes:        t.Notes,
			Status:       string(t.Status),
			ProgressKind: string(t.ProgressKind),
			ProgressPct:  t.ProgressPct,
			Position:     t.Position,
			CreatedAt:    t.CreatedAt,
			UpdatedAt:    t.UpdatedAt,
			CompletedAt:  t.CompletedAt,
			Assignee:     t.Assignee,
			AssignedAt:   t.AssignedAt,
			Priority:     string(t.Priority),
		}
		comments, err := s.ListComments(t.ID)
		if err != nil {
			return ExportList{}, err
		}
		for _, c := range comments {
			et.Comments = append(et.Comments, ExportComment{
				Author: c.Author, Note: c.Note, CreatedAt: c.CreatedAt,
			})
		}
		atts, err := s.ListAttachments(t.ID)
		if err != nil {
			return ExportList{}, err
		}
		for _, a := range atts {
			et.Attachments = append(et.Attachments, ExportAttachment{
				Path: a.Path, CreatedAt: a.CreatedAt,
			})
		}
		el.Tasks = append(el.Tasks, et)
	}
	return el, nil
}

// ImportList recreates one exported list and all of its tasks (with comments
// and attachments) into the store. IDs are regenerated — ULIDs must stay
// unique per database — and parent_id links are rewritten through an
// old->new map. Because the source tasks arrive in preorder (parent before
// child), each parent exists in the map before its child is created. The
// list's created_by and collaborative flag are restored as data.
func (s *Store) ImportList(el ExportList) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	newListID := NewID()
	now := time.Now().Unix()
	if _, err := tx.Exec(
		`INSERT INTO List (id, name, created_at, position, created_by, comments_disabled, collaborative, archived_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		newListID, el.Name, now, el.Position, el.CreatedBy, boolToInt(el.Collaborative), el.ArchivedAt,
	); err != nil {
		return err
	}

	idMap := make(map[string]string, len(el.Tasks))
	for _, et := range el.Tasks {
		var newParent *string
		if et.ParentID != nil {
			mapped, ok := idMap[*et.ParentID]
			if !ok {
				return fmt.Errorf("import: parent %q of task %q not seen yet (export order is not preorder?)", *et.ParentID, et.Title)
			}
			newParent = &mapped
		}
		newTaskID := NewID()
		status := et.Status
		if status == "" {
			status = "pending"
		}
		kind := et.ProgressKind
		if kind == "" {
			kind = "none"
		}
		prio := et.Priority
		if prio == "" {
			prio = "none"
		}
		// Write every column verbatim (status/progress/assignment as stored),
		// bypassing the state-machine mutators — this is a faithful restore,
		// not a fresh write that should recompute derived progress.
		if _, err := tx.Exec(
			`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind,
			                    progress_pct, position, created_at, updated_at, completed_at,
			                    assignee, assigned_at, priority)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			newTaskID, newListID, newParent, et.Title, et.Notes,
			status, kind, et.ProgressPct, et.Position,
			et.CreatedAt, et.UpdatedAt, et.CompletedAt,
			et.Assignee, et.AssignedAt, prio,
		); err != nil {
			return err
		}
		idMap[et.ID] = newTaskID

		for _, c := range et.Comments {
			if _, err := tx.Exec(
				`INSERT INTO TaskComment (id, task_id, author, note, created_at) VALUES (?, ?, ?, ?, ?)`,
				NewID(), newTaskID, c.Author, c.Note, c.CreatedAt,
			); err != nil {
				return err
			}
		}
		for _, a := range et.Attachments {
			if _, err := tx.Exec(
				`INSERT INTO TaskAttachment (id, task_id, path, created_at) VALUES (?, ?, ?, ?)`,
				NewID(), newTaskID, a.Path, a.CreatedAt,
			); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
