#!/bin/sh
# kira-install-contract: 2
# scripts/install.sh installs or updates Kira Space on this Mac: it downloads the latest published
# release's disk image (or the release named by --tag), verifies it, and swaps it into /Applications, then opens the app.
# Releases resolve from the release index asset of the latest release (kira-space-releases.txt: tag,
# size and sha256 of every eligible release), never from the rate-limited GitHub REST API, except
# while the latest release predates the index (docs/PACKAGING.md §7).
# Interactive: `curl -fsSL <raw-url> | sh` — the same command installs a fresh
# copy or replaces an older one. Self-update: the app itself runs this script with
# --wait-pid/--notify-fd, stages the new bundle, hands off over fd 3, then quits once it reads that
# hand-off — the app never overwrites its own running binary. Log file, either mode:
# ~/Library/Logs/<App name>/install.log.
#
# Usage: install.sh [--tag=vX.Y.Z] [--wait-pid=<pid> --notify-fd=<3-9>]
# Exit codes: 0 installed and launched; 1 any failure; 2 usage; 130 interrupted.
#
# P119 §2: rewritten by hand, not generated — every requirement in the plan's own §2 is load-
# bearing, none of it optional polish. Never break the --wait-pid/--notify-fd flags or the
# fd-3 "staged <tag>" handshake without bumping the line-2 marker above (marker 2 dropped --app;
# adding an optional flag, like --tag, needs no bump; removing or changing one does) —
# an older app then refuses the new script with a clear error instead of misdriving it (internal/appupdate/
# install.go's own installContract constant is the other side of this same contract; S11 in
# scripts/verify-packaging.sh checks the two agree literally).
set -eu

# Test seam: scripts/check-install-resolve.ts sources this file with KIRA_INSTALL_SOURCE_ONLY=1 and
# points GH_WEB, GH_API and CURL_PROTO at a local fake GitHub. Only main sets them for a real run,
# to hard-coded https URLs; no env var changes where a real run fetches from.
#
# Globals the functions below read: GH_WEB GH_API CURL_PROTO INDEX_ASSET ASSET APP_NAME WORK LOG
# NOTIFY_FD. They set: TAG ASSET_SIZE ASSET_DIGEST FAIL_MSG.

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

# ---- release index ----------------------------------------------------------------------------
# The index (internal/releaseline index.go) lists every eligible release, one line each:
#   v0.3.3 kira-space-macos-arm64.dmg 43912345 sha256:<64 lowercase hex>
# Order is never trusted. The awk below avoids {n} intervals and other gawk-isms: macOS ships BWK awk.

# index_check FILE: exit 0 only for a well-formed index (the same rules as releaseline.ParseIndex).
index_check() {
  awk '
    BEGIN { FS = "[ ]" }
    NR == 1 { if ($0 != "# kira-release-index 1") bad = 1; next }
    {
      n++
      if (NF != 4) { bad = 1; next }
      if ($1 !~ /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/) bad = 1
      if ($2 == "") bad = 1
      if ($3 !~ /^[1-9][0-9]*$/ || length($3) > 15) bad = 1
      if (length($4) != 71 || $4 !~ /^sha256:[0-9a-f]+$/) bad = 1
      if ($1 in seen) bad = 1
      seen[$1] = 1
    }
    END { exit ((bad || n == 0) ? 1 : 0) }
  ' "$1"
}

# index_max FILE: the highest tag with a $ASSET line.
index_max() {
  awk -v a="$ASSET" '
    NR > 1 && $2 == a {
      split(substr($1, 2), p, ".")
      x = p[1] + 0; y = p[2] + 0; z = p[3] + 0
      if (!have || x > ma || (x == ma && (y > mb || (y == mb && z > mc)))) {
        have = 1; ma = x; mb = y; mc = z; tag = $1
      }
    }
    END { if (have) print tag }
  ' "$1"
}

# index_entry FILE TAG: "<size> <digest>" of the release's $ASSET, or nothing.
index_entry() {
  awk -v t="$2" -v a="$ASSET" 'NR > 1 && $1 == t && $2 == a { print $3, $4; exit }' "$1"
}

