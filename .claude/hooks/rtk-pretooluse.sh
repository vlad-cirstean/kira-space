#!/bin/sh
# PreToolUse(Bash) hook: rtk-oss rewrites the command so its output reaches the agent filtered.
# Fails open: any problem (no rtk, full edition, foreign or too-old rtk, non-zero exit, output that
# is not JSON) exits 0 silently and leaves the command unchanged. Exit 2 would block the tool call.
# Setup: scripts/rtk-setup.sh; docs/DEV_ENVIRONMENT.md "rtk". Test: scripts/rtk-hook-test.sh.
command -v rtk >/dev/null 2>&1 || exit 0
case "$(rtk --version </dev/null 2>/dev/null)" in
  "rtk "*) ;;
  *) exit 0 ;;
esac
rtk pipeline --help </dev/null >/dev/null 2>&1 && exit 0
out="$(RTK_TELEMETRY_DISABLED=1 RTK_SUPPRESS_HOOK_WARNING=1 rtk hook claude 2>/dev/null)" || exit 0
case "$out" in
  "{"*) printf '%s\n' "$out" ;;
esac
exit 0
