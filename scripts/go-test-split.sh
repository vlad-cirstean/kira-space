#!/bin/sh
# go-test-split.sh — splits `go test ./...` into groups and shards for CI (pr.yml go-split).
#
#   go-test-split.sh <containers|rest>[/<shard>] [go test args...]
#   go-test-split.sh --matrix <group>          JSON array of shard names
#   go-test-split.sh --list <group>[/<shard>]  packages, one per line (slice note on stderr)
#   go-test-split.sh --bundles <group>[/<shard>]   image bundles (bare group: all its shards)
#   go-test-split.sh --kinds <bundle...>       pull-test-images.sh kinds of those bundles
#   go-test-split.sh --pins-hash               16 hex chars, changes only when an image pin changes
#   go-test-split.sh --check                   table sanity, needs go and the module
#
# Bare group = whole group, no slicing (bun run test:go:*, cache-warm.yml).
# Load balancing only, never a coverage filter: a package matching no named shard of its group lands
# in that group's `other` shard, so every package runs in exactly one shard by construction.
#
# Basis: PR run 43628609545 (v0.3.13), seconds per shard, `go test` -p 4:
#   containers one job, about 250 s of tests (kibana 131, dbflow 105 longest);
#   adeflow1 and adeflow2 about 139 each (278 split in two), rest other about 500 summed (ade 75 longest).
# A new package that starts containers must match the containers group (GROUP_PATTERN), or its
# fixtures pull from Docker Hub in the `rest` job.
set -eu

GROUP_PATTERN='/internal/adapters/|/internal/ipcfixture|/flows/dockerflow|/flows/dbflow|/flows/dbmcpflow'

# group;shard;pattern (ERE on import path, empty = remainder);slice k/n;image bundles.
# First matching pattern owns a package; rows sharing a pattern are slices of one package by test name.
TABLE='containers;other;;;elastic sql kv stream flow
rest;adeflow1;/internal/flows/adeflow$;1/2;
rest;adeflow2;/internal/flows/adeflow$;2/2;
rest;other;;;'

# bundle:kinds (kinds as in testsupport/images.go; `flow` is pull-test-images.sh's pseudo-kind).
BUNDLES='elastic:elasticsearch kibana
sql:postgres mysql mariadb clickhouse
kv:redis mongo
stream:kafka localstack
flow:flow'

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ROOT_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
SUPPORT_DIR="$ROOT_DIR/apps/kira-space/internal/adapters/testsupport"

usage() {
  echo "usage: go-test-split.sh <containers|rest>[/<shard>] [go test args...]" >&2
  echo "       go-test-split.sh --matrix <group> | --list <group>[/<shard>] | --bundles containers/<shard>" >&2
  echo "       go-test-split.sh --kinds <bundle...> | --pins-hash | --check" >&2
  exit 2
}

# row <group> <shard>: the table line, empty if none.
row() {
  printf '%s\n' "$TABLE" | awk -F';' -v g="$1" -v s="$2" '$1 == g && $2 == s'
}

# field <group> <shard> <n>
field() {
  row "$1" "$2" | awk -F';' -v n="$3" '{print $n}'
}

group_ok() {
  case "$1" in containers | rest) ;; *) usage ;; esac
}

# split_target <group[/shard]>: sets GROUP and SHARD (empty = whole group); exits 2 when unknown.
split_target() {
  GROUP="${1%%/*}"
  SHARD=""
  case "$1" in */*) SHARD="${1#*/}" ;; esac
  group_ok "$GROUP"
  if [ -n "$SHARD" ] && [ -z "$(row "$GROUP" "$SHARD")" ]; then
    echo "go-test-split: unknown shard '$SHARD' in group '$GROUP'" >&2
    usage
  fi
}

group_pkgs() {
  if [ "$1" = containers ]; then
    go list ./... | grep -E "$GROUP_PATTERN" || true
  else
    go list ./... | grep -vE "$GROUP_PATTERN" || true
  fi
}

