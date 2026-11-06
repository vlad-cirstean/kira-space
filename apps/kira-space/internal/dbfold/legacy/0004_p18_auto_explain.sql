-- P18 (v1.1) D18: per-connection auto-explain toggle. A plain non-REFERENCES column with a
-- non-NULL default, so the ADD COLUMN restriction 0002_p8_windows.sql's own comment records
-- (SQLite refuses a REFERENCES column with a non-NULL default) doesn't apply here — no
-- rebuild-and-swap dance needed. A first-class column rather than an options_json key, mirroring
-- `preconnect`'s own precedent: `options` round-trips through the connection URI and the Copy URI
-- menu item, and a behaviour that issues an extra EXPLAIN per run must not be switchable on by
-- pasting a URI.
ALTER TABLE connections ADD COLUMN auto_explain INTEGER NOT NULL DEFAULT 0;
