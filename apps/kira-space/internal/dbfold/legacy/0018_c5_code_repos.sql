-- C5 §3.1: a repo entry is a parallel list with its own schema, never an extended `connections`
-- row (D1: a repository needs none of connections' sixteen fields but `name`). `repo_id` is
-- gitclient.Identify's own identity (internal/gitclient/repo.go) — UNIQUE so two spellings of one
-- checkout cannot be imported twice, matching how codeindex.db already scopes rows.
CREATE TABLE code_repos (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  root       TEXT NOT NULL,
  repo_id    TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX code_repos_repo ON code_repos (repo_id);

-- D2: NULL = studio/api (workspaceKeyOf's own `??` fallback derives the workspace from the tab's
-- kind, exactly as today); 'repo:<code_repos.id>' scopes a repo-workspace tab. `ADD COLUMN` with no
-- default rewrites no existing row — every tab already stored parses as workspace_id IS NULL.
ALTER TABLE tabs ADD COLUMN workspace_id TEXT;
