-- P12 D14: the six collection/variable/history tables are renamed http_* -> api_*, because
-- http_items has stored gRPC requests since 0009_p11_grpc.sql's own `protocol` column, and
-- http_variables resolves a gRPC call's metadata/target too (internal/apivars) -- the name is not
-- stale, it is wrong. grpc_call_history is NOT renamed: it holds gRPC calls only, and is correct
-- as it stands.
--
-- Safe (F17): no views or triggers reference these tables (grep across every migration file
-- confirms it), and db.go sets _foreign_keys=1 on every pooled connection, which is the condition
-- under which SQLite rewrites every OTHER table's own REFERENCES clause to follow a rename --
-- grpc_call_history.item_id, api_response_history.item_id, api_items.parent_id (self-referencing)
-- and api_variables' two owner columns all still point at the right table after this runs, with no
-- explicit rewrite needed here. http_response_history.scope_key's GENERATED ALWAYS AS expression
-- references only its own row's columns, so it is unaffected by the table's own rename.
ALTER TABLE http_collections      RENAME TO api_collections;
ALTER TABLE http_items            RENAME TO api_items;
ALTER TABLE http_environments     RENAME TO api_environments;
ALTER TABLE http_variables        RENAME TO api_variables;
ALTER TABLE http_variable_history RENAME TO api_variable_history;
ALTER TABLE http_response_history RENAME TO api_response_history;

-- SQLite has no ALTER INDEX RENAME (F17) -- a rename is DROP + CREATE, done after the table
-- renames above so each CREATE INDEX names the table's new name directly.
DROP INDEX http_items_tree;
CREATE INDEX api_items_tree ON api_items(collection_id, parent_id, sort_order);

DROP INDEX http_variables_collection;
CREATE INDEX api_variables_collection ON api_variables(collection_id, sort_order);

DROP INDEX http_variables_environment;
CREATE INDEX api_variables_environment ON api_variables(environment_id, sort_order);

DROP INDEX http_variable_history_var;
CREATE INDEX api_variable_history_var ON api_variable_history(variable_id, recorded_at);

DROP INDEX http_response_history_scope;
CREATE INDEX api_response_history_scope ON api_response_history(scope_key, sent_at);

DROP INDEX http_response_history_age;
CREATE INDEX api_response_history_age ON api_response_history(sent_at);

DROP INDEX http_response_history_tab;
CREATE INDEX api_response_history_tab ON api_response_history(tab_id);
