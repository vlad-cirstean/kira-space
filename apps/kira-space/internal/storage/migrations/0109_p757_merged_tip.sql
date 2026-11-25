-- P757: a merge into main is recorded at one branch tip and holds only at that tip.
ALTER TABLE ade_task_branches ADD COLUMN merged_tip TEXT NOT NULL DEFAULT '';

-- Live merges recorded before P757 have no tip. Forget them: the next board build re-records
-- ancestor merges, the next Refresh re-records PR merges.
UPDATE ade_task_branches SET merged_at = NULL
WHERE archived_at IS NULL AND merged_at IS NOT NULL;

-- had_commits of a stacked or non-main-based row was counted against main and may hold its base's
-- commits. Recount from the resolved base on the next build. Main is stored as base ''.
UPDATE ade_task_branches SET had_commits = 0
WHERE archived_at IS NULL AND had_commits = 1 AND (base_branch_id <> '' OR base <> '');
