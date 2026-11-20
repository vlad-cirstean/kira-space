-- P701: TUI task sessions open at quit or crash, offered once at the next start.
ALTER TABLE ade_sessions ADD COLUMN restore_pending INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ade_sessions ADD COLUMN restore_working INTEGER NOT NULL DEFAULT 0;
