-- P731: links found in notes and handovers that the user dismissed; read filters them out.
CREATE TABLE ade_task_found_link_dismissals (
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  url TEXT NOT NULL CHECK (url <> ''),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  PRIMARY KEY (task_id, url)
) STRICT;
