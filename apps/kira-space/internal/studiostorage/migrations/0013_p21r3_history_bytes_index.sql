-- P21 round 3 performance finding 11: repos/response_history.go's and repos/grpc_history.go's own
-- byte-budget guard runs `SELECT COALESCE(SUM(stored_bytes), 0) FROM ...` on every single
-- completed send/call, with a comment claiming it is "a cheap indexed aggregate" that "skips the
-- expensive sweep" — but neither table had an index on stored_bytes, so SQLite full-scanned the
-- table's own b-tree every time regardless. These indexes make the SUM an index-only scan: a
-- covering index over exactly the one column the aggregate reads, with no payload columns to make
-- it any larger than it has to be.
CREATE INDEX api_response_history_bytes ON api_response_history(stored_bytes);
CREATE INDEX grpc_call_history_bytes ON grpc_call_history(stored_bytes);
