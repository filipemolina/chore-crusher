-- 0009_list_archived.sql — lets a list be archived instead of deleted.
--
-- archived_at is a nullable timestamp, not a boolean: it doubles as a
-- sort-by-archive-date key for the archive page, at no extra cost over a
-- flag. NULL (the default for every existing and new list) means active.
ALTER TABLE List ADD COLUMN archived_at integer;
