-- P535: masked-read permission for browse_table. Projection of the connection file's mcp.masked, which stays the
-- source; inherit follows mcp_read_mode. mcp_review_masked is the result review of browse_table (mcp.review.masked),
-- backfilled from the legacy single value like the other modes.
ALTER TABLE connections ADD COLUMN mcp_masked_mode TEXT NOT NULL DEFAULT 'inherit';
ALTER TABLE connections ADD COLUMN mcp_review_masked TEXT NOT NULL DEFAULT 'inherit';
UPDATE connections SET mcp_review_masked = mcp_review_results;
