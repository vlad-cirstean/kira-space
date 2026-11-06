-- P473: the Api module's collections and environments are config files. The four config tables become the
-- projection of <home>/api/*.yaml: a surrogate row keeps its uuid (history and tabs point at it) and is bound
-- to its file by stem, item key and secret ref.
ALTER TABLE api_collections ADD COLUMN stem TEXT;
ALTER TABLE api_environments ADD COLUMN stem TEXT;
ALTER TABLE api_items ADD COLUMN item_key TEXT;
ALTER TABLE api_variables ADD COLUMN secret_ref TEXT;
CREATE UNIQUE INDEX api_collections_stem ON api_collections(stem) WHERE stem IS NOT NULL;
CREATE UNIQUE INDEX api_environments_stem ON api_environments(stem) WHERE stem IS NOT NULL;
CREATE UNIQUE INDEX api_items_key ON api_items(collection_id, item_key) WHERE item_key IS NOT NULL;
