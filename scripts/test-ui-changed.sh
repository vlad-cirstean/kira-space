#!/bin/sh
# test-ui-changed.sh [--dry-run] [<base>] — runs the Playwright `ui` suite of each app a change
# can affect (table in scripts/hook-scope.sh `ui_target_of`). Files: `git diff --name-only <base>`
# plus untracked. Unknown paths select both apps. Default base: merge-base with origin/main.
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
targets="$(ui_targets "$files")"

if [ -z "$targets" ]; then
  echo "no UI suite affected"
  exit 0
fi

for app in studio space; do
  printf '%s\n' "$targets" | grep -qx "$app" || continue
  why="$(printf '%s\n' "$files" | while IFS= read -r f; do
    case "$(ui_target_of "$f")" in "$app" | both) echo "$f"; break ;; esac
  done)"
  echo "ui:$app (because of $why)"
done
[ "$dry" = 1 ] && exit 0

for app in studio space; do
  printf '%s\n' "$targets" | grep -qx "$app" || continue
  bun run "test:ui:$app"
done
