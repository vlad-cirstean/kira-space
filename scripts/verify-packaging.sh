#!/bin/sh
# verify-packaging.sh — the executable form of P15's "auto-update configuration verified"
# deliverable (git show bd0c224688d9:docs/v1/plans/P15-gh-tooling.md §2, §4). Runs on Linux and macOS, locally and in CI.
#
# Rewritten for the Wails/Go shell (P57 M7/§4.13): electron-builder.yml and every check that read
# it are gone — there is no updater feed format, no Electron fuses, no asar to unpack a native
# module out of. What survives is the *property* those checks protected (no *silent* auto-update —
# P119 adds a real one, but only on an explicit Update click, through scripts/install.sh — ad-hoc
# signed, correct bundle identity) reasserted against the Wails bundle's own layout.
#
# Static checks (S1, S2, S5) always run — S3/S4 were electron-builder.yml checks removed in the
# P57 rewrite; the numbering was never closed up, and check IDs stay as-is since docs/PACKAGING.md
# cross-references them. Artifact checks (A1, A3, A5, A6, A7, N2 over the .app; A4, N3 over the .dmg P10
# ships) run only when that artifact exists; otherwise they print "skipped" and pass.
#
# Every check runs before the script exits, so one run reports everything wrong, not just the
# first failure.
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
# shellcheck source=./lib.sh
. "$SCRIPT_DIR/lib.sh"
cd "$ROOT_DIR"

FAILED=0

fail() {
  echo "verify-packaging: $1 — $2" >&2
  FAILED=1
}

note() {
  echo "verify-packaging: note — $1"
}

# --- S1: no *silent* updater dependency -----------------------------------------------------
# P119's own update-and-install still goes through scripts/install.sh on an explicit click —
# electron-updater/update-electron-app are the silent, no-user-action kind this repo has never used.
if grep -qE '"(electron-updater|update-electron-app)"' package.json; then
  fail "updater dependency present" "package.json references electron-updater/update-electron-app; SPEC.md §1/§3 defer silent auto-update past v1"
fi

# --- S2: no silent-updater code in apps/ or packages/ ---------------------------------------
if grep -rnE "autoUpdater|electron-updater" apps/ packages/ >/dev/null 2>&1; then
  fail "updater code present" "apps/ or packages/ references autoUpdater or electron-updater"
fi

