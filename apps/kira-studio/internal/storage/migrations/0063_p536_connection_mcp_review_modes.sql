-- P536: result review per MCP mode. Projection of the connection file's mcp.review (falling back to
-- review_results), which stays the source. Backfilled from mcp_review_results so no row changes meaning.
ALTER TABLE connections ADD COLUMN mcp_review_read TEXT NOT NULL DEFAULT 'inherit';
ALTER TABLE connections ADD COLUMN mcp_review_write TEXT NOT NULL DEFAULT 'inherit';
ALTER TABLE connections ADD COLUMN mcp_review_ddl TEXT NOT NULL DEFAULT 'inherit';
UPDATE connections SET mcp_review_read = mcp_review_results, mcp_review_write = mcp_review_results,
  mcp_review_ddl = mcp_review_results;
