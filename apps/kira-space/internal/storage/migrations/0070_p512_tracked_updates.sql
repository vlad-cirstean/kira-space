-- P512: tracked-item update snapshots, and the comment count and assignees of the change request index.
CREATE TABLE ade_tracked_items (
    task_id      TEXT    NOT NULL,
    item_key     TEXT    NOT NULL,
    kind         TEXT    NOT NULL,
    seen_json    TEXT    NOT NULL,
    current_json TEXT    NOT NULL,
    seen_at      INTEGER NOT NULL,
    observed_at  INTEGER NOT NULL,
    changed_at   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (task_id, item_key)
);
ALTER TABLE git_host_change_requests ADD COLUMN comment_count INTEGER;
ALTER TABLE git_host_change_requests ADD COLUMN assignees TEXT;
