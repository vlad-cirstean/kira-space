-- P28 §5.2: per-connection command rate limit. A plain non-REFERENCES column with a non-NULL
-- default, same precedent as 0004_p18_auto_explain.sql — no rebuild-and-swap dance needed. A
-- first-class column rather than an options_json key: `options` round-trips through the
-- connection URI and the Copy URI menu item, and a safety limit must not be removable by pasting
-- a URI (stronger than auto-explain's own reasoning, which is only about a limit being switched
-- ON unexpectedly — here it could be switched OFF unexpectedly).
ALTER TABLE connections ADD COLUMN throttle_per_sec REAL NOT NULL DEFAULT 0;
