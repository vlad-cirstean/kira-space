-- P17 D14/D15: a description on a variable and an environment entry — F10's own fidelity gap
-- (Postman carries one on a variable, this app silently dropped it on round-trip) closed for these
-- two tables only; the four request tables (headers/params/urlencoded/form-data) stay P4 §8
-- OQ-10, unchanged.
--
-- NOT NULL DEFAULT '' rather than nullable: model.Variable.Description has no three-state to
-- represent, and SQLite's ALTER TABLE ADD COLUMN with a non-null default is a metadata change, not
-- a table rewrite (no row is touched).
ALTER TABLE api_variables    ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE api_environments ADD COLUMN description TEXT NOT NULL DEFAULT '';
