-- P706: mergeable and auto-merge on the change request index, for the one PR glyph.
ALTER TABLE git_host_change_requests ADD COLUMN mergeable TEXT NOT NULL DEFAULT '';
ALTER TABLE git_host_change_requests ADD COLUMN auto_merge INTEGER NOT NULL DEFAULT 0 CHECK (auto_merge IN (0, 1));
