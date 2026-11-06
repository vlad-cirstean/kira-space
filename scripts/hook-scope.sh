#!/bin/sh
# hook-scope.sh — shared by .githooks/pre-commit and pre-push. Sourced, not run.
# Scopes the hooks to changed files; any change to config, lockfiles, shared tooling or an
# unmapped shared package falls back to the full run. CI always runs everything.

# Paths whose change can affect every check.
FULL_RE='(^|/)(package\.json|tsconfig[^/]*\.json|vite[^/]*\.(ts|js|mts)|bun\.lock|bunfig\.toml|biome\.json|knip\.json|go\.mod|go\.sum|\.golangci\.yml)$|^(\.githooks|\.github|scripts)/'

# is_full <file list>: 0 when the list holds a full-run trigger.
is_full() {
  printf '%s\n' "$1" | grep -Eq "$FULL_RE"
}

# any <file list> <regex>
any() {
  printf '%s\n' "$1" | grep -Eq "$2"
}

# typecheck_targets <file list>: prints the `typecheck:*` script names the list touches, or the
# word `full` when a changed TS/Vue file maps to no single project (shared package).
typecheck_targets() {
  # MessageKey derives from the catalog, so a catalog change can break any project.
  if printf '%s\n' "$1" | grep -q '^packages/theme/src/i18n/'; then
    echo full
    return 0
  fi
  _ts="$(printf '%s\n' "$1" | grep -E '\.(ts|mts|vue|js|d\.ts)$' || true)"
  [ -n "$_ts" ] || return 0
  printf '%s\n' "$_ts" | while IFS= read -r _f; do
    case "$_f" in
      apps/kira-space/frontend/proto/*|apps/kira-space/frontend/*proto*|apps/kira-space/tests/proto/*) echo "proto web mobile tests unit" ;;
      apps/kira-space/frontend/*|apps/kira-space/tests/*|apps/kira-space/tsconfig*|apps/kira-space/playwright*) echo "web mobile tests unit" ;;
      packages/api-core/*) echo "api-core" ;;
      packages/git-*/*) echo "git" ;;
      packages/docker-ui/*) echo "docker" ;;
      site/scripts/*) echo "docs-check" ;;
      site/*) echo "site" ;;
      *) echo "full" ;;
    esac
  done | tr ' ' '\n' | sort -u
}

# go_mod_dir <dir>: prints the dir holding the nearest go.mod at or above <dir> (`.` = root module).
go_mod_dir() {
  _d="$1"
  while [ "$_d" != "." ] && [ "$_d" != "/" ] && [ ! -f "$_d/go.mod" ]; do
    _d="$(dirname "$_d")"
  done
  echo "$_d"
}

# go_dirs <file list>: package dirs (./path) of changed Go files inside the root module.
# Nested modules (apps/*/tests/claude) are excluded: the root `golangci-lint`/`go test` reject them.
go_dirs() {
  printf '%s\n' "$1" | grep -E '\.go$' | xargs -r -n1 dirname | sort -u | while IFS= read -r _g; do
    [ "$(go_mod_dir "$_g")" = "." ] && echo "./$_g"
  done
}

# nested_go_mods <file list>: dirs of nested Go modules holding a changed Go file.
nested_go_mods() {
  printf '%s\n' "$1" | grep -E '\.go$' | xargs -r -n1 dirname | sort -u | while IFS= read -r _g; do
    _m="$(go_mod_dir "$_g")"
    [ "$_m" = "." ] || echo "$_m"
  done | sort -u
}

# ui_affects <file>: 0 when one path can affect the Playwright `ui` suite. UI specs run a built
# bundle against mocks, so Go code never reaches them; contract JSON and bindings sit under the
# app dir and select it.
ui_affects() {
  case "$1" in
    go.mod|go.sum|*/go.mod|*/go.sum|.golangci.yml|*/.golangci.yml) return 1 ;;
  esac
  if printf '%s\n' "$1" | grep -Eq "$FULL_RE"; then return 0; fi
  case "$1" in
    *.go|internal/*|apps/*/internal/*|apps/*/tests/claude/*|docs/*|site/*|*.md) return 1 ;;
    *) return 0 ;;
  esac
}
