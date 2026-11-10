-- P593: per-connection expensive-query threshold (estimated rows read); 0 = the app default.
-- A first-class column, not an options_json key: a safety limit must not be settable by pasting a URI.
ALTER TABLE connections ADD COLUMN explain_threshold_rows INTEGER NOT NULL DEFAULT 0 CHECK (explain_threshold_rows >= 0);