# shard_pkgs <group> <shard>: packages of one shard (a slice shard lists its whole package).
# Named shard: matches its pattern and no earlier row's different pattern. Remainder: matches none.
shard_pkgs() {
  _pat="$(field "$1" "$2" 3)"
  if [ -n "$_pat" ]; then
    _out="$(group_pkgs "$1" | grep -E "$_pat" || true)"
    _earlier="$(printf '%s\n' "$TABLE" | awk -F';' -v g="$1" -v s="$2" '
      $1 == g { if ($2 == s) exit; if ($3 != "") print $3 }')"
  else
    _out="$(group_pkgs "$1")"
    _earlier="$(printf '%s\n' "$TABLE" | awk -F';' -v g="$1" '$1 == g && $3 != "" {print $3}' | sort -u)"
  fi
  for _p in $_earlier; do
    [ "$_p" = "$_pat" ] || _out="$(printf '%s\n' "$_out" | grep -vE "$_p" || true)"
  done
  printf '%s\n' "$_out" | sed '/^$/d'
}

# slice_tests <package> <k/n>: top-level test names of slice k of n, round robin over the sorted list.
slice_tests() {
  _k="${2%/*}"
  _n="${2#*/}"
  # Plain assignment: a failed listing (build error, TestMain failure) must fail the shard, not yield an empty slice.
  _raw="$(go test -list '.' "$1")" || {
    echo "go-test-split: go test -list failed for $1" >&2
    return 1
  }
  printf '%s\n' "$_raw" | grep -E '^(Test|Fuzz|Example)' | sort | awk -v k="$_k" -v n="$_n" '(NR - 1) % n == k - 1'
}

slice_regex() {
  printf '^(%s)$' "$(printf '%s\n' "$1" | paste -sd'|' -)"
}

bundle_kinds() {
  printf '%s\n' "$BUNDLES" | awk -F: -v b="$1" '$1 == b {print $2}'
}

