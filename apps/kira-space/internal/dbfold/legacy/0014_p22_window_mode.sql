-- P22 D12: a window remembers which app mode (Studio/Api) it was closed in and reopens into it
-- (F20/F21). NOT NULL DEFAULT 'studio' rather than nullable — the same posture 0011/0012 already
-- took: every pre-existing window row lands on the app's own default mode (state/mode.ts's
-- defaultMode), and SQLite's ALTER TABLE ADD COLUMN with a non-null default is a metadata change,
-- not a table rewrite.
ALTER TABLE windows ADD COLUMN mode TEXT NOT NULL DEFAULT 'studio';
