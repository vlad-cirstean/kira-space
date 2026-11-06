-- P529: per-connection statement timeout for the DB MCP server's run_query path, in seconds, 0 off.
-- First-class column rather than an options_json key for the 0045 reason: `options` round-trips
-- through the URI, and a safety limit must not be settable by pasting one. Default 10 gives every
-- existing connection the timeout.
ALTER TABLE connections ADD COLUMN mcp_statement_timeout_sec INTEGER NOT NULL DEFAULT 10;
