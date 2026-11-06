#!/bin/sh
# sign-bundle.sh — P52 §10.1's four codesign lines, made real (P57 §4.13). P58f: there is no
# vendored node binary and no Kafka native module to sign individually any more — every adapter,
# Kafka included, is served in-process by native Go since P58e/P58f. One deep sign over the whole
# bundle is all that is left.
#
# Paths are <app dir>/build/darwin/Taskfile.yml's create:app:bundle output layout, not P52's — that
# plan explicitly warned its own paths were written from an earlier session and would need
# re-verifying against a real packaged bundle rather than copied. macOS only.
#
# Takes the app dir (relative to the repo root) and app name as $1/$2; the defaults are the one
# app, apps/kira-space/"Kira Space".
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
# shellcheck source=./lib.sh
. "$SCRIPT_DIR/lib.sh"

APP_DIR="${1:-apps/kira-space}"
APP_NAME="${2:-Kira Space}"

APP="$ROOT_DIR/$APP_DIR/bin/$APP_NAME.app"
DMG="$ROOT_DIR/$APP_DIR/bin/$APP_NAME.dmg"

if [ ! -d "$APP" ]; then
  echo "sign-bundle.sh: \"$APP\" not found — run 'cd $APP_DIR && wails3 task darwin:package:dmg' first" >&2
  exit 1
fi

if [ "$(uname -s)" != "Darwin" ]; then
  echo "sign-bundle.sh: codesign is macOS-only; nothing to do on $(uname -s)" >&2
  exit 1
fi

codesign --force --deep --sign - "$APP"
codesign --verify --deep --strict "$APP"

echo "sign-bundle.sh: signed and verified \"$APP\""

# P10: the shipped artifact is the .dmg, so it carries a signature of its own. The copy of the
# .app *inside* it is already signed — `create:app:bundle` runs `codesign:adhoc` before
# `create:dmg` copies the bundle in — and re-signing the outer .app above cannot reach that copy,
# which is why the order matters and why this signs the image rather than rebuilding it. A disk
# image is a flat file: `--deep` has nothing to recurse into, so it is deliberately absent here.
if [ -f "$DMG" ]; then
  codesign --force --sign - "$DMG"
  codesign --verify --strict "$DMG"
  echo "sign-bundle.sh: signed and verified \"$DMG\""
else
  echo "sign-bundle.sh: no \"$DMG\" to sign — 'wails3 task darwin:package' builds only the .app; 'darwin:package:dmg' builds both" >&2
fi
