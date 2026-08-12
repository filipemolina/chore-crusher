package store

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
