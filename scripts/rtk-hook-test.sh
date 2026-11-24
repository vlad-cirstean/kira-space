#!/bin/sh
# rtk-hook-test.sh: proves .claude/hooks/rtk-pretooluse.sh never exits non-zero and never prints
# junk, against fake rtk binaries (exit 2, exit 1, junk output, missing, full edition, passthrough).
# With RTK_REAL=<path to a pinned rtk-oss> it also checks a real rewrite passes through. Touches no
# global state: each case runs with PATH limited to its own temp dir plus system tools.
set -u
HOOK="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)/.claude/hooks/rtk-pretooluse.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
STDIN='{"tool_name":"Bash","tool_input":{"command":"git status"}}'
fail=0

# fake NAME VERSION_LINE PIPELINE_RC HOOK_BODY: writes $TMP/NAME/rtk
fake() {
  mkdir -p "$TMP/$1"
  cat >"$TMP/$1/rtk" <<EOF
#!/bin/sh
case "\$1" in
  --version) echo "$2"; exit 0 ;;
  pipeline) exit $3 ;;
  hook) $4 ;;
esac
exit 2
EOF
  chmod 755 "$TMP/$1/rtk"
}

# run NAME WANT_STDOUT: hook must exit 0 and print exactly WANT_STDOUT
run() {
  got="$(printf '%s' "$STDIN" | PATH="$TMP/$1:/usr/bin:/bin" sh "$HOOK" 2>"$TMP/err")"
  rc=$?
  if [ "$rc" -ne 0 ] || [ "$got" != "$2" ] || [ -s "$TMP/err" ]; then
    echo "FAIL $1: rc=$rc stdout='$got' stderr='$(cat "$TMP/err")', want rc=0 stdout='$2'"
    fail=1
  else
    echo "ok   $1"
  fi
}

REWRITE='{"hookSpecificOutput":{"updatedInput":{"command":"rtk git status"}}}'
fake exit2 "rtk 1.2.1" 1 'echo boom >&2; exit 2'
fake exit1 "rtk 1.2.1" 1 'exit 1'
fake junk "rtk 1.2.1" 1 'echo "not json"; exit 0'
fake junk-exit1 "rtk 1.2.1" 1 'echo "{partial"; exit 1'
fake foreign "rtk 0.9.0" 2 'exit 2'
fake noversion "usage: rtk" 2 'exit 2'
fake full "rtk 1.2.1" 0 "echo '$REWRITE'; exit 0"
fake ok "rtk 1.2.1" 1 "cat >/dev/null; echo '$REWRITE'; exit 0"
fake empty "rtk 1.2.1" 1 'cat >/dev/null; exit 0'
mkdir -p "$TMP/missing"

run exit2 ""
run exit1 ""
run junk ""
run junk-exit1 ""
run foreign ""
run noversion ""
run full ""
run missing ""
run ok "$REWRITE"
run empty ""

if [ -n "${RTK_REAL:-}" ]; then
  mkdir -p "$TMP/real"
  ln -s "$RTK_REAL" "$TMP/real/rtk"
  got="$(printf '%s' "$STDIN" | HOME="$TMP/home" PATH="$TMP/real:/usr/bin:/bin" sh "$HOOK")"
  rc=$?
  case "$got" in
    "{"*"rtk git status"*) [ "$rc" -eq 0 ] && echo "ok   real rtk-oss rewrite" || { echo "FAIL real: rc=$rc"; fail=1; } ;;
    *) echo "FAIL real: rc=$rc stdout='$got'"; fail=1 ;;
  esac
fi
exit "$fail"
