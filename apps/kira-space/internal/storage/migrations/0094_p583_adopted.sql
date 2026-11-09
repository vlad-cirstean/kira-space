-- P583: a second task can work on a branch another live task row already names (adopted = 1).
-- The unique live-name index now covers only non-adopted rows, so each name keeps exactly one owner row.
ALTER TABLE ade_task_branches ADD COLUMN adopted INTEGER NOT NULL DEFAULT 0 CHECK (adopted IN (0, 1));

DROP INDEX ade_task_branches_live_name;

CREATE UNIQUE INDEX ade_task_branches_live_name ON ade_task_branches (code_repo_id, name) WHERE name <> '' AND archived_at IS NULL AND adopted = 0;
