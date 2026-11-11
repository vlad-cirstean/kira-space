#!/bin/sh
# test-matrix.sh — the complete auth/config permutation matrix runner (P25 §2.10,
# git show bd0c224688d9:docs/v1.1/plans/P25-connection-auth-test-matrix.md). Runs on every PR in
# pr.yml (one job per group, P614), not in `bun run test:go`.
#
# Modelled directly on db-compat.sh, which is this repo's own precedent for this problem: for
# each adapter, this runs its package's own KIRA_TEST_MATRIX-gated authmatrix_test.go (§3's
# Case/RunMatrix table) with the env var set, against a real container. Every row runs even when
# an earlier one fails (no `set -e` abort mid-matrix) — a run that dies on the first failing
# adapter is worth much less than one that reports a full result table, the same reasoning
# db-compat.sh's own comment already gives.
#
# P608: --only/--except take comma lists of matrix kinds, so the release workflow runs one job per
# weight group (postgres, mysqlfamily, clickhouse, rest). Each run pulls only the images its
# selected kinds start (third MATRIX field), never every pinned image.
# P614: --print-images lists those images (the PR image-cache key); --pull-only pulls them and
# exits (cache-warm.yml).
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
# shellcheck source=./lib.sh
. "$SCRIPT_DIR/lib.sh"
cd "$ROOT_DIR"

ONLY=""
EXCEPT=""
MIRROR=0
NO_PULL=0
PULL_ONLY=0
PRINT=0

usage() {
  echo "usage: $0 [--only <kind[,kind...]> | --except <kind[,kind...]>] [--mirror] [--no-pull | --pull-only | --print-images]" >&2
  exit 2
}

while [ $# -gt 0 ]; do
  case "$1" in
    --only) ONLY="${2:-}"; shift 2 || usage ;;
    --except) EXCEPT="${2:-}"; shift 2 || usage ;;
    --mirror) MIRROR=1; shift ;;
    --no-pull) NO_PULL=1; shift ;;
    --pull-only) PULL_ONLY=1; shift ;;
    --print-images) PRINT=1; shift ;;
    -h|--help) usage ;;
    *) echo "test-matrix: unrecognized argument '$1'" >&2; usage ;;
  esac
done

if [ -n "$ONLY" ] && [ -n "$EXCEPT" ]; then
  echo "test-matrix: --only and --except are mutually exclusive" >&2
  usage
fi

if [ $((NO_PULL + PULL_ONLY + PRINT)) -gt 1 ]; then
  echo "test-matrix: --no-pull, --pull-only and --print-images are mutually exclusive" >&2
  exit 2
fi

# --- the matrix (P25 §2.4-§2.9): kind|go package|testimages kinds the package starts ------------------------------------------------
# One row per adapter package's own authmatrix_test.go. Images come from testsupport's own constants
# (the testimages command). A compat-matrix cross product (min/max × auth matrix) is explicitly out
# of scope (P25 doesn't ask for it, and db-compat.sh already covers the min/max axis on the
# general-tier suite).
MATRIX="
postgres|./apps/kira-space/internal/adapters/postgres/...|postgres
mysqlfamily|./apps/kira-space/internal/adapters/mysqlfamily/...|mysql mariadb
clickhouse|./apps/kira-space/internal/adapters/clickhouse/...|clickhouse
mongo|./apps/kira-space/internal/adapters/mongo/...|mongo
redis|./apps/kira-space/internal/adapters/redis/...|redis
kafka|./apps/kira-space/internal/adapters/kafka/...|kafka
sqs|./apps/kira-space/internal/adapters/sqs/...|localstack
s3|./apps/kira-space/internal/adapters/s3/...|localstack
sqlite|./apps/kira-space/internal/adapters/sqlite/...|
"

# Every named kind must exist: a workflow typo must fail, not silently run an unintended set.
ALLKINDS="$(printf '%s\n' "$MATRIX" | cut -d'|' -f1 | grep -v '^$')"
for k in $(printf '%s' "$ONLY,$EXCEPT" | tr ',' ' '); do
  if ! printf '%s\n' "$ALLKINDS" | grep -qx "$k"; then
    echo "test-matrix: unknown kind '$k'" >&2
    exit 2
  fi
done

OLDIFS="$IFS"
IFS='
'
FILTERED=""
for row in $MATRIX; do
  [ -z "$row" ] && continue
  kind="$(echo "$row" | cut -d'|' -f1)"
  if [ -n "$ONLY" ]; then
    case ",$ONLY," in *",$kind,"*) ;; *) continue ;; esac
  fi
  if [ -n "$EXCEPT" ]; then
    case ",$EXCEPT," in *",$kind,"*) continue ;; esac
  fi
  FILTERED="$FILTERED$row
"
done
IFS="$OLDIFS"

if [ -z "$FILTERED" ]; then
  echo "test-matrix: no rows matched --only='$ONLY' --except='$EXCEPT'" >&2
  exit 2
fi

KINDS="$(printf '%s' "$FILTERED" | cut -d'|' -f3 | tr ' ' '\n' | grep -v '^$' | sort -u | tr '\n' ' ')"

if [ "$PRINT" = "1" ]; then
  # Never call testimages with no kinds: that prints every image.
  if [ -n "$KINDS" ]; then
    # shellcheck disable=SC2086
    go run ./apps/kira-space/internal/adapters/testsupport/cmd/testimages $KINDS
  fi
  exit 0
fi

echo "test-matrix: docker kernel $(docker info --format '{{.KernelVersion}}' 2>/dev/null || echo unknown)"
echo "test-matrix: kinds: $(printf '%s' "$FILTERED" | cut -d'|' -f1 | tr '\n' ' ' | sed 's/ $//')"

if [ "$NO_PULL" != "1" ] && [ -n "$KINDS" ]; then
  # shellcheck disable=SC2086
  IMAGES="$(go run ./apps/kira-space/internal/adapters/testsupport/cmd/testimages $KINDS)"
  # shellcheck disable=SC2086
  pull_images test-matrix "$MIRROR" $IMAGES || exit 1
fi

if [ "$PULL_ONLY" = "1" ]; then
  echo "test-matrix: pulled"
  exit 0
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
  # -timeout 8m sits under the 12-min CI job cap, so a hang panics with a goroutine dump naming the
  # test before the runner kills the job.
  KIRA_TEST_MATRIX=1 go test -count=1 -timeout 8m "$pkg"
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
