-- P328: who started the op, 'user' (the UI) or 'mcp' (an MCP client). Older rows predate MCP
-- tool-driven ops being tagged, so they read as 'user'.
ALTER TABLE op_log ADD COLUMN source TEXT NOT NULL DEFAULT 'user';
