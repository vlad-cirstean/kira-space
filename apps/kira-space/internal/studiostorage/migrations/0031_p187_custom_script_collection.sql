-- P187: quick commands group under a free-text collection; '' = ungrouped.
ALTER TABLE custom_scripts ADD COLUMN collection TEXT NOT NULL DEFAULT '';
