# go.sh — sourced by run.sh. Per-package gremlins loop; see docs/DEV_ENVIRONMENT.md.
# Expects run.sh's SNAP, RUN_DIR, WORKERS, TARGETS, CHANGED, TOOLS_DIR, ROOT_DIR, unit_done.

GREMLINS_VERSION="v0.6.0"

# Test infrastructure and generated code; mutants there say nothing about product tests.
go_excluded() {
  case "$1" in
    internal/layeringtest | internal/testx | apps/kira-space/internal/adapters/testsupport | \
      apps/kira-space/internal/ipcfixture | apps/kira-space/internal/page/wire | \
      apps/kira-space/internal/gitwire)
      return 0 ;;
  esac
  return 1
}

ensure_gremlins() {
  require_cmd go "install Go first (your OS package manager, or go.dev), then re-run"
  _want="gremlins $GREMLINS_VERSION go$(go_directive)"
  _stamp="$TOOLS_DIR/bin/gremlins.stamp"
  if [ -x "$TOOLS_DIR/bin/gremlins" ] && [ "$(cat "$_stamp" 2>/dev/null)" = "$_want" ]; then
    return
  fi
  echo "mutation: installing gremlins $GREMLINS_VERSION"
  mkdir -p "$TOOLS_DIR/bin"
  GOBIN="$TOOLS_DIR/bin" GOTOOLCHAIN="go$(go_directive)" \
    go install "github.com/go-gremlins/gremlins/cmd/gremlins@$GREMLINS_VERSION"
  printf '%s\n' "$_want" >"$_stamp"
}

# Files in dir $1 (relative to SNAP) whose //go:build line requires darwin.
go_darwin_files() {
  for _f in "$SNAP/$1"/*.go; do
    [ -f "$_f" ] || continue
    case "$_f" in *_test.go) continue ;; esac
    case "$_f" in *_darwin.go) basename "$_f"; continue ;; esac
    _line="$(grep -m1 '^//go:build ' "$_f" || true)"
    case "$_line" in
      *'!darwin'*) ;;
      *darwin*) basename "$_f" ;;
    esac
  done
}

# Regex-escape a file name for gremlins -E.
go_re() { printf '%s' "$1" | sed 's/[.]/\\./g'; }

mutation_go() {
  ensure_gremlins
  mkdir -p "$RUN_DIR/go"
  cd "$SNAP"

  if [ -n "$TARGETS" ]; then
    _pkgs=""
    for _t in $TARGETS; do _pkgs="$_pkgs ${_t#./}"; done
  else
    _pkgs="$(go list -e -f '{{if or .TestGoFiles .XTestGoFiles}}{{.Dir}}{{end}}' \
      ./internal/... ./apps/kira-space/internal/... |
      sed "s|^$SNAP/||" | sort)"
  fi

  _changed_files=""
  if [ -n "$CHANGED" ]; then
    _changed_files="$(git -C "$ROOT_DIR" diff --name-only "$CHANGED" -- '*.go')"
  fi

  for _rel in $_pkgs; do
    _rel="${_rel%/}"
    go_excluded "$_rel" && continue
    _slug="$(printf '%s' "$_rel" | sed 's|/|__|g')"
    unit_done go "$_slug" && continue

    _excl=""
    _darwin="$(go_darwin_files "$_rel")"
    _excl="-E / -E _darwin\\.go\$"
    for _d in $_darwin; do _excl="$_excl -E (^|/)$(go_re "$_d")\$"; done

    if [ -n "$CHANGED" ]; then
      _mine="$(printf '%s\n' "$_changed_files" | grep -E "^$_rel/[^/]+\.go\$" || true)"
      if [ -z "$_mine" ]; then continue; fi
      # Mutate only changed files: exclude every other non-test file of the package.
      for _f in "$SNAP/$_rel"/*.go; do
        _b="$(basename "$_f")"
        case "$_b" in *_test.go) continue ;; esac
        printf '%s\n' "$_mine" | grep -qx "$_rel/$_b" && continue
        _excl="$_excl -E (^|/)$(go_re "$_b")\$"
      done
    fi

    echo "mutation[go]: $_rel"
    _t0="$(date +%s.%N)"
    _status=ok
    _reason=""
    if ! go test -count=1 "./$_rel" >"$RUN_DIR/go/$_slug.log" 2>&1; then
      _status=red
      _reason="$(grep -m1 -e '--- FAIL' -e '^FAIL' -e 'panic' "$RUN_DIR/go/$_slug.log" || echo 'go test failed')"
    fi
    _t1="$(date +%s.%N)"
    _suite="$(awk -v a="$_t0" -v b="$_t1" 'BEGIN{d=b-a; if(d<0.05)d=0.05; printf "%.3f", d}')"
    # Timeout coefficient scales the coverage-run time; floor the effective timeout at 60s.
    _coef="$(awk -v t="$_suite" 'BEGIN{m=3*t; if(m<60)m=60; c=int((m+t-0.0001)/t); if(c<3)c=3; print c}')"

    if [ "$_status" = ok ]; then
      _attempt=1
      while :; do
        # shellcheck disable=SC2086
        if "$TOOLS_DIR/bin/gremlins" unleash -o "$RUN_DIR/go/$_slug.json" --workers "$WORKERS" \
          --timeout-coefficient "$_coef" $_excl "./$_rel" >"$RUN_DIR/go/$_slug.log" 2>&1; then
          break
        fi
        if [ "$_attempt" -ge 2 ]; then
          _status=red
          _reason="$(grep -m1 -i -e 'fail' -e 'error' "$RUN_DIR/go/$_slug.log" || echo 'gremlins failed')"
          break
        fi
        _attempt=2
      done
    fi
    _t2="$(date +%s.%N)"
    jq -n --arg pkg "$_rel" --arg status "$_status" --arg reason "$_reason" \
      --arg darwin "$_darwin" --argjson suite "$_suite" --argjson coef "$_coef" \
      --argjson sec "$(awk -v a="$_t0" -v b="$_t2" 'BEGIN{printf "%.1f", b-a}')" \
      '{pkg: $pkg, status: $status, reason: $reason, suiteSec: $suite, timeoutCoefficient: $coef,
        durationSec: $sec, darwinFiles: ($darwin | split("\n") | map(select(. != "")))}' \
      >"$RUN_DIR/go/$_slug.info.json"
  done
  cd "$ROOT_DIR"
}
