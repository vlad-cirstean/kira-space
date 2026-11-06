-- P18 D16/D19: an environment carries a user-assigned colour, from the same palette Studio's
-- connections use (domain/color.ts's paletteColorSchema) -- 'none' is a real, storable value and
-- the deliberate default (D16: "no colour is the default"), so every existing row is already a
-- valid palette value with no backfill needed.
--
-- NOT NULL DEFAULT 'none' rather than '' or nullable: 0011's own reasoning applies identically --
-- model.Environment.Color has no three-state to represent, and SQLite's ALTER TABLE ADD COLUMN
-- with a non-null default is a metadata change, not a table rewrite (no row is touched).
ALTER TABLE api_environments ADD COLUMN color TEXT NOT NULL DEFAULT 'none';
