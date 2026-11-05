-- P532: per-connection table allow list on the DB MCP server. NULL = all tables (every existing row);
-- otherwise a JSON array of tableref entries. Projection of the connection file's mcp.tables, which stays
-- the source. First-class column rather than an options_json key for the 0022 reason: a safety gate must
-- not be settable through the URI.
ALTER TABLE connections ADD COLUMN mcp_tables_json TEXT;
