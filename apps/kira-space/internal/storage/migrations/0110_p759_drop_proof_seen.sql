-- P759: the proof ready popup is gone; nothing reads seen_at.
ALTER TABLE proof_sessions DROP COLUMN seen_at;
