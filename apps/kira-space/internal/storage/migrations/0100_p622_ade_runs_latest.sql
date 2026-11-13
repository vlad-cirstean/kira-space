-- P622: LatestRunsOf's correlated MAX(attempt) subquery looks up (task, step, branch, purpose).
CREATE INDEX IF NOT EXISTS ade_runs_latest ON ade_runs (task_id, step_id, branch_id, purpose, attempt);
