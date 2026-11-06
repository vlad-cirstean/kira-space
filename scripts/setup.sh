#!/bin/sh
# setup.sh — the one entry point for everything a fresh clone (or a dev/package loop) needs before
# building: the Bun workspace, the Go module, the pinned `wails3` CLI, and — only when the CLI or
# toolchain identity has actually changed — a bindings regeneration. Wired as `predev`/`prepackage`.
#
# Absorbs install-deps.sh + wails-dev-setup.sh (P20 D6): the two were always invoked as a pair
# (this file's own callers; README.md's script table already described them as one step) and
# duplicated the ROOT_DIR idiom, a `command -v` preflight and the GOPATH/bin bootstrap between
# them. P58f: there is no vendored Node runtime or bundled engine to check for any more — every
# adapter is served in-process by native Go.
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
# shellcheck source=./lib.sh
. "$SCRIPT_DIR/lib.sh"

require_cmd bun "install it from https://bun.sh"
require_cmd go "install Go first (your OS package manager, or go.dev), then re-run"

echo "setup: bun install (workspace root + every apps/*/frontend)"
(cd "$ROOT_DIR" && bun install)

echo "setup: go mod download"
(cd "$ROOT_DIR" && retry 3 0 go mod download)

# Saved before ensure_gopath_on_path patches $PATH for the rest of this process (below) — the
# "add this to your shell profile" hint further down needs the *pre-patch* value, or it can never
# fire (P12 round 2 finding #15): checking the already-patched $PATH always finds GOPATH/bin on it.
ORIGINAL_PATH="$PATH"
ensure_gopath_on_path

PINNED_VERSION="$(pinned_wails_version)"
if [ -z "$PINNED_VERSION" ]; then
  echo "setup: could not read the wails/v3 version from go.mod" >&2
  exit 1
fi
GO_DIRECTIVE="$(go_directive)"
if [ -z "$GO_DIRECTIVE" ]; then
  echo "setup: could not read the go directive from go.mod" >&2
  exit 1
fi

# a < b, for dotted numeric versions with no pre-release suffix (exactly what `go.mod`'s own `go`
# directive and `go version -m`'s toolchain both are). Plain POSIX arithmetic rather than `sort -V`,
# which BSD/macOS `sort` doesn't support.
version_lt() {
  a1=$(echo "$1" | cut -d. -f1); a2=$(echo "$1" | cut -d. -f2); a3=$(echo "$1" | cut -d. -f3)
  b1=$(echo "$2" | cut -d. -f1); b2=$(echo "$2" | cut -d. -f2); b3=$(echo "$2" | cut -d. -f3)
  a2=${a2:-0}; a3=${a3:-0}; b2=${b2:-0}; b3=${b3:-0}
  [ "$a1" -lt "$b1" ] && return 0
  [ "$a1" -gt "$b1" ] && return 1
  [ "$a2" -lt "$b2" ] && return 0
  [ "$a2" -gt "$b2" ] && return 1
  [ "$a3" -lt "$b3" ]
}

INSTALLED_VERSION=""
INSTALLED_TOOLCHAIN=""
if command -v wails3 >/dev/null 2>&1; then
  # `wails3 version` writes to stderr, not stdout — confirmed by direct experiment.
  INSTALLED_VERSION="$(wails3 version 2>&1 1>/dev/null | head -1 | tr -d '[:space:]')"
  INSTALLED_TOOLCHAIN="$(go version -m "$(command -v wails3)" 2>/dev/null | awk 'NR==1{print $NF}')"
  INSTALLED_TOOLCHAIN="${INSTALLED_TOOLCHAIN#go}"
fi

# Reinstall when the pinned version changed, or when the installed binary's own build toolchain is
# older than go.mod's directive (P20 F7): `GOTOOLCHAIN=auto go install` resolves the toolchain from
# the *target module's* own directive, not this repo's, so an unpinned install silently degrades
# the bindings generator's type-checker (52 spurious "requires newer Go version" warnings,
# measured) even when the CLI version itself is already correct.
NEED_INSTALL=0
if [ "$INSTALLED_VERSION" != "$PINNED_VERSION" ]; then
  NEED_INSTALL=1
