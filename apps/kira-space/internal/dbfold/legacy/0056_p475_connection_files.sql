-- P475: connections are config files. The connections table becomes the projection of <home>/connections/*.yaml:
-- a surrogate row keeps its uuid (tabs, saved queries, metadata cache point at it) and is bound to its file by
-- stem. config_json is the last valid file content. The password and uri columns, connection_tree_filters and
-- connection_ddl are dropped by the one-shot export in connfiles, after it copies their data into files.
ALTER TABLE connections ADD COLUMN stem TEXT;
ALTER TABLE connections ADD COLUMN config_json TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX connections_stem ON connections(stem) WHERE stem IS NOT NULL;