pins_hash() {
  {
    grep -hoE '[A-Za-z]*Image[A-Za-z]* *= *"[^"]+"' "$SUPPORT_DIR"/*.go
    grep -hoE 'library/[a-z]+:[0-9][0-9.]*' "$SCRIPT_DIR/pull-test-images.sh"
  } | sort | if command -v sha256sum >/dev/null 2>&1; then sha256sum; else shasum -a 256; fi | cut -c1-16
}

check() {
  _fail=0
  for _g in containers rest; do
    for _s in $(printf '%s\n' "$TABLE" | awk -F';' -v g="$_g" '$1 == g && $3 != "" {print $2}'); do
      if [ -z "$(shard_pkgs "$_g" "$_s")" ]; then
        echo "go-test-split: shard $_g/$_s matches no package (stale table)" >&2
        _fail=1
      fi
    done
  done
  _union="$( (group_pkgs containers; group_pkgs rest) | sort)"
  if [ "$_union" != "$(go list ./... | sort)" ] || [ -n "$(printf '%s\n' "$_union" | uniq -d)" ]; then
    echo "go-test-split: containers and rest are not a partition of go list ./..." >&2
    _fail=1
  fi
  # Every shard package runs exactly once per group.
  for _g in containers rest; do
    _got="$(for _s in $(matrix_names "$_g"); do shard_pkgs "$_g" "$_s"; done | sort -u)"
    if [ "$_got" != "$(group_pkgs "$_g" | sort)" ]; then
      echo "go-test-split: shards of $_g do not cover the group exactly" >&2
      _fail=1
    fi
  done
  _kinds="$(grep -oE '^[[:space:]]*\{"[a-z]+",' "$SUPPORT_DIR/images.go" | grep -oE '"[a-z]+"' | tr -d '"' | sort)"
  _bundled="$(printf '%s\n' "$BUNDLES" | awk -F: '$1 != "flow" {print $2}' | tr ' ' '\n' | sort)"
  if [ "$_kinds" != "$_bundled" ]; then
    echo "go-test-split: a testimages kind is in no bundle or in two" >&2
    _fail=1
  fi
  _pins="$(grep -hoE '[A-Za-z]*Image[A-Za-z]* *= *"[^"]+"' "$SUPPORT_DIR"/*.go | wc -l | tr -d ' ')"
  _images="$(cd "$ROOT_DIR" && go run ./apps/kira-space/internal/adapters/testsupport/cmd/testimages | wc -l | tr -d ' ')"
  if [ "$_pins" != "$_images" ]; then
    echo "go-test-split: pins-hash grep finds $_pins pins, testimages prints $_images (renamed constant?)" >&2
    _fail=1
  fi
  [ "$_fail" = 0 ] && echo "go-test-split: ok"
  return "$_fail"
}

matrix_names() {
  printf '%s\n' "$TABLE" | awk -F';' -v g="$1" '$1 == g {print $2}'
}

case "${1:-}" in
  --matrix)
    [ "$#" -eq 2 ] || usage
    group_ok "$2"
    printf '[%s]\n' "$(matrix_names "$2" | sed 's/.*/"&"/' | paste -sd, -)"
    exit 0
    ;;
  --list)
    [ "$#" -eq 2 ] || usage
    split_target "$2"
    if [ -n "$SHARD" ]; then
      _pkgs="$(shard_pkgs "$GROUP" "$SHARD")"
      _slice="$(field "$GROUP" "$SHARD" 4)"
      printf '%s\n' "$_pkgs"
      if [ -n "$_slice" ]; then
        _st="$(slice_tests "$_pkgs" "$_slice")" || exit 1
        echo "slice $_slice: $(printf '%s\n' "$_st" | sed '/^$/d' | wc -l | tr -d ' ') tests" >&2
      fi
    else
      group_pkgs "$GROUP"
    fi
    exit 0
    ;;
  --bundles)
    [ "$#" -eq 2 ] || usage
    split_target "$2"
    if [ -n "$SHARD" ]; then
      field "$GROUP" "$SHARD" 5
    else
      printf '%s\n' "$TABLE" | awk -F';' -v g="$GROUP" '$1 == g {print $5}' | tr ' ' '\n' | sed '/^$/d' | sort -u | tr '\n' ' ' | sed 's/ *$//'
      echo
    fi
    exit 0
    ;;
  --kinds)
    shift
    [ "$#" -ge 1 ] || usage
    for _b in "$@"; do
      _k="$(bundle_kinds "$_b")"
      if [ -z "$_k" ]; then
        echo "go-test-split: unknown bundle '$_b'" >&2
        usage
      fi
      echo "$_k"
    done | tr '\n' ' ' | sed 's/ *$//'
    echo
    exit 0
    ;;
  --pins-hash)
    pins_hash
    exit 0
    ;;
  --check)
    check
    exit $?
    ;;
  "" | -*) usage ;;
esac

split_target "$1"
shift

if [ "$GROUP" = containers ]; then
  echo "go-test-split: docker kernel $(docker info --format '{{.KernelVersion}}' 2>/dev/null || echo unknown)"
fi

if [ -z "$SHARD" ]; then
  PKGS="$(group_pkgs "$GROUP")"
else
  PKGS="$(shard_pkgs "$GROUP" "$SHARD")"
fi
if [ -z "$PKGS" ]; then
  echo "go-test-split: $GROUP${SHARD:+/$SHARD} matches no package" >&2
  exit 1
fi

SLICE=""
[ -z "$SHARD" ] || SLICE="$(field "$GROUP" "$SHARD" 4)"
if [ -n "$SLICE" ]; then
  for _a in "$@"; do
    case "$_a" in
      -run | -run=* | --run | --run=* | -list | -list=* | --list | --list=* | -exec | -exec=* | --exec | --exec=*)
        echo "go-test-split: slice shard $GROUP/$SHARD refuses $_a (it would break or fake the slice)" >&2
        exit 2
        ;;
    esac
  done
  TESTS="$(slice_tests "$PKGS" "$SLICE")" || exit 1
  if [ -z "$TESTS" ]; then
    echo "go-test-split: slice $SLICE of $PKGS has no tests" >&2
    exit 1
  fi
  echo "go-test-split: $GROUP/$SHARD runs slice $SLICE ($(printf '%s\n' "$TESTS" | wc -l | tr -d ' ') tests)"
  exec go test "$@" -run "$(slice_regex "$TESTS")" "$PKGS"
fi

# $PKGS is deliberately word-split into package paths.
# shellcheck disable=SC2086
exec go test "$@" $PKGS
