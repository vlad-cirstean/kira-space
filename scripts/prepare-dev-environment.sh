#!/bin/sh
# prepare-dev-environment.sh — the one entry point for getting a fresh worktree entirely ready:
# app dev readiness (scripts/prepare-worktree.sh), the Playwright webkit browser
# (scripts/prepare-ui-tests.sh), the CodeGraph index
# (scripts/codegraph-setup.sh) plus the rtk-oss binary (scripts/rtk-setup.sh), so a Claude session
# spawned into it has hooks/build passing and a synced index from its first turn. Idempotent — safe to re-run, cheap when already done.
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

sh "$SCRIPT_DIR/prepare-worktree.sh"
sh "$SCRIPT_DIR/prepare-ui-tests.sh"
sh "$SCRIPT_DIR/codegraph-setup.sh"
sh "$SCRIPT_DIR/rtk-setup.sh"

echo "prepare-dev-environment: done"
