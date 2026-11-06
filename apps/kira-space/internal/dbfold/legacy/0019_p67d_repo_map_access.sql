-- P67d: per-repository MCP access. 0 = not shared (the default for every existing row, and for
-- every future import) — nothing is exposed to an MCP client until a user grants it explicitly.
ALTER TABLE code_repos ADD COLUMN mcp_enabled INTEGER NOT NULL DEFAULT 0;
