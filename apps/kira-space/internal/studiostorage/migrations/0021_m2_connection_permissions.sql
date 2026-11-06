-- M2: per-connection MCP metadata and per-operation permissions. Four plain columns with non-NULL
-- defaults, the 0004/0020 precedent — first-class rather than options_json keys for the same
-- reason those two give: `options` round-trips through the connection URI and the Copy URI menu
-- item, and a permission grant must not be settable, or clearable, by pasting a URI.
-- Defaults tighten what M1 shipped: an exposed connection could already be written to over MCP
-- (M1 §9's own stated honest state of it); after this migration a write prompts and DDL is
-- refused, on every existing row and every new one.
ALTER TABLE connections ADD COLUMN mcp_description TEXT NOT NULL DEFAULT '';
ALTER TABLE connections ADD COLUMN mcp_read_mode TEXT NOT NULL DEFAULT 'allow';
ALTER TABLE connections ADD COLUMN mcp_write_mode TEXT NOT NULL DEFAULT 'prompt';
ALTER TABLE connections ADD COLUMN mcp_ddl_mode TEXT NOT NULL DEFAULT 'deny';