# index_next_step FILE CURRENT: the tag to install for a running version, or nothing. The awk twin of
# releaseline.NextStep and of the JXA nextStep it replaced: the last patch of the installed minor when
# newer, else the lowest release of the next minor; only lines for $ASSET count, like Eligible. CURRENT parses like semver: optional v, optional
# -prerelease (sorts below its own version), +build ignored. scripts/check-install-step.ts (S16)
# replays internal/releaseline/testdata/next_step_index.json through it.
index_next_step() {
  awk -v cur="$2" -v asset="$ASSET" '
    BEGIN {
      s = cur
      sub(/^v/, "", s)
      i = index(s, "+")
      if (i > 0) {
        if (substr(s, i) !~ /^\+[0-9A-Za-z.-]+$/) exit
        s = substr(s, 1, i - 1)
      }
      i = index(s, "-")
      if (i > 0) {
        if (substr(s, i) !~ /^-[0-9A-Za-z.-]+$/) exit
        pre = 1
        s = substr(s, 1, i - 1)
      }
      if (s !~ /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/) exit
      split(s, p, ".")
      ca = p[1] + 0; cb = p[2] + 0; cc = p[3] + 0
      ok = 1
    }
    NR > 1 && ok && $2 == asset {
      split(substr($1, 2), p, ".")
      a = p[1] + 0; b = p[2] + 0; c = p[3] + 0
      if (a == ca && b == cb) {
        if (!own || c > ownTop) { own = 1; ownTop = c }
      } else if (a > ca || (a == ca && b > cb)) {
        if (!nx || a < na || (a == na && (b < nb || (b == nb && c < nc)))) { nx = 1; na = a; nb = b; nc = c }
      }
    }
    END {
      if (!ok) exit
      if (own && (ownTop > cc || (ownTop == cc && pre))) print "v" ca "." cb "." ownTop
      else if (nx) print "v" na "." nb "." nc
    }
  ' "$1"
}

# fetch_index: $WORK/index.txt from the latest release (a web redirect, no API rate limit). 404 means
# the latest release predates the index: fetch_api_index. Any other failure dies without the API.
fetch_index() {
  INDEX_FILE="$WORK/index.txt"
  set +e
  HTTP_STATUS="$(curl --proto "$CURL_PROTO" --proto-redir "$CURL_PROTO" --tlsv1.2 -fsSL \
    --connect-timeout 20 --retry 3 --retry-delay 2 --max-filesize 1048576 \
    -o "$INDEX_FILE" -w '%{http_code}' \
    "$GH_WEB/releases/latest/download/$INDEX_ASSET" </dev/null)"
  CURL_STATUS=$?
  set -e
  if [ "$CURL_STATUS" = "0" ]; then
    index_check "$INDEX_FILE" || die "release index is malformed"
  elif [ "$HTTP_STATUS" = "404" ]; then
    fetch_api_index
  else
    die "couldn't reach GitHub to read the release index (curl exit $CURL_STATUS, HTTP $HTTP_STATUS)"
  fi
}

# fetch_api_index: transitional, only while the latest release carries no index (v0.3.21 and
# older). Lists releases over the REST API (unauthenticated: 60 requests/hour/IP) and converts them
# to index text. Remove with the v0.4 minor bump (docs/PACKAGING.md §7).
fetch_api_index() {
  log "release index not found on the latest release; using the GitHub API"
  PAGE=1
  while [ "$PAGE" -le 10 ]; do
    rm -f "$WORK/api-headers.txt"
    # No --retry: curl retries 429, which only spends more of the budget.
    set +e
    HTTP_STATUS="$(curl --proto "$CURL_PROTO" --proto-redir "$CURL_PROTO" --tlsv1.2 -fsSL \
      --connect-timeout 20 \
      -H 'Accept: application/vnd.github+json' -H 'X-GitHub-Api-Version: 2022-11-28' \
      -D "$WORK/api-headers.txt" -o "$WORK/releases-$PAGE.json" -w '%{http_code}' \
      "$GH_API/releases?per_page=100&page=$PAGE" </dev/null)"
    CURL_STATUS=$?
    set -e
    if [ "$CURL_STATUS" != "0" ]; then
      case "$HTTP_STATUS" in
        403 | 429)
          LIMIT_NOTE=""
          if grep -qi '^x-ratelimit-remaining: *0' "$WORK/api-headers.txt" 2>/dev/null; then
            LIMIT_NOTE=" x-ratelimit-remaining: 0,"
          fi
          die "GitHub API rate limit reached for this network (HTTP $HTTP_STATUS,$LIMIT_NOTE unauthenticated). The latest release predates the release index; try again in an hour."
          ;;
      esac
      die "couldn't list releases (curl exit $CURL_STATUS, HTTP $HTTP_STATUS)"
    fi
    # A page with fewer than 100 releases is the last one.
    PAGE_COUNT="$(grep -o '"tag_name"' "$WORK/releases-$PAGE.json" | wc -l | tr -d ' ')"
    [ "$PAGE_COUNT" -ge 100 ] || break
    PAGE=$((PAGE + 1))
  done
  [ "$PAGE" -le 10 ] || PAGE=10
  # JXA (osascript -l JavaScript), not plutil/jq/python3: plutil rejects JSON nulls (GitHub's
  # release JSON has them), jq ships only from macOS 15, sed/grep over JSON is fragile, and
  # python3 on 14 is a stub that prompts to install the Command Line Tools. JXA ships with every
  # macOS since 10.10. apiToIndex follows releaseline.FormatIndex; S16 checks they agree.
  cat >"$WORK/api-index.js" <<'API_INDEX_JXA_EOF'
