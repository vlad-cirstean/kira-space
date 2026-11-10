-- P603: one MCP switch and one approval mode per cluster, like scripts. Never widens access:
-- a cluster turns on only if some operation was prompt or allow; it runs without asking only if every
-- operation was allow. Any other stored word counts as deny.
ALTER TABLE kube_clusters ADD COLUMN mcp_enabled INTEGER NOT NULL DEFAULT 0 CHECK (mcp_enabled IN (0, 1));
ALTER TABLE kube_clusters ADD COLUMN mcp_approval TEXT NOT NULL DEFAULT 'prompt' CHECK (mcp_approval IN ('prompt', 'allow'));
UPDATE kube_clusters SET
  mcp_enabled = CASE WHEN mcp_discover_mode IN ('prompt', 'allow') OR mcp_list_mode IN ('prompt', 'allow')
    OR mcp_describe_mode IN ('prompt', 'allow') OR mcp_logs_mode IN ('prompt', 'allow') THEN 1 ELSE 0 END,
  mcp_approval = CASE WHEN mcp_discover_mode = 'allow' AND mcp_list_mode = 'allow'
    AND mcp_describe_mode = 'allow' AND mcp_logs_mode = 'allow' THEN 'allow' ELSE 'prompt' END;
ALTER TABLE kube_clusters DROP COLUMN mcp_discover_mode;
ALTER TABLE kube_clusters DROP COLUMN mcp_list_mode;
ALTER TABLE kube_clusters DROP COLUMN mcp_describe_mode;
ALTER TABLE kube_clusters DROP COLUMN mcp_logs_mode;
