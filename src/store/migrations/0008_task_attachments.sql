-- 0008_task_attachments.sql — adds file attachments to tasks.
--
-- TaskAttachment stores file paths (screenshots, documents, etc.) associated
-- with tasks. The path is stored as-is; the UI displays them as clickable
-- references. No file content is stored in the database.
--
-- Attachments are ordered by creation time (created_at) within each task.
-- Deleting a task cascades to its attachments via the foreign key.

CREATE TABLE IF NOT EXISTS TaskAttachment (
    id         text primary key,          -- ULID, generated in store.NewID
    task_id    text not null references Task(id) on delete cascade,
    path       text not null,             -- file path (absolute or relative)
    created_at integer not null           -- unix seconds
);

CREATE INDEX IF NOT EXISTS idx_task_attachment_task ON TaskAttachment(task_id);