# --- S6: the packaged frontend bundle does not carry the Playwright debug hooks ----------------
# P29 F1/§2.1's trap: build/Taskfile.yml's build:frontend task fingerprints its `sources` (frontend
# excluding node_modules/dist), not `dist` itself, so `bun run test:ui` (build:test, hooks on) then
# `bun run package` with no intervening source edit would otherwise let Task's up-to-date check
# skip the rebuild and embed the hooks-enabled bundle. Checked against frontend/dist rather than
# the .app bundle so this also runs on Linux and before packaging.
# __kiraScrollTrace is deliberately excluded — its console.warn string literal survives in
# scrollTrace.ts's always-shipped note* half, so it would be a false positive; the five below
# appear nowhere outside main.ts's __KIRA_DEBUG_HOOKS__-gated block.
if [ -d apps/kira-space/frontend/dist/assets ]; then
  if grep -lE '__kiraCount|__kiraCacheStats|__kiraRetention|__kiraRetainedBytes|__kiraTreeConnectionIds' \
      apps/kira-space/frontend/dist/assets/*.js >/dev/null 2>&1; then
    fail "debug hooks in packaged bundle" "frontend/dist/assets/*.js carries the Playwright debug hooks — rebuild with 'bun run build', not 'build:test'"
  fi
else
  note "skipped S6 — apps/kira-space/frontend/dist/assets not present (run 'bun run build' first)"
fi

# --- S7: no un-gated debug global in main.ts ----------------------------------------------------
# Every window.__kira* assignment in main.ts must sit inside the `if (__KIRA_DEBUG_HOOKS__)` block
# (P29 F1) — this is the static check that would have caught window.__kiraGridEngine shipping
# unconditionally, and would catch the next hook added the same way.
#
# Finding 8 (round 2): the actual invariant is "nowhere in the file, full stop" — the awk pass
# below used to only scan lines BEFORE the gate (`NR < gate`), so a hook added AFTER the block's
# closing brace (arguably the more natural place to add one) passed silently, un-gated. Fixed by
# locating the block's own closing brace (the next line that is *exactly* `}` — a lone top-level
# brace, never `} from '...'`'s import-continuation shape or one of the block's own indented,
# nested closes) and flagging a window.__kira* assignment anywhere outside [gate, close], not just
# before it.
MAIN_TS="apps/kira-space/frontend/src/main.ts"
if [ ! -f "$MAIN_TS" ]; then
  fail "main.ts missing" "$MAIN_TS not found — this check needs updating along with it"
fi
# A file with no window.__kira* assignment needs no gate; one with any needs the block form.
for MAIN_FILE in "$MAIN_TS" apps/kira-space/frontend/src/studio/main.ts; do
  [ -f "$MAIN_FILE" ] || continue
  GATE_LINE="$(grep -n '^if (__KIRA_DEBUG_HOOKS__) {' "$MAIN_FILE" | head -1 | cut -d: -f1)"
  if [ -z "$GATE_LINE" ]; then
    if grep -qE '^[[:space:]]*window\.__kira' "$MAIN_FILE"; then
      fail "no debug-hook gate" "$MAIN_FILE assigns window.__kira* but has no top-level 'if (__KIRA_DEBUG_HOOKS__) {' block"
    fi
    continue
  fi
  UNGATED="$(awk -v gate="$GATE_LINE" '
    NR == gate { in_block = 1; next }
    in_block && $0 == "}" { in_block = 0; next }
    !in_block && /^[[:space:]]*window\.__kira/ { print NR": "$0 }
  ' "$MAIN_FILE")"
  if [ -n "$UNGATED" ]; then
    fail "un-gated debug global" "$MAIN_FILE assigns window.__kira* outside its __KIRA_DEBUG_HOOKS__ gate (block starts at line $GATE_LINE): $UNGATED"
  fi
done

# --- S8: no dev-mode env branch in shipped Go's own entry point --------------------------------
# apps/kira-space/main.go must read no os.Getenv at all — true after P29 F2 deleted the
# KIRA_G1_BLANK branch. A precise, low-false-positive invariant for the app's own entry point.
MAIN_GO="apps/kira-space/main.go"
if [ -f "$MAIN_GO" ]; then
  if grep -q 'os\.Getenv' "$MAIN_GO"; then
    fail "dev-mode env branch in main.go" "$MAIN_GO calls os.Getenv — the app's own entry point must read no env-driven dev/debug branch"
  fi
else
  fail "main.go missing" "$MAIN_GO not found — this check needs updating along with it"
fi

# --- S10: no app code downloads a release asset itself -------------------------------------
# Rescoped by P119 (was: apps/ packages/ only) — internal/appupdate now lives at internal/, and
# the invariant still holds for every line of app code: only scripts/install.sh, run by the user's
# own explicit Update click (never silently), is a sanctioned downloader. scripts/ is deliberately
# excluded from this scan for exactly that reason.
if grep -rnE 'browser_download_url|releases/download' apps/ packages/ internal/ >/dev/null 2>&1; then
  fail "release asset download present" "apps/, packages/ or internal/ references a release asset download outside scripts/install.sh"
fi

# --- S11: the curl installer and the Go installer agree on their one shared contract --------
# P119 §2/§3.3: scripts/install.sh and internal/appupdate/install.go hand off over a fd-3 line and
# a contract-marker comment neither side can drift from silently.
if [ ! -f scripts/install.sh ]; then
  fail "installer script missing" "scripts/install.sh not found"
elif ! sh -n scripts/install.sh; then
  fail "installer script invalid" "sh -n scripts/install.sh failed — see the syntax error above"
fi

SCRIPT_CONTRACT_LINE="$(sed -n '2p' scripts/install.sh 2>/dev/null || true)"
if [ "$SCRIPT_CONTRACT_LINE" != "# kira-install-contract: 1" ]; then
  fail "installer contract marker changed" "scripts/install.sh:2 is '$SCRIPT_CONTRACT_LINE', want '# kira-install-contract: 1'"
fi
GO_CONTRACT_COUNT="$(grep -c 'installContract *= *"# kira-install-contract: 1"' internal/appupdate/install.go 2>/dev/null || true)"
if [ "${GO_CONTRACT_COUNT:-0}" -ne 1 ]; then
  fail "installer contract marker out of sync" "internal/appupdate/install.go's installContract does not match scripts/install.sh's own contract line"
fi

INSTALL_URL_HITS="$(grep -rn 'raw.githubusercontent.com' apps/ packages/ internal/ \
  --include=*.go --include=*.ts --include=*.vue 2>/dev/null || true)"
INSTALL_URL_COUNT="$(printf '%s\n' "$INSTALL_URL_HITS" | grep -c . || true)"
case "$INSTALL_URL_HITS" in
  *"internal/appupdate/install.go"*"/vlad-cirstean/kira-space/main/scripts/install.sh"*) ;;
  *)
    fail "installer URL not pinned as expected" "expected exactly one raw.githubusercontent.com reference, in internal/appupdate/install.go's InstallScriptURL, pointing at /vlad-cirstean/kira-space/main/scripts/install.sh; got: $INSTALL_URL_HITS"
    ;;
esac
if [ "${INSTALL_URL_COUNT:-0}" -ne 1 ]; then
  fail "installer URL referenced more than once" "raw.githubusercontent.com should appear exactly once across apps/, packages/, internal/ (InstallScriptURL only); got $INSTALL_URL_COUNT hits: $INSTALL_URL_HITS"
fi

# --- S12: the bundled ONNX Runtime version agrees everywhere it is pinned (P210) ----------------
# internal/memory/embed/paths.go's ORTVersion names the dylib RuntimeLib loads from Frameworks; the
# fetch script downloads that version and the bundle task copies that file name.
ORT_GO_VERSION="$(sed -n 's/^const ORTVersion = "\([^"]*\)".*/\1/p' internal/memory/embed/paths.go 2>/dev/null | head -1)"
ORT_SCRIPT_VERSION="$(sed -n 's/^ORT_VERSION="\([^"]*\)".*/\1/p' scripts/fetch-onnxruntime.sh 2>/dev/null | head -1)"
if [ -z "$ORT_GO_VERSION" ] || [ "$ORT_GO_VERSION" != "$ORT_SCRIPT_VERSION" ]; then
  fail "ONNX Runtime version mismatch" "internal/memory/embed/paths.go ORTVersion ('$ORT_GO_VERSION') != scripts/fetch-onnxruntime.sh ORT_VERSION ('$ORT_SCRIPT_VERSION')"
fi
if ! grep -q "libonnxruntime\.$ORT_GO_VERSION\.dylib" apps/kira-space/build/darwin/Taskfile.yml 2>/dev/null; then
  fail "ONNX Runtime not bundled" "apps/kira-space/build/darwin/Taskfile.yml's create:app:bundle does not reference libonnxruntime.$ORT_GO_VERSION.dylib"
fi

# --- S13: Kira Space's document types match the file-type vocabulary (P332) --------------------
# Both Info plists carry a generated CFBundleDocumentTypes/UTImportedTypeDeclarations block; the
# generator's --check exits non-zero when it drifts from apps/kira-space/internal/filetypes.
if command -v go >/dev/null 2>&1; then
  if ! go run ./apps/kira-space/tools/gendoctypes --check; then
    fail "document types out of date" "Kira Space's Info plists differ from internal/filetypes — run 'go run ./apps/kira-space/tools/gendoctypes'"
  fi
else
  note "skipped S13 — go not on PATH"
fi

# --- S14: the app floor is macOS 26 in both bundle plists (P346) -------------------------------
for plist in apps/kira-space/build/darwin/Info.plist apps/kira-space/build/darwin/Info.dev.plist; do
  if ! grep -A1 '<key>LSMinimumSystemVersion</key>' "$plist" 2>/dev/null | grep -q '<string>26.0.0</string>'; then
    fail "macOS floor wrong" "$plist LSMinimumSystemVersion is not 26.0.0 (P346); re-narrow after regenerating build assets"
  fi
done

# --- S15: the app icon is a full-bleed Icon Composer layer named by CFBundleIconName (P349) ------
app=kira-space
svg="apps/$app/build/appicon.icon/Assets/kira_icon_vector.svg"
if tr '\n' ' ' <"$svg" | grep -Eq '<clipPath[^>]*>[^<]*<rect[^>]* rx='; then
  fail "icon layer pre-masked" "$svg has a rounded clipPath; the layer must be full-bleed, the system applies the mask (P349)"
fi
if ! grep -q 'matrix(1.242718' "$svg"; then
  fail "icon layer not full-bleed" "$svg lacks the matrix(1.242718 ...) scale that maps the artwork onto the full canvas (P349)"
fi
if command -v jq >/dev/null 2>&1; then
  jq -e '.fill' "apps/$app/build/appicon.icon/icon.json" >/dev/null 2>&1 || fail "icon fill missing" "apps/$app/build/appicon.icon/icon.json has no top-level fill (P349)"
elif ! grep -q '"fill"' "apps/$app/build/appicon.icon/icon.json"; then
  fail "icon fill missing" "apps/$app/build/appicon.icon/icon.json has no top-level fill (P349)"
fi
for plist in "apps/$app/build/darwin/Info.plist" "apps/$app/build/darwin/Info.dev.plist"; do
  if ! grep -A1 '<key>CFBundleIconName</key>' "$plist" 2>/dev/null | grep -q '<string>appicon</string>'; then
    fail "icon name missing" "$plist CFBundleIconName is not appicon (P349)"
  fi
done

if command -v shellcheck >/dev/null 2>&1; then
  if ! shellcheck -s sh scripts/install.sh; then
    fail "shellcheck failed" "shellcheck -s sh scripts/install.sh reported issues — see above"
  fi
else
  note "skipped shellcheck on scripts/install.sh — shellcheck not on PATH"
fi

# --- S5: the packaging scripts cannot publish -----------------------------------------------
# A POSIX `sed` read, not `node -p require(...)`: this repository does not declare `node` as a
# dependency anywhere (P58f deleted the vendored runtime), so a machine that satisfies every
# documented Requirement could still fail this script at line 1 before a single check ran (P20 F13).
PACKAGE_SCRIPT="$(sed -n 's/^[[:space:]]*"package": *"\(.*\)",\{0,1\}$/\1/p' package.json | head -1)"
case "$PACKAGE_SCRIPT" in
  # P10: the shipped artifact is the .dmg, so the script must run the task that builds one.
  # `darwin:package` alone stops at the .app and would leave the release with nothing to upload —
  # and it is a prefix of this string, so match the full task name, not a substring of it.
  *"wails3 task darwin:package:dmg"*) ;;
  *) fail "package script changed" "package.json's 'package' script no longer runs 'wails3 task darwin:package:dmg' — this check needs updating along with it" ;;
