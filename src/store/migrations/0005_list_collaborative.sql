-- 0005_list_collaborative.sql — per-list opt-in flag letting any agent make
-- structural edits, not just the list's own created_by owner (see
-- src/mcpserver's requireWritable). Explicit opt-in: an untagged list stays
-- foreign to every agent unless this is set. Defaults to 0 (off) for every
-- existing and new list, the same shape comments_disabled uses in 0004.
ALTER TABLE List ADD COLUMN collaborative integer not null default 0;
