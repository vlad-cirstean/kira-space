#!/bin/sh
# kira-install-contract: 1
# scripts/install.sh installs or updates Kira Space on this Mac: it downloads the latest published
# release's disk image, verifies it, and swaps it into /Applications, then opens the app.
# Interactive: `curl -fsSL <raw-url> | sh -s -- --app=space` — the same command installs a fresh
# copy or replaces an older one. Self-update: the app itself runs this script with
# --wait-pid/--notify-fd, stages the new bundle, hands off over fd 3, then quits once it reads that
# hand-off — the app never overwrites its own running binary. Log file, either mode:
# ~/Library/Logs/<App name>/install.log.
#
# Usage: install.sh --app=space [--wait-pid=<pid> --notify-fd=<3-9>]
# Exit codes: 0 installed and launched; 1 any failure; 2 usage; 130 interrupted.
#
# P119 §2: rewritten by hand, not generated — every requirement in the plan's own §2 is load-
# bearing, none of it optional polish. Never break the --app/--wait-pid/--notify-fd flags or the
# fd-3 "staged <tag>" handshake without bumping the line-2 marker above to 2 — an older app then
# refuses the new script with a clear error instead of misdriving it (internal/appupdate/
# install.go's own installContract constant is the other side of this same contract; S11 in
# scripts/verify-packaging.sh checks the two agree literally).
set -eu

