#!/bin/sh
# fetch-onnxruntime.sh — downloads the pinned Microsoft.ML.OnnxRuntime NuGet package for Kira Space's
# local memory embeddings (P210) and extracts one platform's library into
# apps/kira-space/build/onnxruntime/<platform>/, checksum-verified.
#
# Usage: scripts/fetch-onnxruntime.sh [osx-arm64|linux-x64]   (default osx-arm64, the shipped slice)
#   osx-arm64  the dylib the macOS bundle task copies into Contents/Frameworks
#   linux-x64  development and tests only: KIRA_ORT_LIB=<printed path>
#
# Lives in scripts/: S10 scans only apps/, packages/ and internal/ for release-asset downloads.
# Idempotent. The version is pinned here and in internal/memory/embed/paths.go; verify-packaging.sh
# S12 keeps them equal.
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
# shellcheck source=./lib.sh
. "$SCRIPT_DIR/lib.sh"

ORT_VERSION="1.31.1"
# Microsoft.ML.OnnxRuntime from nuget.org: one package holds every platform's library, and nuget.org
# never replaces a published version (P673: the GitHub release tgz returned 404).
ARCHIVE_SHA256="6871ccbc97f3d5bdb4095a1dcec33aa78d56f6ed8de5980ef5fa1d9f2e7c679a"
PLATFORM="${1:-osx-arm64}"

case "$PLATFORM" in
  osx-arm64)
    SRC="runtimes/osx-arm64/native/libonnxruntime.dylib"
    LIB="libonnxruntime.$ORT_VERSION.dylib"
    ;;
  linux-x64)
    SRC="runtimes/linux-x64/native/libonnxruntime.so"
    LIB="libonnxruntime.so.$ORT_VERSION"
    ;;
  *)
    echo "fetch-onnxruntime: unknown platform '$PLATFORM' (want osx-arm64 or linux-x64)" >&2
    exit 1
    ;;
esac

DEST="$ROOT_DIR/apps/kira-space/build/onnxruntime/$PLATFORM"
STAMP="$DEST/.archive-sha256"

if [ -f "$DEST/$LIB" ] && [ -f "$STAMP" ] && [ "$(cat "$STAMP")" = "$ARCHIVE_SHA256" ]; then
  echo "$DEST/$LIB"
  exit 0
fi

require_cmd curl "install curl"
require_cmd unzip "install unzip"
if command -v sha256sum >/dev/null 2>&1; then
  hash_file() { sha256sum "$1" | cut -d' ' -f1; }
else
  hash_file() { shasum -a 256 "$1" | cut -d' ' -f1; }
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM

URL="https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/$ORT_VERSION/microsoft.ml.onnxruntime.$ORT_VERSION.nupkg"
PKG="$TMP/onnxruntime.nupkg"
if ! curl -fsSL -o "$PKG" "$URL"; then
  echo "fetch-onnxruntime: download failed: $URL" >&2
  exit 1
fi
GOT="$(hash_file "$PKG")"
if [ "$GOT" != "$ARCHIVE_SHA256" ]; then
  echo "fetch-onnxruntime: checksum mismatch for $URL: got $GOT, want $ARCHIVE_SHA256" >&2
  exit 1
fi

unzip -q "$PKG" "$SRC" LICENSE ThirdPartyNotices.txt -d "$TMP/x"
rm -rf "$DEST"
mkdir -p "$DEST"
cp "$TMP/x/$SRC" "$DEST/$LIB"
cp "$TMP/x/LICENSE" "$TMP/x/ThirdPartyNotices.txt" "$DEST/"
printf '%s' "$ARCHIVE_SHA256" >"$STAMP"
echo "$DEST/$LIB"
