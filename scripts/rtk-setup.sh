#!/bin/sh
# rtk-setup.sh — installs rtk-oss (https://github.com/rtk-ai/rtk), the Apache-2.0 edition, so the
# committed PreToolUse hook (.claude/hooks/rtk-pretooluse.sh) can filter Bash output for the agent.
# The default `rtk` edition links a BUSL-1.1 engine; this script never installs it. Run once per
# machine (the binary is user-global, worktrees share it), never as a SessionStart hook. Pinned
# version and per-target SHA-256 from the release's checksums.txt; no `curl | sh`, no `rtk init`.
# Upgrade recipe: docs/DEV_ENVIRONMENT.md "rtk".
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
# shellcheck source=./lib.sh
. "$SCRIPT_DIR/lib.sh"

RTK_VERSION=1.2.1
INSTALL_DIR="${RTK_INSTALL_DIR:-$HOME/.local/bin}"
BIN="$INSTALL_DIR/rtk"

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64)
    TARGET=x86_64-unknown-linux-musl
    SHA256=44aa460c4827d206d11405c652832c9f7082edccfbc3a9f15a69a851e8a9e8b4
    ;;
  Linux-aarch64 | Linux-arm64)
    TARGET=aarch64-unknown-linux-musl
    SHA256=b01860ae2d3f44e0f7fc4431a95dd66b29b4bd87ec7f5e60aff13681b05db5dd
    ;;
  Darwin-arm64)
    TARGET=aarch64-apple-darwin
    SHA256=5a37e5e8e016b559597024900e7b21cff7a0015aa189ac6ac80d461cbb4be65b
    ;;
  Darwin-x86_64)
    TARGET=x86_64-apple-darwin
    SHA256=5e39ee8c4613c798f8d62868d4851bacca5aefcc134301052841e458ed71c797
    ;;
  *)
    echo "rtk-setup: unsupported platform $(uname -s)-$(uname -m)" >&2
    exit 1
    ;;
esac

# The oss build has no `pipeline` subcommand; the full edition does. `--version` alone cannot tell
# them apart.
is_oss() {
  case "$("$1" --version 2>/dev/null)" in
    "rtk "*) ;;
    *) return 1 ;;
  esac
  if "$1" pipeline --help >/dev/null 2>&1; then
    return 1
  fi
  return 0
}

is_pinned_oss() {
  [ "$("$1" --version 2>/dev/null)" = "rtk $RTK_VERSION" ] && is_oss "$1"
}

if [ -e "$BIN" ] && is_pinned_oss "$BIN"; then
  echo "rtk-setup: rtk-oss $RTK_VERSION already at $BIN"
else
  if [ -e "$BIN" ] && ! is_oss "$BIN"; then
    echo "rtk-setup: $BIN is not an rtk-oss build (full BUSL edition or a foreign binary, e.g. the crates.io 'rtk'); remove it, then re-run" >&2
    exit 1
  fi
  require_cmd curl "install curl, then re-run"
  require_cmd tar "install tar, then re-run"
  TMP_DIR="$(mktemp -d)"
  trap 'rm -rf "$TMP_DIR"' EXIT
  ARCHIVE="$TMP_DIR/rtk-oss-$TARGET.tar.gz"
  curl -fsSL -o "$ARCHIVE" "https://github.com/rtk-ai/rtk/releases/download/v$RTK_VERSION/rtk-oss-$TARGET.tar.gz"
  GOT="$(sha256_file "$ARCHIVE")"
  if [ "$GOT" != "$SHA256" ]; then
    echo "rtk-setup: SHA-256 mismatch for rtk-oss-$TARGET.tar.gz: got $GOT, pinned $SHA256" >&2
    exit 1
  fi
  if [ "$(tar -tzf "$ARCHIVE" | sort | tr '\n' ' ')" != "LICENSE rtk " ]; then
    echo "rtk-setup: archive must hold exactly rtk and LICENSE" >&2
    exit 1
  fi
  tar -xzf "$ARCHIVE" -C "$TMP_DIR" rtk
  mkdir -p "$INSTALL_DIR"
  mv "$TMP_DIR/rtk" "$BIN"
  chmod 755 "$BIN"
  if ! is_pinned_oss "$BIN"; then
    echo "rtk-setup: installed $BIN is not rtk-oss $RTK_VERSION" >&2
    exit 1
  fi
fi

RESOLVED="$(command -v rtk 2>/dev/null || true)"
if [ "$RESOLVED" != "$BIN" ]; then
  echo "rtk-setup: PATH resolves rtk to '${RESOLVED:-nothing}', not $BIN; put $INSTALL_DIR first on PATH" >&2
  exit 1
fi

# Telemetry is consent-gated and off by default; record the refusal explicitly. Never `telemetry enable`.
RTK_TELEMETRY_DISABLED=1 "$BIN" telemetry disable >/dev/null
if ! RTK_TELEMETRY_DISABLED=1 "$BIN" telemetry status | grep -Eq 'enabled: *no'; then
  echo "rtk-setup: telemetry still enabled after 'rtk telemetry disable'" >&2
  exit 1
fi

echo "rtk-setup: rtk-oss $RTK_VERSION at $BIN, telemetry off"
