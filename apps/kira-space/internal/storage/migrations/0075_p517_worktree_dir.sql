-- P517: reserved worktree folder name per branch row (wt_<id>), independent of the branch name.
ALTER TABLE ade_task_branches ADD COLUMN worktree_dir TEXT NOT NULL DEFAULT '';
