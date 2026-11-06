-- P316: script runs remember the agent provider that ran them.
ALTER TABLE script_runs ADD COLUMN provider TEXT NOT NULL DEFAULT 'claude';
