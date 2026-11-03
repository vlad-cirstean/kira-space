-- P485: user order of the cluster list. NULL until the user first reorders; NULL rows sort last.
ALTER TABLE kube_clusters ADD COLUMN sort_order INTEGER;
