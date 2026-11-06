-- P440: per-cluster MCP description and per-operation modes (deny | prompt | allow), 0021's shape.
ALTER TABLE kube_clusters ADD COLUMN mcp_description TEXT NOT NULL DEFAULT '';
ALTER TABLE kube_clusters ADD COLUMN mcp_discover_mode TEXT NOT NULL DEFAULT 'prompt';
ALTER TABLE kube_clusters ADD COLUMN mcp_list_mode TEXT NOT NULL DEFAULT 'prompt';
ALTER TABLE kube_clusters ADD COLUMN mcp_describe_mode TEXT NOT NULL DEFAULT 'prompt';
ALTER TABLE kube_clusters ADD COLUMN mcp_logs_mode TEXT NOT NULL DEFAULT 'prompt';