function hasAsset(r, name) {
  var assets = r.assets || [];
  for (var i = 0; i < assets.length; i++) {
    var a = assets[i];
    if (a.name === name && a.state === "uploaded" && a.size > 0) return a;
  }
  return null;
}
function cmpVer(a, b) {
  for (var i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] < b[i] ? -1 : 1;
  }
  return 0;
}
// The index text for a releases listing, "" when no release is eligible (the Go twin: releaseline.FormatIndex).
function apiToIndex(rels, asset) {
  var rows = [];
  var seen = {};
  for (var i = 0; i < rels.length; i++) {
    var r = rels[i];
    var m = /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.exec(r.tag_name);
    if (!m || r.draft || r.prerelease) continue;
    var a = hasAsset(r, asset);
    if (!a) continue;
    if (seen[r.tag_name]) throw new Error("duplicate release " + r.tag_name);
    seen[r.tag_name] = true;
    if (!/^sha256:[0-9a-f]{64}$/.test(a.digest || "")) {
      throw new Error("release " + r.tag_name + ": asset " + asset + " has no sha256 digest");
    }
    rows.push({ tag: r.tag_name, v: [Number(m[1]), Number(m[2]), Number(m[3])], size: a.size, digest: a.digest });
  }
  if (rows.length === 0) return "";
  rows.sort(function (x, y) { return cmpVer(x.v, y.v); });
  var out = "# kira-release-index 1\n";
  for (var j = 0; j < rows.length; j++) {
    out += rows[j].tag + " " + asset + " " + rows[j].size + " " + rows[j].digest + "\n";
  }
  return out;
}
function run(argv) {
  var app = Application.currentApplication();
  app.includeStandardAdditions = true;
  var rels = [];
  for (var p = 1; p <= Number(argv[2]); p++) {
    rels = rels.concat(JSON.parse(app.read(Path(argv[1] + "/releases-" + p + ".json"))));
  }
  // osascript appends its own newline.
  return apiToIndex(rels, argv[0]).replace(/\n$/, "");
}
API_INDEX_JXA_EOF
  osascript -l JavaScript "$WORK/api-index.js" "$ASSET" "$WORK" "$PAGE" >"$INDEX_FILE" </dev/null \
    || die "couldn't read the release list from the GitHub API"
  [ -s "$INDEX_FILE" ] || die "no published release yet"
  index_check "$INDEX_FILE" || die "release index is malformed"
}

# resolve_release REQ_TAG CUR_VERSION SELF_UPDATE: sets TAG, ASSET_SIZE and ASSET_DIGEST.
# Interactive installs take the newest release. A self-update without --tag (apps up to 0.3.14)
# steps one minor at a time, else the newest.
resolve_release() {
  fetch_index
  if [ -n "$1" ]; then
    TAG="$1"
  else
    TAG=""
    if [ "$3" = "1" ]; then
      if [ -n "$2" ]; then
        TAG="$(index_next_step "$INDEX_FILE" "$2")"
      fi
      if [ -n "$TAG" ]; then
        log "stepping from $2 to $TAG"
      else
        log "no update step found from '$2'; taking the latest release"
      fi
    fi
    if [ -z "$TAG" ]; then
      TAG="$(index_max "$INDEX_FILE")"
    fi
    [ -n "$TAG" ] || die "no published release yet"
  fi
  ENTRY="$(index_entry "$INDEX_FILE" "$TAG")"
  [ -n "$ENTRY" ] || die "release $TAG is not a published release (not in the release index)"
  ASSET_SIZE="${ENTRY% *}"
  ASSET_DIGEST="${ENTRY#* }"
}

