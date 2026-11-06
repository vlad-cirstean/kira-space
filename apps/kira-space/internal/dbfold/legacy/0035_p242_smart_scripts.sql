-- P242 Part 2: smart scripts (headless Claude), their params and tool allowlist, run details and logs.
ALTER TABLE custom_scripts ADD COLUMN kind TEXT NOT NULL DEFAULT 'script' CHECK (kind IN ('script', 'smart'));
ALTER TABLE custom_scripts ADD COLUMN params_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE custom_scripts ADD COLUMN smart_json TEXT NOT NULL DEFAULT '';

ALTER TABLE script_runs ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE script_runs ADD COLUMN session_id TEXT NOT NULL DEFAULT '';
-- The exact prompt sent; a secret param is never in it.
ALTER TABLE script_runs ADD COLUMN prompt TEXT NOT NULL DEFAULT '';
-- [{name, value}] for display; a secret value is stored as the mask.
ALTER TABLE script_runs ADD COLUMN params_json TEXT NOT NULL DEFAULT '[]';
-- {tools, allowedTools, mcpServers} the run was launched with.
ALTER TABLE script_runs ADD COLUMN tools_json TEXT NOT NULL DEFAULT '{}';

CREATE TABLE script_run_logs (
  run_id TEXT NOT NULL,
  seq    INTEGER NOT NULL,
  stream TEXT NOT NULL CHECK (stream IN ('stdout', 'stderr', 'event')),
  text   TEXT NOT NULL,
  PRIMARY KEY (run_id, seq)
);
