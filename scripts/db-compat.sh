#!/bin/sh
# db-compat.sh — the on-demand DB adapter min/max version compatibility runner (P16 D5,
# git show bd0c224688d9:docs/v1.1/plans/P16-db-compat-suite.md §5 C3). Not run as part of `bun run test:go` or
# ci.yml — SPEC.md's own words for this phase are "not run as part of the regular CI/test run".
#
# For each (kind, min|max) pair below, this pulls that pair's image(s), retags them, exports the
# testsupport.ImageFor override(s) (P16 D2), and runs the adapter's existing conformance package
# verbatim — the same coverage `bun run test:go` already runs, just against the version extremes
# instead of one pinned default. The matrix mirrors §3's version table exactly; if the two ever
# disagree, the table in the plan doc is wrong, not this script.
#
# Every pair runs even when an earlier one fails (no `set -e` abort mid-matrix) — a run that dies
# on the first old MySQL is worth much less than one that reports all sixteen results, which is
# why FAILED is tracked by hand rather than left to the shell's own exit-on-error.
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
# shellcheck source=./lib.sh
. "$SCRIPT_DIR/lib.sh"
cd "$ROOT_DIR"

ONLY=""
EXTREME=""
MIRROR=0
NO_PULL=0

usage() {
  echo "usage: $0 [--only <kind>] [--min|--max] [--mirror] [--no-pull]" >&2
  exit 2
}

while [ $# -gt 0 ]; do
  case "$1" in
    --only) ONLY="$2"; shift 2 ;;
    --min) EXTREME="min"; shift ;;
    --max) EXTREME="max"; shift ;;
    --mirror) MIRROR=1; shift ;;
    --no-pull) NO_PULL=1; shift ;;
    -h|--help) usage ;;
    *) echo "db-compat: unrecognized argument '$1'" >&2; usage ;;
  esac
done

echo "db-compat: docker kernel $(docker info --format '{{.KernelVersion}}' 2>/dev/null || echo unknown)"

# --- the version table (P16 §3) --------------------------------------------------------------
POSTGRES_MIN="postgres:14.24-alpine"
POSTGRES_MAX="postgres:18.6-alpine"
MARIADB_MIN="mariadb:10.11.19"
MARIADB_MAX="mariadb:12.3.3"
MYSQL_MIN="mysql:8.0.46"
MYSQL_MAX="mysql:9.7.2"
CLICKHOUSE_MIN="clickhouse/clickhouse-server:25.3.14"
CLICKHOUSE_MAX="clickhouse/clickhouse-server:26.8.33"
MONGO_MIN="mongo:4.4.30"
MONGO_MAX="mongo:7.0.45"
REDIS_MIN="redis:7.0.15"
REDIS_MAX="redis:8.10.2"
KAFKA_MIN="confluentinc/cp-kafka:7.4.0"
KAFKA_MAX="confluentinc/cp-kafka:8.3.0"
LOCALSTACK_MIN="localstack/localstack:3.8.1"
LOCALSTACK_MAX="localstack/localstack:4.14.0"

# kind|extreme|go package|env assignments (space-separated KEY=value), one line per invocation —
# D3's 16-row table, verbatim. mysqlfamily sets both KIRA_COMPAT_IMAGE_MARIADB and
# KIRA_COMPAT_IMAGE_MYSQL per run (min-with-min, max-with-max) since both containers live in one
# test binary (D3) — sqlite is absent by design, it has no server to vary (F11).
MATRIX="
postgres|min|./apps/kira-space/internal/adapters/postgres/...|KIRA_COMPAT_IMAGE_POSTGRES=$POSTGRES_MIN
postgres|max|./apps/kira-space/internal/adapters/postgres/...|KIRA_COMPAT_IMAGE_POSTGRES=$POSTGRES_MAX
mysqlfamily|min|./apps/kira-space/internal/adapters/mysqlfamily/...|KIRA_COMPAT_IMAGE_MARIADB=$MARIADB_MIN KIRA_COMPAT_IMAGE_MYSQL=$MYSQL_MIN
mysqlfamily|max|./apps/kira-space/internal/adapters/mysqlfamily/...|KIRA_COMPAT_IMAGE_MARIADB=$MARIADB_MAX KIRA_COMPAT_IMAGE_MYSQL=$MYSQL_MAX
clickhouse|min|./apps/kira-space/internal/adapters/clickhouse/...|KIRA_COMPAT_IMAGE_CLICKHOUSE=$CLICKHOUSE_MIN
clickhouse|max|./apps/kira-space/internal/adapters/clickhouse/...|KIRA_COMPAT_IMAGE_CLICKHOUSE=$CLICKHOUSE_MAX
mongo|min|./apps/kira-space/internal/adapters/mongo/...|KIRA_COMPAT_IMAGE_MONGO=$MONGO_MIN
mongo|max|./apps/kira-space/internal/adapters/mongo/...|KIRA_COMPAT_IMAGE_MONGO=$MONGO_MAX
redis|min|./apps/kira-space/internal/adapters/redis/...|KIRA_COMPAT_IMAGE_REDIS=$REDIS_MIN
redis|max|./apps/kira-space/internal/adapters/redis/...|KIRA_COMPAT_IMAGE_REDIS=$REDIS_MAX
kafka|min|./apps/kira-space/internal/adapters/kafka/...|KIRA_COMPAT_IMAGE_KAFKA=$KAFKA_MIN
kafka|max|./apps/kira-space/internal/adapters/kafka/...|KIRA_COMPAT_IMAGE_KAFKA=$KAFKA_MAX
sqs|min|./apps/kira-space/internal/adapters/sqs/...|KIRA_COMPAT_IMAGE_LOCALSTACK=$LOCALSTACK_MIN
sqs|max|./apps/kira-space/internal/adapters/sqs/...|KIRA_COMPAT_IMAGE_LOCALSTACK=$LOCALSTACK_MAX
s3|min|./apps/kira-space/internal/adapters/s3/...|KIRA_COMPAT_IMAGE_LOCALSTACK=$LOCALSTACK_MIN
s3|max|./apps/kira-space/internal/adapters/s3/...|KIRA_COMPAT_IMAGE_LOCALSTACK=$LOCALSTACK_MAX
"

