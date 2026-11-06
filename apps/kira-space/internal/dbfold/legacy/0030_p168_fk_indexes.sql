-- P168 Part 2: SQLite resolves ON DELETE CASCADE / SET NULL by scanning a child table unless its
-- FK column has an index. Deleting a collection or folder scanned api_items, api_response_history
-- and grpc_call_history once per deleted item; a connection delete scanned op_log.
CREATE INDEX api_items_parent ON api_items(parent_id);
CREATE INDEX api_response_history_item ON api_response_history(item_id);
CREATE INDEX grpc_call_history_item ON grpc_call_history(item_id);
CREATE INDEX op_log_connection ON op_log(connection_id);
