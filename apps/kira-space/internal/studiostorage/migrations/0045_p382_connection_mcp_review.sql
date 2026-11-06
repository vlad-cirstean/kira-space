-- P382: per-connection result review on the DB MCP server's run_query path. 'inherit' follows the
-- app setting dbMcp.previewResponses, 'review' always holds, 'send' never holds. First-class column
-- rather than an options_json key for the 0022 reason: `options` round-trips through the URI, and a
-- safety gate must not be settable by pasting one. Default 'inherit' leaves every row's behaviour
-- unchanged.
ALTER TABLE connections ADD COLUMN mcp_review_results TEXT NOT NULL DEFAULT 'inherit';
