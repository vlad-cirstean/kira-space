-- P85: the user's own launchable scripts, shown in the tab strip's "+" dropdown. A table, not a
-- settings leaf: these are named records with a stable id and an order, created and deleted one at
-- a time with immediate effect — the same reasoning 0023's connection_mask_rules carries, and the
-- shape code_repos (0018) already uses for the app's other user-curated list.
CREATE TABLE custom_scripts (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  command     TEXT NOT NULL,
  -- '' means "the active repo workspace's own worktree directory" (plan §7). A non-empty value is
  -- an absolute path, checked on write; its existence is not, since the launch path already checks
  -- it (bridge/terminal.go's Open) and a path may legitimately not exist yet at save time.
  working_dir TEXT NOT NULL DEFAULT '',
  -- domain/color.ts's paletteColorSchema. 'none' is a real stored value, not a null stand-in.
  color       TEXT NOT NULL DEFAULT 'none',
  sort_order  INTEGER NOT NULL,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
