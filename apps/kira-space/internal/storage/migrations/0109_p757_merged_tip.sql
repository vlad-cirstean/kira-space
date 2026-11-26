-- P757: a merge into main is recorded at one branch tip and holds only at that tip.
ALTER TABLE ade_task_branches ADD COLUMN merged_tip TEXT NOT NULL DEFAULT '';

-- A live merged_at recorded before P757 has no tip. It stays: the next board build re-validates it
-- against the current tip and records it at that tip, or clears it when it no longer holds.

-- recount_commits marks a stacked or non-main-based row whose had_commits was counted against main
-- and may hold its base's commits. The next board build recounts it against the resolved base and
-- clears the mark. Main is stored as base ''.
ALTER TABLE ade_task_branches ADD COLUMN recount_commits INTEGER NOT NULL DEFAULT 0;
UPDATE ade_task_branches SET recount_commits = 1
WHERE archived_at IS NULL AND had_commits = 1 AND (base_branch_id <> '' OR base <> '');