# download_release: $WORK/app.dmg for $TAG, checked against the index's size and sha256.
download_release() {
  # --speed-limit/--speed-time: abort a stalled transfer instead of hanging forever.
  set +e
  HTTP_STATUS="$(curl --proto "$CURL_PROTO" --proto-redir "$CURL_PROTO" --tlsv1.2 -fsSL \
    --connect-timeout 20 --retry 3 --retry-delay 2 --speed-limit 1024 --speed-time 60 \
    -o "$WORK/app.dmg" -w '%{http_code}' \
    "$GH_WEB/releases/download/$TAG/$ASSET" </dev/null)"
  CURL_STATUS=$?
  set -e
  if [ "$CURL_STATUS" != "0" ]; then
    if [ "$HTTP_STATUS" = "404" ]; then
      die "release $TAG is not a published release (its disk image is gone, HTTP 404)"
    fi
    die "couldn't download $APP_NAME (curl exit $CURL_STATUS, HTTP $HTTP_STATUS)"
  fi

  GOT_SIZE="$(wc -c <"$WORK/app.dmg" | tr -d ' ')"
  if [ "$GOT_SIZE" != "$ASSET_SIZE" ]; then
    die "downloaded file size ($GOT_SIZE bytes) does not match the release's reported size ($ASSET_SIZE bytes)"
  fi

  WANT_HASH="${ASSET_DIGEST#sha256:}"
  GOT_HASH="$(shasum -a 256 "$WORK/app.dmg" </dev/null | awk '{print $1}')"
  if [ "$GOT_HASH" != "$WANT_HASH" ]; then
    die "downloaded file's sha256 does not match the release's reported digest"
  fi
}

main() {
  # ---- fixed environment (§2.2) --------------------------------------------------------------
  PATH=/usr/bin:/bin:/usr/sbin:/sbin
  export PATH
  umask 022

  RAW_BASE="https://raw.githubusercontent.com/vlad-cirstean/kira-space/main/scripts/install.sh"

  # Every global this script touches gets a default here, up front — set -u then never trips on a
  # variable a later step hasn't reached yet, including inside cleanup, which can run at any point
  # once its trap is armed.
  WAIT_PID="" NOTIFY_FD="" LOG="" REQ_TAG="" TAG="" ASSET_SIZE="" ASSET_DIGEST=""
  # Kira Space is the only app.
  APP_NAME="Kira Space"
  BUNDLE_ID="com.kirathecat.kira-space"
  ASSET="kira-space-macos-arm64.dmg"
  INDEX_ASSET="kira-space-releases.txt"
  GH_WEB="https://github.com/vlad-cirstean/kira-space"
  GH_API="https://api.github.com/repos/vlad-cirstean/kira-space"
  CURL_PROTO="=https"
  SLUG="space"
  LOG_NAME="Kira Space"
  WORK="" STAGE="" MNT=""
  MOUNTED=0
  HANDED_OFF=0
  FAIL_MSG=""

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
        ALERT_MSG="$FAIL_MSG Details: ~/Library/Logs/$LOG_NAME/install.log. To reinstall: curl -fsSL $RAW_BASE | sh"
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
      --wait-pid=*) WAIT_PID="${ARG#--wait-pid=}" ;;
      --notify-fd=*) NOTIFY_FD="${ARG#--notify-fd=}" ;;
      --tag=*) REQ_TAG="${ARG#--tag=}" ;;
      *)
        echo "usage: install.sh [--tag=vX.Y.Z] [--wait-pid=<pid> --notify-fd=<3-9>]" >&2
        exit 2
        ;;
    esac
  done

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

  # --tag is interpolated into the release URL below, so only this shape ever gets there.
  if [ -n "$REQ_TAG" ] && ! expr "$REQ_TAG" : 'v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*$' >/dev/null; then
    echo "usage: --tag must match ^v[0-9]+\.[0-9]+\.[0-9]+\$" >&2
    exit 2
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
  log "kira-install starting: mode=$MODE_LABEL pid=$$"

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

  # ---- 4b: step one minor at a time (self-update without --tag) -------------------------------
  # Apps up to 0.3.14 run this script without --tag, so it would install the newest release and skip
  # the intermediate minors' migrations. resolve_release applies the updater's stepping rule
  # (internal/releaseline NextStep) through index_next_step, which scripts/verify-packaging.sh (S16)
  # holds to internal/releaseline/testdata/next_step_index.json. Newer apps always pass --tag, so
  # this only ever serves the older ones. Interactive mode is a clean install or a deliberate
  # reinstall and keeps taking the newest release.
  CUR_VERSION="" SELF_UPDATE=0
  if [ -n "$NOTIFY_FD" ] && [ -z "$REQ_TAG" ]; then
    SELF_UPDATE=1
    CUR_VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' \
      "/Applications/$APP_NAME.app/Contents/Info.plist" 2>/dev/null || echo '')"
  fi

  # ---- 5: resolve release --------------------------------------------------------------------
  resolve_release "$REQ_TAG" "$CUR_VERSION" "$SELF_UPDATE"

  # ---- 6: download ----------------------------------------------------------------------------
  download_release

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

[ "${KIRA_INSTALL_SOURCE_ONLY:-}" = 1 ] || main "$@"
