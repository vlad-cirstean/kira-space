-- M1 §6.1: per-connection MCP exposure, deny by default. A plain non-REFERENCES column with a
-- non-NULL default, the 0004/0005 precedent — no rebuild-and-swap dance needed. First-class rather
-- than an options_json key for the same reason as those two: `options` round-trips through the
-- connection URI and the Copy URI menu item, and an access grant must not be settable — or
-- clearable — by pasting a URI.
ALTER TABLE connections ADD COLUMN mcp_enabled INTEGER NOT NULL DEFAULT 0;
