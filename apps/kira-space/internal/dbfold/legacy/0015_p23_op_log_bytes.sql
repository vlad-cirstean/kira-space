-- P23 D3: op_log gains what api_response_history/grpc_call_history already have — a per-row
-- stored_bytes so OpsRepo.Prune can enforce a table-wide byte budget (D2) behind an index-only
-- SUM, the same shape 0013_p21r3_history_bytes_index.sql gave those two tables — plus a
-- command_truncated flag so OperationsPanel.vue can refuse to Re-run a command it knows is
-- incomplete (D1(c)). Both ADD COLUMNs carry a non-NULL default, so this is a metadata change,
-- not a table rewrite (the same property 0011/0012/0014 each record).
ALTER TABLE op_log ADD COLUMN stored_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE op_log ADD COLUMN command_truncated INTEGER NOT NULL DEFAULT 0;

-- Byte-counted via CAST(... AS BLOB), not length()'s character count, so a non-ASCII command is
-- not under-counted (D3).
UPDATE op_log SET stored_bytes = length(CAST(coalesce(command, '') AS BLOB))
                               + length(CAST(coalesce(error, '')   AS BLOB));

-- Rows written before this migration have no per-row cap. A single oversized one would break
-- D2's sweep invariant ("no single row can exceed the budget") on the very first prune, emptying
-- the whole table. Deleting them is one statement with no rune-boundary hazard; truncating them
-- in SQL instead would need a byte-exact substr over a BLOB cast for rows that are by definition
-- the pathological ones nobody wants. The app has not shipped, so the only databases this can
-- touch are developers' own. 73728 = maxOpCommandBytes (65536) + maxOpErrorBytes (8192).
DELETE FROM op_log WHERE stored_bytes > 73728;

CREATE INDEX op_log_bytes ON op_log(stored_bytes);