esac

# --- The one bundle: Kira Space.app and its disk image -----------------------------------------
APP="apps/kira-space/bin/Kira Space.app"
DMG="apps/kira-space/bin/Kira Space.dmg"

if [ ! -d "$APP" ]; then
  note "skipped A1/A3/A5/N2 — \"$APP\" not present (run 'bun run package' first)"
else
  # A1: ad-hoc signature.
  if command -v codesign >/dev/null 2>&1; then
    for target in "$APP"; do
      if [ -e "$target" ]; then
        if ! codesign -dv --verbose=2 "$target" 2>&1 | grep -q 'Signature=adhoc'; then
          fail "signature not ad-hoc" "codesign on \"$target\" did not report Signature=adhoc"
        fi
      else
        note "skipped the ad-hoc signature check for \"$target\" — not present"
      fi
    done
  else
    note "skipped A1 — codesign not available on this runner"
  fi

  # A3: bundle identifier.
  if command -v /usr/libexec/PlistBuddy >/dev/null 2>&1; then
    BUNDLE_ID="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$APP/Contents/Info.plist" 2>/dev/null || echo '')"
    if [ "$BUNDLE_ID" != "com.kirathecat.kira-space" ]; then
      fail "wrong bundle identifier" "CFBundleIdentifier is '$BUNDLE_ID', expected com.kirathecat.kira-space"
    fi
  else
    note "skipped A3 — PlistBuddy not available on this runner"
  fi

  # A5: the bundle reports the version build/config.yml holds.
  if command -v /usr/libexec/PlistBuddy >/dev/null 2>&1; then
    WANT_VERSION="$(sed -n 's/^  version: *"\([^"]*\)".*/\1/p' apps/kira-space/build/config.yml | head -1)"
    GOT_VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist" 2>/dev/null || echo '')"
    if [ "$GOT_VERSION" != "$WANT_VERSION" ]; then
      fail "wrong bundle version" "CFBundleShortVersionString is '$GOT_VERSION', expected '$WANT_VERSION' (apps/kira-space/build/config.yml's info.version)"
    fi
  else
    note "skipped A5 — PlistBuddy not available on this runner"
  fi

  # A6: minos 26.0 and SDK >= 26. A smaller SDK is a failure on CI (macos-26); only a note locally.
  if command -v vtool >/dev/null 2>&1; then
    LINKED_SDK="$(vtool -show-build "$APP/Contents/MacOS/Kira Space" 2>/dev/null | awk '$1=="sdk"{print $2}')"
    LINKED_MINOS="$(vtool -show-build "$APP/Contents/MacOS/Kira Space" 2>/dev/null | awk '$1=="minos"{print $2}')"
    if [ "${LINKED_SDK%%.*}" -ge 26 ] 2>/dev/null; then
      if [ "$LINKED_MINOS" != "26.0" ]; then
        fail "linked minos wrong" "linked minos is '$LINKED_MINOS' in $APP, expected 26.0"
      fi
    elif [ "${CI:-}" = "true" ]; then
      fail "linked SDK too old" "$APP links SDK '$LINKED_SDK' (< 26), minos '$LINKED_MINOS'; CI runs macos-26"
    else
      note "A6 warning — $APP links SDK '$LINKED_SDK' (< 26): minos '$LINKED_MINOS', smaller traffic lights; build with Xcode 26+ (CI runs macos-26)"
    fi
  else
    note "skipped A6 — vtool not available on this runner"
  fi

  # A7: Assets.car ships with the appicon icon, named by CFBundleIconName (P349).
  if [ ! -f "$APP/Contents/Resources/Assets.car" ]; then
    fail "Assets.car missing" "$APP/Contents/Resources/Assets.car not found; actool 26 compiles appicon.icon on macos-26 (P349)"
  fi
  if [ -x /usr/libexec/PlistBuddy ]; then
    if [ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIconName' "$APP/Contents/Info.plist" 2>/dev/null || echo '')" != "appicon" ]; then
      fail "icon name wrong" "CFBundleIconName in $APP is not appicon (P349)"
    fi
  fi
  if command -v xcrun >/dev/null 2>&1 && xcrun --sdk macosx --find assetutil >/dev/null 2>&1; then
    if ! xcrun --sdk macosx assetutil --info "$APP/Contents/Resources/Assets.car" 2>/dev/null | grep -q '"Name" : "appicon"'; then
      fail "icon asset missing" "Assets.car in $APP has no appicon asset (P349)"
    fi
  else
    note "skipped A7 assetutil — xcrun assetutil not available on this runner"
  fi

  # --- N2: the whole bundle verifies deep-signed -------------------------------------------------
  if command -v codesign >/dev/null 2>&1; then
    if ! codesign --verify --deep --strict "$APP" >/dev/null 2>&1; then
      fail "bundle does not verify" "codesign --verify --deep --strict \"$APP\" did not exit 0"
    fi
  else
    note "skipped N2 — codesign not available on this runner"
  fi
fi

# --- A4/N3: the shipped .dmg (only if it exists) --------------------------------------
if [ ! -f "$DMG" ]; then
  note "skipped A4/N3 — \"$DMG\" not present (run 'bun run package' first)"
elif ! command -v codesign >/dev/null 2>&1; then
  note "skipped A4/N3 — codesign not available on this runner"
else
  # A4: the image carries its own ad-hoc signature (scripts/sign-bundle.sh), same property A1
  # asserts for the bundle. No --deep: a disk image is a flat file, not a bundle tree.
  if ! codesign -dv --verbose=2 "$DMG" 2>&1 | grep -q 'Signature=adhoc'; then
    fail "dmg signature not ad-hoc" "codesign on \"$DMG\" did not report Signature=adhoc"
  fi
  # N3: and it verifies.
  if ! codesign --verify --strict "$DMG" >/dev/null 2>&1; then
    fail "dmg does not verify" "codesign --verify --strict \"$DMG\" did not exit 0"
  fi
fi

if [ "$FAILED" = "1" ]; then
  echo "verify-packaging: FAILED — see above" >&2
  exit 1
fi

echo "verify-packaging: all checks passed"
