-- P534: column review for masking. Projection of the connection file's mask_review, which stays the source.
-- mask_review_on = 0 for every existing row: no review, nothing withheld. connection_mask_reviewed holds the
-- columns the user confirmed as reviewed, as tableref column entries; a table because the list can hold
-- tens of thousands of entries and the summary rides every connections:changed push.
ALTER TABLE connections ADD COLUMN mask_review_on INTEGER NOT NULL DEFAULT 0;
CREATE TABLE connection_mask_reviewed (
  connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  column_ref    TEXT NOT NULL,
  PRIMARY KEY (connection_id, column_ref)
);
