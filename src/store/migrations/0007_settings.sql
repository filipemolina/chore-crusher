-- 0007_settings.sql: a small key/value table for app state.
--
-- Unlike config.yaml (user preferences, edited by hand), Setting holds state
-- the app itself writes: the id of the last active list, restored at the
-- next launch so the TUI reopens where the user left off (docs/DESIGN.md §7).
-- Values are TEXT; the store layer maps to Go types per key.
CREATE TABLE IF NOT EXISTS Setting (
    key   text primary key,
    value text not null
);