# --- filter the matrix down to what was asked for ----------------------------------------------
FILTERED=""
OLDIFS="$IFS"
IFS='
'
for row in $MATRIX; do
  [ -z "$row" ] && continue
  kind="$(echo "$row" | cut -d'|' -f1)"
  extreme="$(echo "$row" | cut -d'|' -f2)"
  if [ -n "$ONLY" ] && [ "$kind" != "$ONLY" ]; then continue; fi
  if [ -n "$EXTREME" ] && [ "$extreme" != "$EXTREME" ]; then continue; fi
  FILTERED="$FILTERED$row
"
done
IFS="$OLDIFS"

if [ -z "$FILTERED" ]; then
  echo "db-compat: no rows matched --only='$ONLY' --min/--max='$EXTREME'" >&2
  exit 2
fi

# Unique images across the filtered rows' env assignments, pulled once each up front, in parallel
# (§6.2: "pull first, all of it, before running anything" — a mid-run pull failure is much harder
# to read than a pre-flight one). --mirror: see pull_images in lib.sh.
if [ "$NO_PULL" != "1" ]; then
  IMAGES="$(printf '%s' "$FILTERED" | cut -d'|' -f4 | tr ' ' '\n' | cut -d'=' -f2- | sort -u)"
  # shellcheck disable=SC2086
  pull_images db-compat "$MIRROR" $IMAGES || exit 1
fi

# --- run every pair, recording pass/fail without letting one abort the rest -------------------
RESULTS=""
FAILED=0

IFS='
'
for row in $FILTERED; do
  [ -z "$row" ] && continue
  # `for row in $FILTERED` already split on IFS at loop entry — resetting it back to the shell's
  # default here (rather than after the loop) is what lets `env $envassign` below word-split on
  # spaces again; IFS='|' would silently pass one glued-together arg to env otherwise.
  IFS="$OLDIFS"
  kind="$(echo "$row" | cut -d'|' -f1)"
  extreme="$(echo "$row" | cut -d'|' -f2)"
  pkg="$(echo "$row" | cut -d'|' -f3)"
  envassign="$(echo "$row" | cut -d'|' -f4)"

  echo ""
  echo "=== db-compat: $kind ($extreme) — $envassign ==="
  set +e
  # -count=1 (F13): go test's own result cache keys on env vars a test observed, and a skipped
  # (no-Docker) run may never read the override at all, so a stale cache entry can report green
  # without ever starting the container this pair asked for.
  env $envassign go test -count=1 -timeout 30m $pkg
  status=$?
  set -e

  if [ "$status" = "0" ]; then
    RESULTS="$RESULTS$kind|$extreme|$envassign|PASS
"
  else
    RESULTS="$RESULTS$kind|$extreme|$envassign|FAIL
"
    FAILED=1
  fi
done
IFS="$OLDIFS"

echo ""
echo "=== db-compat summary ==="
printf '%-14s %-6s %-70s %s\n' "KIND" "EXTREME" "IMAGE(S)" "RESULT"
printf '%s' "$RESULTS" | while IFS='|' read -r kind extreme envassign result; do
  [ -z "$kind" ] && continue
  printf '%-14s %-6s %-70s %s\n' "$kind" "$extreme" "$envassign" "$result"
done

if [ "$FAILED" = "1" ]; then
  echo "db-compat: FAILED — see above" >&2
  exit 1
fi

echo "db-compat: all pairs passed"