main() {
  # ---- fixed environment (§2.2) --------------------------------------------------------------
  PATH=/usr/bin:/bin:/usr/sbin:/sbin
  export PATH
  umask 022

  RAW_BASE="https://raw.githubusercontent.com/vlad-cirstean/kira-studio/main/scripts/install.sh"

  # Every global this script touches gets a default here, up front — set -u then never trips on a
  # variable a later step hasn't reached yet, including inside cleanup, which can run at any point
  # once its trap is armed.
  APP_ARG="" WAIT_PID="" NOTIFY_FD="" LOG=""
  APP_NAME="" BUNDLE_ID="" ASSET="" SLUG="" LOG_NAME=""
  WORK="" STAGE="" MNT=""
  MOUNTED=0
  HANDED_OFF=0
  FAIL_MSG=""

  # ---- small helpers, defined before anything that could call them ------------------------------

  # log: interactive mode writes to stderr AND appends to $LOG; self-update mode writes stderr
  # only — the app already made stdout/stderr the log file itself, so appending again here would
  # duplicate every line (§2.1).
  log() {
    TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf '%s\n' "[$TS] $1" >&2
    if [ -z "$NOTIFY_FD" ]; then
      printf '%s\n' "[$TS] $1" >>"$LOG" 2>/dev/null || true
    fi
  }

  die() {
    FAIL_MSG="$1"
    log "ERROR: $1"
    exit 1
  }

  # cleanup is the only exit path (trap cleanup EXIT, §2.4). $? is captured first, before anything
  # else here can overwrite it.
  # Called only via trap cleanup EXIT below; shellcheck's static flow doesn't trace
  # trap-registered functions, so it misreads this as dead code.
  # shellcheck disable=SC2317,SC2329
  cleanup() {
    STATUS=$?

    if [ "$MOUNTED" = "1" ]; then
      hdiutil detach "$MNT" >/dev/null 2>&1 </dev/null \
        || hdiutil detach "$MNT" -force >/dev/null 2>&1 </dev/null || true
    fi
    [ -n "$WORK" ] && rm -rf "$WORK"
    # Only the exact mktemp path this run created, never a glob.
    [ -n "$STAGE" ] && rm -rf "$STAGE"

    # Non-zero status after the app already quit on our word (HANDED_OFF=1): the user has no app
    # open and no dialog, so this is the one place that reopens the app (if it's still there)
    # and shows a blocking alert. Before hand-off, the app itself is still running and shows the
    # failure in its own dialog (§5) — no alert here.
    if [ "$STATUS" != "0" ] && [ "$HANDED_OFF" = "1" ]; then
      if [ -d "/Applications/$APP_NAME.app" ]; then
        open "/Applications/$APP_NAME.app" >/dev/null 2>&1 </dev/null || true
        ALERT_MSG="$FAIL_MSG Details: ~/Library/Logs/$LOG_NAME/install.log"
      else
        ALERT_MSG="$FAIL_MSG Details: ~/Library/Logs/$LOG_NAME/install.log. To reinstall: curl -fsSL $RAW_BASE | sh -s -- --app=$APP_ARG"
      fi
      # argv-passed strings, never interpolated into the AppleScript source itself.
      osascript - "$APP_NAME couldn't update" "$ALERT_MSG" >/dev/null 2>&1 <<'APPLESCRIPT_EOF' || true
on run argv
  display alert (item 1 of argv) message (item 2 of argv)
end run
APPLESCRIPT_EOF
    fi

    exit "$STATUS"
  }

  trap 'exit 130' INT TERM HUP

  # ---- 1: parse args ----------------------------------------------------------------------------
  for ARG in "$@"; do
    case "$ARG" in
      --app=*) APP_ARG="${ARG#--app=}" ;;
      --wait-pid=*) WAIT_PID="${ARG#--wait-pid=}" ;;
      --notify-fd=*) NOTIFY_FD="${ARG#--notify-fd=}" ;;
      *)
        echo "usage: install.sh --app=space [--wait-pid=<pid> --notify-fd=<3-9>]" >&2
        exit 2
        ;;
    esac
  done

  case "$APP_ARG" in
    space)
      APP_NAME="Kira Space"
      BUNDLE_ID="com.kirathecat.kira-space"
      ASSET="kira-space-macos-arm64.dmg"
      SLUG="space"
      LOG_NAME="Kira Space"
      ;;
    *)
      echo "usage: install.sh --app=space [--wait-pid=<pid> --notify-fd=<3-9>]" >&2
      exit 2
      ;;
  esac

  # --wait-pid/--notify-fd are passed only by the app itself (self-update mode) — both or neither.
  # The regex on --notify-fd is what keeps the eval below safe: it is interpolated into a
  # redirection, and this is the only shape it can ever take by the time that eval runs.
  if [ -n "$WAIT_PID" ] || [ -n "$NOTIFY_FD" ]; then
    if [ -z "$WAIT_PID" ] || [ -z "$NOTIFY_FD" ]; then
      echo "usage: --wait-pid and --notify-fd must both be given, or neither" >&2
      exit 2
    fi
    case "$WAIT_PID" in
      '' | *[!0-9]*)
        echo "usage: --wait-pid must match ^[0-9]+\$" >&2
        exit 2
        ;;
    esac
    case "$NOTIFY_FD" in
      3 | 4 | 5 | 6 | 7 | 8 | 9) ;;
      *)
        echo "usage: --notify-fd must match ^[3-9]\$" >&2
        exit 2
        ;;
    esac
  fi

  LOG="$HOME/Library/Logs/$LOG_NAME/install.log"
  mkdir -p "$(dirname "$LOG")"
  chmod 700 "$(dirname "$LOG")" 2>/dev/null || true
  if [ -z "$NOTIFY_FD" ] && [ -f "$LOG" ]; then
    LOG_SIZE="$(wc -c <"$LOG" | tr -d ' ')"
    if [ "$LOG_SIZE" -gt 1048576 ]; then
      : >"$LOG"
    fi
  fi
  if [ -n "$NOTIFY_FD" ]; then
    MODE_LABEL="self-update"
  else
    MODE_LABEL="interactive"
  fi
  log "kira-install starting: mode=$MODE_LABEL app=$APP_ARG pid=$$"

  # ---- 2: preflight -------------------------------------------------------------------------
  # sysctl, not `uname -m` — a Rosetta shell reports x86_64 on Apple Silicon.
  if [ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" != "1" ]; then
    die "this Mac is not Apple Silicon (arm64) — $APP_NAME requires macOS 26+ on Apple Silicon"
  fi
  OS_MAJOR="$(sw_vers -productVersion | cut -d. -f1)"
  case "$OS_MAJOR" in
    '' | *[!0-9]*) die "couldn't read this Mac's macOS version" ;;
  esac
  if [ "$OS_MAJOR" -lt 26 ]; then
    die "macOS $OS_MAJOR is too old — $APP_NAME requires macOS 26 or later"
  fi
  if [ ! -d /Applications ] || [ ! -w /Applications ]; then
    die "/Applications isn't writable by this user — run as an admin user."
  fi

  # ---- 3: work dirs -------------------------------------------------------------------------
  WORK="$(mktemp -d "${TMPDIR:-/tmp}/kira-install.XXXXXX")"
  # Same volume as the target, so the final swap (step 11) is two rename(2)s, never a cross-device
  # copy.
  STAGE="$(mktemp -d "/Applications/.$SLUG-install.XXXXXX")"
  MNT="$WORK/mnt"
  mkdir -p "$MNT"

  # ---- 4: traps -----------------------------------------------------------------------------
  trap cleanup EXIT
  trap 'exit 130' INT TERM HUP

  # ---- 5: resolve release --------------------------------------------------------------------
  RELEASE_JSON="$WORK/release.json"
  set +e
  HTTP_STATUS="$(curl --proto '=https' --tlsv1.2 -fsSL --connect-timeout 20 --retry 3 --retry-delay 2 \
    -H 'Accept: application/vnd.github+json' -H 'X-GitHub-Api-Version: 2022-11-28' \
    -o "$RELEASE_JSON" -w '%{http_code}' \
    https://api.github.com/repos/vlad-cirstean/kira-studio/releases/latest </dev/null)"
  CURL_STATUS=$?
  set -e
  if [ "$CURL_STATUS" != "0" ]; then
    if [ "$HTTP_STATUS" = "404" ]; then
      die "no published release yet"
    fi
    die "couldn't reach GitHub to check for a release (curl exit $CURL_STATUS, HTTP $HTTP_STATUS)"
  fi

  # JXA (osascript -l JavaScript), not plutil/jq/python3: plutil rejects JSON nulls (GitHub's
  # release JSON has them), jq ships only from macOS 15, sed/grep over JSON is fragile, and
  # python3 on 14 is a stub that prompts to install the Command Line Tools. JXA ships with every
  # macOS since 10.10.
  cat >"$WORK/parse-release.js" <<'JXA_EOF'
