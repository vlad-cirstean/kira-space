#!/bin/sh
# generate-wire.sh — regenerates the FlatBuffers data-plane wire code (P11) from
# packages/shared/protocol/wire.fbs. Fetches a pinned flatc into a gitignored .tools/ cache
# (verified by SHA-256) rather than assuming one is on PATH, the same reasoning setup.sh applies to
# wails3: an unpinned compiler is exactly how generated code drifts from what's
# committed. Run via `bun run generate:wire` whenever wire.fbs changes; the generated output is
# committed to the repo (P11 D11), so this script's job is to reproduce it byte-for-byte, not to
# run automatically on every build.
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
# shellcheck source=./lib.sh
. "$SCRIPT_DIR/lib.sh"

SCHEMA="$ROOT_DIR/packages/shared/protocol/wire.fbs"
GIT_SCHEMA="$ROOT_DIR/packages/git-ipc/schema/gitwire.fbs"

FLATC_VERSION=25.9.23
TOOLS_DIR="$ROOT_DIR/.tools/flatc-$FLATC_VERSION"

if [ -n "${FLATC:-}" ]; then
  : # explicit override, trust the caller
elif [ -x "$TOOLS_DIR/flatc" ]; then
  FLATC="$TOOLS_DIR/flatc"
else
  case "$(uname -s)" in
    Linux)
      case "$(uname -m)" in
        x86_64)
          ASSET="Linux.flatc.binary.g++-13.zip"
          SHA256="de0c6ad114a5a686ecf64322528c602c7d4512446a93f290f54f00ee5abea487"
          ;;
        *)
          echo "generate-wire: no prebuilt flatc for Linux $(uname -m) — build flatc $FLATC_VERSION" >&2
          echo "yourself and set FLATC=/path/to/flatc, then re-run" >&2
          exit 1
          ;;
      esac
      ;;
    Darwin)
      case "$(uname -m)" in
        arm64)
          ASSET="Mac.flatc.binary.zip"
          SHA256="1e14d2feade6d109fa9c102e6e5ead68f325ed3da1d3022ce08d3222f828d983"
          ;;
        x86_64)
          ASSET="MacIntel.flatc.binary.zip"
          SHA256="7a1de9cd4d0e769a39c41f3c59496bd011bc7a94d97baa58b0df8df782dc5c8d"
          ;;
        *)
          echo "generate-wire: no prebuilt flatc for Darwin $(uname -m) — build flatc $FLATC_VERSION" >&2
          echo "yourself and set FLATC=/path/to/flatc, then re-run" >&2
          exit 1
          ;;
      esac
      ;;
    *)
      echo "generate-wire: no prebuilt flatc for $(uname -s) — build flatc $FLATC_VERSION yourself" >&2
      echo "and set FLATC=/path/to/flatc, then re-run" >&2
      exit 1
      ;;
  esac

  require_cmd curl "install it (your OS package manager, or curl.se), then re-run"
  require_cmd unzip "install it (your OS package manager), then re-run"

  mkdir -p "$TOOLS_DIR"
  ARCHIVE="$TOOLS_DIR/$ASSET"
  echo "generate-wire: downloading flatc $FLATC_VERSION ($ASSET)"
  curl -fsSL -o "$ARCHIVE" \
    "https://github.com/google/flatbuffers/releases/download/v$FLATC_VERSION/$ASSET"

  # P12 round 2 finding #9: was `sha256sum`, which doesn't exist on macOS — this app's only
  # supported platform — so every run there without a cached flatc printed a false "SHA-256
  # mismatch" (empty actual value, `sha256sum: command not found` masked by `set -eu` not
  # catching a piped command's real exit status) and deleted the archive, never having verified
  # anything.
  ACTUAL_SHA256="$(sha256_file "$ARCHIVE")"
  if [ "$ACTUAL_SHA256" != "$SHA256" ]; then
    echo "generate-wire: SHA-256 mismatch for $ASSET" >&2
    echo "  expected: $SHA256" >&2
    echo "  actual:   $ACTUAL_SHA256" >&2
    rm -f "$ARCHIVE"
    exit 1
  fi

  unzip -oq "$ARCHIVE" -d "$TOOLS_DIR"
  chmod +x "$TOOLS_DIR/flatc"
  FLATC="$TOOLS_DIR/flatc"
fi

ACTUAL_VERSION="$("$FLATC" --version | awk '{print $NF}')"
if [ "$ACTUAL_VERSION" != "$FLATC_VERSION" ]; then
  echo "generate-wire: $FLATC reports version $ACTUAL_VERSION, expected $FLATC_VERSION" >&2
  exit 1
fi

echo "generate-wire: using $FLATC (version $ACTUAL_VERSION)"

# --gen-onefile and --ts-no-import-ext, as written in the P11 plan's §5.1, don't match this
# pinned flatc's actual CLI: --gen-onefile is a boolean presence flag (no `=false` form; omitting
# it already gives one-file-per-type, which is what's wanted here) and --ts-no-import-ext doesn't
# exist in 25.9.23 at all. Both are simply dropped below; see the C1 commit message.
"$FLATC" --go -o "$ROOT_DIR/apps/kira-space/internal/page" "$SCHEMA"
"$FLATC" --ts -o "$ROOT_DIR/packages/shared/protocol" "$SCHEMA"

"$FLATC" --go -o "$ROOT_DIR/apps/kira-space/internal" "$GIT_SCHEMA"
"$FLATC" --ts -o "$ROOT_DIR/packages/git-ipc/src/generated" "$GIT_SCHEMA"

echo "generate-wire: done"
