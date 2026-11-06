#!/bin/sh
# prepare-worktree.sh — the one entry point for getting a fresh checkout (a `git worktree`, or any
# clone that hasn't run this before) ready to develop in: hooks pass, `go build ./...` succeeds,
# `bun run typecheck`/`lint` succeed. Idempotent — safe to re-run, cheap when already done.
#
# Covers exactly the five gaps a genuinely fresh worktree hits (confirmed by hitting each one):
#   1. Linux system packages the `wails3` CLI itself needs to build (checked, not reinstalled)
#   2/3/4. `bun install`, `go mod download`, pinned `wails3` install + bindings codegen — delegated to `scripts/setup.sh`, which already does exactly this
#   5. a real `frontend/dist`, so `go build ./...` (pre-push) doesn't fail on
#      `//go:embed all:frontend/dist` finding no matching files
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
# shellcheck source=./lib.sh
. "$SCRIPT_DIR/lib.sh"

# ---- 1: Linux system packages for building the wails3 CLI (not the app itself) -------------------
# Checked first, not reinstalled every run — `apt-get install` on already-installed packages is
# cheap but not free, and this runs once per fresh worktree, not once per repo.
if [ "$(uname -s)" = "Linux" ] && ! pkg-config --exists gtk4 webkitgtk-6.0 2>/dev/null; then
  echo "prepare-worktree: installing libgtk-4-dev/libwebkitgtk-6.0-dev/pkg-config (wails3 CLI build dep)"
  SUDO=""
  [ "$(id -u)" -eq 0 ] || SUDO="sudo"
  $SUDO apt-get update -qq
  $SUDO apt-get install -y --no-install-recommends libgtk-4-dev libwebkitgtk-6.0-dev pkg-config
else
  echo "prepare-worktree: wails3 CLI build deps already present, skipping apt-get"
fi

# ---- 2-4: bun workspace, Go module, pinned wails3 + bindings ------------------------
sh "$SCRIPT_DIR/setup.sh"

# ---- 5: a real frontend/dist, so `go build ./...` (pre-push) doesn't fail on the Go binaries'
# own `//go:embed all:frontend/dist` finding zero matching files -----------------------------------
for APP_DIR in apps/kira-space; do
  DIST_DIR="$ROOT_DIR/$APP_DIR/frontend/dist"
  if [ -d "$DIST_DIR" ] && [ -n "$(ls -A "$DIST_DIR" 2>/dev/null)" ]; then
    echo "prepare-worktree: $APP_DIR/frontend/dist already built, skipping"
  else
    echo "prepare-worktree: bun run build ($APP_DIR/frontend)"
    (cd "$ROOT_DIR/$APP_DIR/frontend" && bun run build)
  fi
done

# P212: Kira Space also embeds the phone app (`//go:embed all:frontend/dist-mobile`).
MOBILE_DIST="$ROOT_DIR/apps/kira-space/frontend/dist-mobile"
if [ -f "$MOBILE_DIST/index.html" ]; then
  echo "prepare-worktree: apps/kira-space/frontend/dist-mobile already built, skipping"
else
  echo "prepare-worktree: bun run build:mobile (apps/kira-space/frontend)"
  (cd "$ROOT_DIR/apps/kira-space/frontend" && bun run build:mobile)
fi

echo "prepare-worktree: done — hooks, go build ./..., bun run typecheck/lint are ready"