elif [ -n "$INSTALLED_TOOLCHAIN" ] && version_lt "$INSTALLED_TOOLCHAIN" "$GO_DIRECTIVE"; then
  NEED_INSTALL=1
fi

if [ "$NEED_INSTALL" = "1" ]; then
  if [ -n "$INSTALLED_VERSION" ]; then
    echo "setup: wails3 $INSTALLED_VERSION (toolchain go$INSTALLED_TOOLCHAIN) installed, go.mod pins $PINNED_VERSION (go$GO_DIRECTIVE) — reinstalling"
  else
    echo "setup: wails3 not found — installing $PINNED_VERSION"
  fi
  if [ "$(uname -s)" = "Linux" ] && ! pkg-config --exists gtk4 webkitgtk-6.0 2>/dev/null; then
    echo "setup: the wails3 CLI needs GTK4/WebKitGTK dev headers to build on Linux (even though" >&2
    echo "the app itself targets macOS) — install them first, then re-run:" >&2
    echo "  sudo apt-get install -y libgtk-4-dev libwebkitgtk-6.0-dev pkg-config" >&2
    exit 1
  fi
  # GOTOOLCHAIN pinned explicitly to go.mod's own directive — never `auto` (picks the target
  # module's floor, go1.26.8 here) and never `local` (this container's base toolchain, go1.24.7,
  # is below Wails' own floor and `local` refuses outright).
  retry 3 0 env GOTOOLCHAIN="go$GO_DIRECTIVE" go install "github.com/wailsapp/wails/v3/cmd/wails3@$PINNED_VERSION"
  case ":$ORIGINAL_PATH:" in
    *":$(go env GOPATH)/bin:"*) ;;
    *)
      echo "setup: add this to your shell profile so wails3 stays on PATH:" >&2
      echo "  export PATH=\"\$PATH:$(go env GOPATH)/bin\"" >&2
      ;;
  esac
  INSTALLED_TOOLCHAIN="$GO_DIRECTIVE"
fi

# Bindings regeneration is delegated to the task itself (P3 D11's preferred branch, P20 D8) —
# `common:generate:bindings` has a correct content checksum (P20 D3) over its own Go/TS sources
# (confirmed directly: touching a Go source file this task reads flips it from "up to date" to a
# real ~7s regen; reverting it drops back to a ~0.2s no-op run), so it's called unconditionally
# below, every setup.sh run. It's the sources it can't see that need a stamp: the wails3 CLI's own
# identity, and whether its build toolchain matches go.mod's directive (P20 F5/F6/F7). Task's
# per-task checksum cache survives a CLI reinstall, so on its own a "same sources, new/reinstalled
# CLI" run reports "up to date" and skips regenerating even though a different generator binary can
# produce different output for identical input — the stale-toolchain bug P20 F5/F6/F7 names. Wiping
# Task's cache on an identity change forces the unconditional call below to do a real regen instead
# of trusting a checksum computed under the old binary.
#
# One app dir: the `typecheck:*` scripts are raw `tsgo`/`vue-tsc` invocations that bypass the
# Taskfile's own `generate:bindings` dep, so a fresh clone's first `bun run typecheck` needs the
# bindings pre-generated here.
for APP_DIR in apps/kira-space; do
  BINDINGS_DIR="$ROOT_DIR/$APP_DIR/frontend/bindings"
  STAMP_FILE="$ROOT_DIR/$APP_DIR/.task/bindings.stamp"
  STAMP="$PINNED_VERSION|$GO_DIRECTIVE|$INSTALLED_TOOLCHAIN"
  if [ ! -d "$BINDINGS_DIR" ] || [ ! -f "$STAMP_FILE" ] || [ "$(cat "$STAMP_FILE")" != "$STAMP" ]; then
    rm -rf "$ROOT_DIR/$APP_DIR/.task"
    mkdir -p "$(dirname "$STAMP_FILE")"
    printf '%s' "$STAMP" >"$STAMP_FILE"
  fi
  echo "setup: wails3 task common:generate:bindings ($APP_DIR)"
  (cd "$ROOT_DIR/$APP_DIR" && wails3 task common:generate:bindings)
done
