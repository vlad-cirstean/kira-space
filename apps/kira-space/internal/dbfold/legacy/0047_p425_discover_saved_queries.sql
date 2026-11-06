-- P425: saved Discover queries are kind 'discover', one name per connection (Kibana's own rule).
-- Partial: filter and console rows keep allowing duplicate names, as before.
CREATE UNIQUE INDEX saved_queries_discover_name
  ON saved_queries(connection_id, lower(name)) WHERE kind = 'discover';
