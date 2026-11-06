-- P242 Part 3: scripts in ADE. A script may run in a task's worktree; a run remembers its task and branch.
ALTER TABLE custom_scripts ADD COLUMN use_ade_dir INTEGER NOT NULL DEFAULT 1 CHECK (use_ade_dir IN (0, 1));
ALTER TABLE script_runs ADD COLUMN task_id TEXT NOT NULL DEFAULT '';
ALTER TABLE script_runs ADD COLUMN task_title TEXT NOT NULL DEFAULT '';
ALTER TABLE script_runs ADD COLUMN branch_id TEXT NOT NULL DEFAULT '';
ALTER TABLE script_runs ADD COLUMN branch_label TEXT NOT NULL DEFAULT '';
CREATE INDEX script_runs_task ON script_runs (task_id, created_at DESC);
