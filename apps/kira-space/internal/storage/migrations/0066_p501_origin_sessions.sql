-- P501: the Kira session that wrote a handover or reported an artifact. '' = user, detected or unknown (old rows).
ALTER TABLE ade_task_handovers ADD COLUMN session_id TEXT NOT NULL DEFAULT '';
ALTER TABLE ade_task_artifacts ADD COLUMN session_id TEXT NOT NULL DEFAULT '';
