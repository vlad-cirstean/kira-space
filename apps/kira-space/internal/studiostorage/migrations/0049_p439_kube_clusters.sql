-- P439: one row per (kubeconfig context, server URL). Rows outlive the context in the kubeconfig.
CREATE TABLE kube_clusters (
  id TEXT PRIMARY KEY,
  context TEXT NOT NULL,
  server TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL,
  UNIQUE (context, server)
);
