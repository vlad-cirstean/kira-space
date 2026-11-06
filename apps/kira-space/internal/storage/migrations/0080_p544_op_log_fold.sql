-- P544: the database op log joins op_log. Studio's rows keep their connection, tab, row count, path and
-- truncation flag; db_op_log goes away.
ALTER TABLE op_log ADD COLUMN connection_id TEXT REFERENCES connections(id) ON DELETE SET NULL;
ALTER TABLE op_log ADD COLUMN tab_id TEXT;
ALTER TABLE op_log ADD COLUMN rows INTEGER;
ALTER TABLE op_log ADD COLUMN command_truncated INTEGER NOT NULL DEFAULT 0;
ALTER TABLE op_log ADD COLUMN path TEXT;
CREATE INDEX op_log_connection ON op_log (connection_id);

INSERT OR IGNORE INTO op_log (id, started_ms, duration_ms, category, kind, status, module, trigger_, source, subject,
    repo_root, repo_name, task_id, parent_id, command, error, steps_json, steps_dropped, background, repeat_count,
    connection_id, tab_id, rows, command_truncated, path)
SELECT d.id, COALESCE(CAST(unixepoch(d.started_at, 'subsec') * 1000 AS INTEGER), 0), d.duration_ms, 'db', d.kind, d.status,
    'database', d.source, d.source, COALESCE(c.name, ''), '', '', '', '', d.command, d.error, '[]', 0, 0, 1,
    d.connection_id, d.tab_id, d.rows, d.command_truncated, d.path
FROM db_op_log d LEFT JOIN connections c ON c.id = d.connection_id;

DROP TABLE db_op_log;
