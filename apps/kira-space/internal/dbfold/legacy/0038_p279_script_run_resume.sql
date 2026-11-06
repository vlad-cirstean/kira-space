-- P279: a smart run a restart ended can resume its Claude session; a resumed run links to its origin.
ALTER TABLE script_runs ADD COLUMN resume_id TEXT NOT NULL DEFAULT '';
ALTER TABLE script_runs ADD COLUMN resumed_from TEXT NOT NULL DEFAULT '';
