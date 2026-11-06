-- P97: repo-map is removed. Both the per-repository MCP grant (0019) and the settings leaf that
-- toggled the embedded server have no reader left.
ALTER TABLE code_repos DROP COLUMN mcp_enabled;
DELETE FROM settings WHERE key = 'codeIntel.mcpServerEnabled';
