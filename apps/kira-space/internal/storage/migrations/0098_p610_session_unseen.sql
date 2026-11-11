-- P610: finished-not-viewed marker for an ADE session; 0 means seen.
ALTER TABLE ade_sessions ADD COLUMN unseen_at INTEGER NOT NULL DEFAULT 0 CHECK (unseen_at >= 0);
