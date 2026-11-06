-- P441: a run a quit or crash ended is offered in the Runs banner after the next start. Set with the outcome source
-- `restart`; the next boot takes the marks into its in-memory offer and clears them.
ALTER TABLE script_runs ADD COLUMN restore_pending INTEGER NOT NULL DEFAULT 0;
