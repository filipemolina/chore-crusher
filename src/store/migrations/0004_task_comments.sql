-- 0004_task_comments.sql — per-task comment thread.
--
-- Comments are append-only for v1 (no edit, no delete — see
-- docs/plan/task-comments.md §1). Each row is one comment on a task; the
-- author is the OS username (CLI/TUI path) or the MCP server's configured
-- identity (agent path), per that plan. ON DELETE CASCADE on task_id means
-- deleting a task drops its thread automatically.
CREATE TABLE IF NOT EXISTS TaskComment (
    id         text primary key,          -- ULID, generated in store.NewID
    task_id    text not null references Task(id) on delete cascade,
    author     text not null,             -- OS username (human) or CRUSH_AGENT identity (agent)
    note       text not null,
    created_at integer not null           -- unix seconds
);

CREATE INDEX IF NOT EXISTS idx_taskcomment_task ON TaskComment(task_id);

-- Per-list switch to turn comments off entirely. Integer 0/1 to match the
-- SQLite convention (no native BOOLEAN); store layer maps it to a Go bool.
-- Idempotent: re-running 0004 on an already-migrated file is a no-op on the
-- recorded schema_migrations row.
ALTER TABLE List ADD COLUMN comments_disabled integer not null default 0;
