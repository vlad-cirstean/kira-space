-- P717: per-cluster review of what list, describe and logs return. inherit follows the app setting.
ALTER TABLE kube_clusters ADD COLUMN mcp_review TEXT NOT NULL DEFAULT 'inherit' CHECK (mcp_review IN ('inherit', 'review', 'send'));
