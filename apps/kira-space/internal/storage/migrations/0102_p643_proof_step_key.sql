-- P643: proofs live in task folders. A step is identified by its id in proof.md (step_key), a session
-- belongs to the proof folder proofs/<slug>/ of its task link, and a pending edit proposes its own
-- name and summary. Existing steps get the id s<ordinal> the migration export writes.
ALTER TABLE proof_steps ADD COLUMN step_key TEXT NOT NULL DEFAULT '';
UPDATE proof_steps SET step_key = 's' || ordinal;
ALTER TABLE proof_task_links ADD COLUMN slug TEXT NOT NULL DEFAULT '';
ALTER TABLE proof_revisions ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE proof_revisions ADD COLUMN summary TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX proof_task_links_slug ON proof_task_links (task_id, slug) WHERE slug <> '';
