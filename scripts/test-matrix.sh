#!/bin/sh
# test-matrix.sh — the on-demand complete auth/config permutation matrix runner (P25 §2.10,
# git show bd0c224688d9:docs/v1.1/plans/P25-connection-auth-test-matrix.md). Not run as part of `bun run test:go` or
# ci.yml — the same "not run as part of the regular CI/test run" posture db-compat.sh already
# established for exactly this kind of real-container matrix (P16 D6).
#
# Modelled directly on db-compat.sh, which is this repo's own precedent for this problem: for
# each adapter, this runs its package's own KIRA_TEST_MATRIX-gated authmatrix_test.go (§3's
# Case/RunMatrix table) with the env var set, against a real container. Every row runs even when
# an earlier one fails (no `set -e` abort mid-matrix) — a run that dies on the first failing
# adapter is worth much less than one that reports a full result table, the same reasoning
# db-compat.sh's own comment already gives.
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
# shellcheck source=./lib.sh
. "$SCRIPT_DIR/lib.sh"
cd "$ROOT_DIR"

ONLY=""
MIRROR=0
NO_PULL=0

usage() {
  echo "usage: $0 [--only <kind>] [--mirror] [--no-pull]" >&2
  exit 2
}

while [ $# -gt 0 ]; do
  case "$1" in
    --only) ONLY="$2"; shift 2 ;;
    --mirror) MIRROR=1; shift ;;
    --no-pull) NO_PULL=1; shift ;;
    -h|--help) usage ;;
    *) echo "test-matrix: unrecognized argument '$1'" >&2; usage ;;
  esac
done

echo "test-matrix: docker kernel $(docker info --format '{{.KernelVersion}}' 2>/dev/null || echo unknown)"

# --- the matrix (P25 §2.4-§2.9): kind|go package ------------------------------------------------
# One row per adapter package's own authmatrix_test.go. Images come from testsupport's own constants
# (the testimages command). A compat-matrix cross product (min/max × auth matrix) is explicitly out
# of scope (P25 doesn't ask for it, and db-compat.sh already covers the min/max axis on the
# general-tier suite).
MATRIX="
postgres|./apps/kira-space/internal/adapters/postgres/...
mysqlfamily|./apps/kira-space/internal/adapters/mysqlfamily/...
clickhouse|./apps/kira-space/internal/adapters/clickhouse/...
mongo|./apps/kira-space/internal/adapters/mongo/...
redis|./apps/kira-space/internal/adapters/redis/...
kafka|./apps/kira-space/internal/adapters/kafka/...
sqs|./apps/kira-space/internal/adapters/sqs/...
s3|./apps/kira-space/internal/adapters/s3/...
sqlite|./apps/kira-space/internal/adapters/sqlite/...
"

OLDIFS="$IFS"
IFS='
'
FILTERED=""
for row in $MATRIX; do
  [ -z "$row" ] && continue
  kind="$(echo "$row" | cut -d'|' -f1)"
  if [ -n "$ONLY" ] && [ "$kind" != "$ONLY" ]; then continue; fi
  FILTERED="$FILTERED$row
"
done
IFS="$OLDIFS"

if [ -z "$FILTERED" ]; then
  echo "test-matrix: no rows matched --only='$ONLY'" >&2
  exit 2
fi

if [ "$NO_PULL" != "1" ]; then
  IMAGES="$(go run ./apps/kira-space/internal/adapters/testsupport/cmd/testimages)"
  # shellcheck disable=SC2086
  pull_images test-matrix "$MIRROR" $IMAGES || exit 1
fi

# --- run every adapter's matrix, recording pass/fail without letting one abort the rest --------
RESULTS=""
FAILED=0

IFS='
'
for row in $FILTERED; do
  [ -z "$row" ] && continue
  IFS="$OLDIFS"
  kind="$(echo "$row" | cut -d'|' -f1)"
  pkg="$(echo "$row" | cut -d'|' -f2)"

  echo ""
  echo "=== test-matrix: $kind ==="
  set +e
  # -count=1 (db-compat.sh's own documented reason, carried over verbatim): go test's result cache
  # keys on env vars a test actually observed, and a skipped run may never read KIRA_TEST_MATRIX at
  # all, so a stale cache entry can report green without running anything.
  KIRA_TEST_MATRIX=1 go test -count=1 -timeout 30m "$pkg"
  status=$?
  set -e

  if [ "$status" = "0" ]; then
    RESULTS="$RESULTS$kind|PASS
"
  else
    RESULTS="$RESULTS$kind|FAIL
"
    FAILED=1
  fi
done
IFS="$OLDIFS"

echo ""
echo "=== test-matrix summary ==="
printf '%-14s %s\n' "KIND" "RESULT"
printf '%s' "$RESULTS" | while IFS='|' read -r kind result; do
  [ -z "$kind" ] && continue
  printf '%-14s %s\n' "$kind" "$result"
done

if [ "$FAILED" = "1" ]; then
  echo "test-matrix: FAILED — see above" >&2
  exit 1
fi

echo "test-matrix: all adapters passed"
