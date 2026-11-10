-- P595: requests no API access rule matched were refused once any rule existed. Installs with stored rules
-- keep that as an explicit default; installs without rules keep the new default, ask.
INSERT INTO settings (key, value)
SELECT 'dbMcp.apiDefault', '"deny"'
WHERE EXISTS (
  SELECT 1 FROM settings
  WHERE key = 'dbMcp.apiRules'
    AND NOT (json_valid(value) AND (json_type(value) = 'null' OR (json_type(value) = 'array' AND json_array_length(value) = 0)))
)
ON CONFLICT (key) DO NOTHING;
