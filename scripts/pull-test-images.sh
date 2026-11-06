#!/bin/sh
# pull-test-images.sh — pulls every Testcontainers image through mirror.gcr.io in parallel and
# re-tags it to the plain name (Docker Hub blob CDN is blocked in the sandbox; the mirror is not).
# Flow-test images (alpine, bash) keep their mirror name only: a second plain tag on the same image
# changes which tag the Docker registry-link logic picks and fails dockerflow TestImageRegistryURL.
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

IMAGES="$(cd "$ROOT_DIR" && go run ./apps/kira-space/internal/adapters/testsupport/cmd/testimages)"

for _i in $IMAGES; do
  pull_one "$_i" &
done
pull_mirror_only library/alpine:3.20 &
pull_mirror_only library/bash:5.2 &
wait
exit 0
