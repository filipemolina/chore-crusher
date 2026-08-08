-- 0001_init.sql — the initial schema: the migration tracker, lists, and tasks.
--
-- schema_migrations records which numbered migrations have been applied; the
-- version column holds the numeric prefix of the applied file's name. It is
-- the first statement here so a brand-new database and a migrated one are
-- distinguishable.
--
-- CREATE TABLE ... IF NOT EXISTS makes concurrent first-run safe: when the
-- TUI and a CLI invocation open a brand-new file at the same time, the second
-- migration pass no-ops on the tables the first one just created and sees
-- version 1 already recorded. store.Open's migration runner is written
-- against that property.
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    integer primary key,
    applied_at integer not null
);

CREATE TABLE IF NOT EXISTS List (
    id         text primary key,          -- ULID, generated in store.NewID
    name       text not null,
    created_at integer not null,          -- unix seconds
    position   integer not null           -- manual ordering among lists
);

CREATE TABLE IF NOT EXISTS Task (
    id            text primary key,       -- ULID
    list_id       text not null references List(id) on delete cascade,
    parent_id     text references Task(id) on delete cascade,  -- null = root-level task
    title         text not null,
    notes         text not null default '',
    status        text not null default 'pending',      -- pending | in_progress | complete
    progress_kind text not null default 'none',         -- none | simple | subtasks | percentage
    progress_pct  integer,                              -- 0-100, only when progress_kind='percentage'
    position      integer not null,                     -- manual ordering among siblings
    created_at    integer not null,
    updated_at    integer not null,
    completed_at  integer                               -- null unless status='complete'
);

-- The tree read (ListTasks) and the ancestor walk (recomputeAncestors) both
-- filter on these; without the indexes SQLite scans every row per query.
CREATE INDEX IF NOT EXISTS idx_task_list ON Task(list_id);
CREATE INDEX IF NOT EXISTS idx_task_parent ON Task(parent_id);
