#!/bin/sh
# pull-test-images.sh [--print] [kind...] — pulls Testcontainers images through mirror.gcr.io in
# parallel and
# re-tags it to the plain name (Docker Hub blob CDN is blocked in the sandbox; the mirror is not).
# Flow-test images (alpine, bash) keep their mirror name only: a second plain tag on the same image
# changes which tag the Docker registry-link logic picks and fails dockerflow TestImageRegistryURL.
# No kinds: every kind plus `flow`. Kinds: only those; `flow` is the pseudo-kind for the two
# mirror-only flow images. --print lists the refs `docker save` needs and pulls nothing.
# A failed pull is reported, never fatal: testcontainers pulls that image itself at test time.
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
# shellcheck source=./lib.sh
. "$SCRIPT_DIR/lib.sh"
MIRROR=mirror.gcr.io

pull_mirror_only() {
  _img="$MIRROR/$1"
  if docker image inspect "$_img" >/dev/null 2>&1; then
    echo "present $_img"
  elif retry 2 240 docker pull -q "$_img" >/dev/null 2>&1; then
    echo "pulled  $_img"
  else
    echo "FAILED  $_img (the flow tests pull it themselves)"
  fi
}

pull_one() {
  _img="$1"
  case "$_img" in
    */*) _mirrored="$MIRROR/$_img" ;;
    *) _mirrored="$MIRROR/library/$_img" ;;
  esac
  if docker image inspect "$_img" >/dev/null 2>&1; then
    echo "present $_img"
  elif retry 2 240 docker pull -q "$_mirrored" >/dev/null 2>&1 && docker tag "$_mirrored" "$_img"; then
    echo "pulled  $_img"
  else
    echo "FAILED  $_img (testcontainers will pull it from Docker Hub)"
  fi
}

# Flow-test pins (mirror-only). go-test-split.sh --pins-hash greps them from this line.
FLOW_IMAGES="library/alpine:3.20 library/bash:5.2"

PRINT=0
if [ "${1:-}" = "--print" ]; then
  PRINT=1
  shift
fi

WANT_FLOW=0
KINDS=""
if [ "$#" -eq 0 ]; then
  WANT_FLOW=1
else
  for _k in "$@"; do
    if [ "$_k" = flow ]; then
      WANT_FLOW=1
    else
      KINDS="$KINDS $_k"
    fi
  done
fi

IMAGES=""
if [ "$#" -eq 0 ] || [ -n "$KINDS" ]; then
  # $KINDS is deliberately word-split into kind names.
  # shellcheck disable=SC2086
  IMAGES="$(cd "$ROOT_DIR" && go run ./apps/kira-space/internal/adapters/testsupport/cmd/testimages $KINDS)"
fi

if [ "$PRINT" = 1 ]; then
  for _i in $IMAGES; do
    echo "$_i"
  done
  if [ "$WANT_FLOW" = 1 ]; then
    for _i in $FLOW_IMAGES; do
      echo "$MIRROR/$_i"
    done
  fi
  exit 0
fi

for _i in $IMAGES; do
  pull_one "$_i" &
done
if [ "$WANT_FLOW" = 1 ]; then
  for _i in $FLOW_IMAGES; do
    pull_mirror_only "$_i" &
  done
fi
wait
exit 0
