-- 0005_list_collaborative.sql — per-list opt-in flag letting any agent make
-- structural edits, not just the list's own created_by owner. Explicit
-- opt-in: an untagged list stays foreign to every agent unless this is set.
-- The store is unenforced; the flag is advisory (a human may always
-- restructure their own list). Defaults to 0 (off) for every existing and
-- new list, the same shape comments_disabled uses in 0004.
ALTER TABLE List ADD COLUMN collaborative integer not null default 0;