function run(argv) {
  var jsonPath = argv[0];
  var assetName = argv[1];
  var app = Application.currentApplication();
  app.includeStandardAdditions = true;
  var raw = app.read(Path(jsonPath));
  var data = JSON.parse(raw);
  var tagName = (data.tag_name === undefined || data.tag_name === null) ? "" : data.tag_name;
  var draft = data.draft ? "true" : "false";
  var prerelease = data.prerelease ? "true" : "false";
  var url = "";
  var size = "";
  var digest = "";
  var assets = data.assets || [];
  for (var i = 0; i < assets.length; i++) {
    if (assets[i].name === assetName) {
      url = assets[i].browser_download_url || "";
      size = (assets[i].size === undefined || assets[i].size === null) ? "" : String(assets[i].size);
      digest = (assets[i].digest === undefined || assets[i].digest === null) ? "" : assets[i].digest;
      break;
    }
  }
  return [tagName, draft, prerelease, url, size, digest].join("\n");
}
JXA_EOF

  osascript -l JavaScript "$WORK/parse-release.js" "$RELEASE_JSON" "$ASSET" \
    >"$WORK/release.txt" </dev/null \
    || die "couldn't parse the release metadata"

  TAG="$(sed -n '1p' "$WORK/release.txt")"
  IS_DRAFT="$(sed -n '2p' "$WORK/release.txt")"
  IS_PRERELEASE="$(sed -n '3p' "$WORK/release.txt")"
  ASSET_URL="$(sed -n '4p' "$WORK/release.txt")"
  ASSET_SIZE="$(sed -n '5p' "$WORK/release.txt")"
  ASSET_DIGEST="$(sed -n '6p' "$WORK/release.txt")"

  # Same belt-and-braces as internal/appupdate.buildResult — releases/latest already excludes both.
  [ "$IS_DRAFT" = "false" ] || die "the latest release is a draft"
  [ "$IS_PRERELEASE" = "false" ] || die "the latest release is a prerelease"
  if ! expr "$TAG" : 'v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*$' >/dev/null; then
    die "release tag '$TAG' does not look like v<major>.<minor>.<patch>"
  fi
  case "$ASSET_URL" in
    "https://github.com/vlad-cirstean/kira-studio/releases/download/$TAG/"*) ;;
    *) die "release asset URL is not the expected GitHub download URL" ;;
  esac
  case "$ASSET_SIZE" in
    '' | *[!0-9]*) die "release asset '$ASSET' was not found on the latest release" ;;
  esac

  # ---- 6: download ----------------------------------------------------------------------------
  # --speed-limit/--speed-time: abort a stalled transfer instead of hanging forever.
  curl --proto '=https' --tlsv1.2 -fsSL --connect-timeout 20 --retry 3 --retry-delay 2 \
    --speed-limit 1024 --speed-time 60 \
    -o "$WORK/app.dmg" "$ASSET_URL" </dev/null \
    || die "couldn't download $APP_NAME"

  GOT_SIZE="$(wc -c <"$WORK/app.dmg" | tr -d ' ')"
  if [ "$GOT_SIZE" != "$ASSET_SIZE" ]; then
    die "downloaded file size ($GOT_SIZE bytes) does not match the release's reported size ($ASSET_SIZE bytes)"
  fi

  case "$ASSET_DIGEST" in
    sha256:*)
      WANT_HASH="${ASSET_DIGEST#sha256:}"
      GOT_HASH="$(shasum -a 256 "$WORK/app.dmg" </dev/null | awk '{print $1}')"
      if [ "$GOT_HASH" != "$WANT_HASH" ]; then
        die "downloaded file's sha256 does not match the release's reported digest"
      fi
      ;;
    *)
      die "the release reported no sha256 digest for $ASSET (got '$ASSET_DIGEST'); refusing to install unverified. Ad-hoc codesign below only detects corruption, not tampering"
      ;;
  esac

  # ---- 7: mount and copy ----------------------------------------------------------------------
  hdiutil attach -nobrowse -readonly -noautoopen -mountpoint "$MNT" "$WORK/app.dmg" \
    >/dev/null </dev/null \
    || die "couldn't mount the downloaded disk image"
  MOUNTED=1
  [ -d "$MNT/$APP_NAME.app" ] || die "$APP_NAME.app was not found on the downloaded disk image"
  # ditto, not cp -R: preserves symlinks, xattrs, and the code signature.
  ditto "$MNT/$APP_NAME.app" "$STAGE/$APP_NAME.app" </dev/null \
    || die "couldn't copy $APP_NAME.app from the disk image"
  hdiutil detach "$MNT" >/dev/null </dev/null || die "couldn't unmount the downloaded disk image"
  MOUNTED=0

  # ---- 8: verify staged bundle ------------------------------------------------------------------
  if ! codesign --verify --deep --strict "$STAGE/$APP_NAME.app" >/dev/null 2>&1 </dev/null; then
    die "codesign verification failed for the downloaded $APP_NAME.app"
  fi
  GOT_ID="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' \
    "$STAGE/$APP_NAME.app/Contents/Info.plist" 2>/dev/null || echo '')"
  if [ "$GOT_ID" != "$BUNDLE_ID" ]; then
    die "downloaded app's bundle id ('$GOT_ID') does not match the expected '$BUNDLE_ID'"
  fi
  WANT_VERSION="${TAG#v}"
  GOT_VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' \
    "$STAGE/$APP_NAME.app/Contents/Info.plist" 2>/dev/null || echo '')"
  if [ "$GOT_VERSION" != "$WANT_VERSION" ]; then
    die "downloaded app's version ('$GOT_VERSION') does not match the release tag ('$WANT_VERSION')"
  fi

  # ---- 9: hand off (self-update mode only) -------------------------------------------------------
  if [ -n "$NOTIFY_FD" ]; then
    eval "printf 'staged %s\\n' \"\$TAG\" >&$NOTIFY_FD"
    eval "exec $NOTIFY_FD>&-"
    HANDED_OFF=1

    WAITED=0
    while kill -0 "$WAIT_PID" 2>/dev/null; do
      if [ "$WAITED" -ge 60 ]; then
        die "$APP_NAME didn't quit"
      fi
      sleep 1
      WAITED=$((WAITED + 1))
    done
  fi

  # ---- 10: running check ----------------------------------------------------------------------
  # Process name is the bundle executable.
  if pgrep -xq "$APP_NAME" </dev/null; then
    if [ -n "$NOTIFY_FD" ]; then
      die "$APP_NAME was reopened before the update finished"
    fi
    die "Quit $APP_NAME first, then rerun."
  fi

  # ---- 11: swap (critical section) --------------------------------------------------------------
  trap '' INT TERM HUP
  if [ -d "/Applications/$APP_NAME.app" ]; then
    mv "/Applications/$APP_NAME.app" "$STAGE/previous.app" \
      || die "couldn't move the installed $APP_NAME.app aside"
  fi

  set +e
  MV_ERR="$(mv "$STAGE/$APP_NAME.app" "/Applications/$APP_NAME.app" 2>&1 >/dev/null)"
  MV_STATUS=$?
  set -e
  if [ "$MV_STATUS" != "0" ]; then
    if [ -d "$STAGE/previous.app" ]; then
      mv "$STAGE/previous.app" "/Applications/$APP_NAME.app" 2>/dev/null || true
    fi
    trap 'exit 130' INT TERM HUP
    case "$MV_ERR" in
      *"Operation not permitted"*)
        die "couldn't install $APP_NAME.app into /Applications: Operation not permitted. On macOS 13+, allow this under System Settings > Privacy & Security > App Management, then try again."
        ;;
      *)
        die "couldn't install $APP_NAME.app into /Applications: $MV_ERR"
        ;;
    esac
  fi
  # At no instant does a partial bundle sit at the final path: it is either the old complete
  # bundle, absent for one rename gap, or the new complete, verified bundle. previous.app (if any)
  # goes with $STAGE in cleanup.
  trap 'exit 130' INT TERM HUP

  # ---- 12: launch -----------------------------------------------------------------------------
  open "/Applications/$APP_NAME.app" </dev/null >/dev/null 2>&1 \
    || die "$APP_NAME installed but couldn't be opened automatically — open it from /Applications"
  log "installed $TAG"
  exit 0
}

main "$@"
