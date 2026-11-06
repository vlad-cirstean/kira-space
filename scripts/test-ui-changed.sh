#!/bin/sh
# test-ui-changed.sh [--dry-run] [<base>] — runs the Playwright `ui` suite when a change can affect
# it (`ui_affects` in scripts/hook-scope.sh). Files: `git diff --name-only <base>` plus untracked.
# Default base: merge-base with origin/main.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=./hook-scope.sh
. scripts/hook-scope.sh

dry=0
base=""
for a in "$@"; do
  case "$a" in
    --dry-run) dry=1 ;;
    *) base="$a" ;;
  esac
done
[ -n "$base" ] || [ -n "${UI_CHANGED_FILES:-}" ] || base="$(git merge-base HEAD origin/main)"

# UI_CHANGED_FILES (newline list) replaces the git file list, for checking the table.
files="${UI_CHANGED_FILES:-$( { git diff --name-only "$base"; git ls-files --others --exclude-standard; } | sort -u)}"

why="$(printf '%s\n' "$files" | while IFS= read -r f; do
  [ -n "$f" ] || continue
  if ui_affects "$f"; then echo "$f"; break; fi
done)"

if [ -z "$why" ]; then
  echo "no UI suite affected"
  exit 0
fi

echo "ui (because of $why)"
[ "$dry" = 1 ] && exit 0
bun run test:ui
