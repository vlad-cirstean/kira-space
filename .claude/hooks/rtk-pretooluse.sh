#!/bin/sh
# PreToolUse(Bash) hook: rtk-oss rewrites the command so its output reaches the agent filtered.
# Fails open: no rtk on PATH, or the BUSL full edition, leaves the command unchanged.
# Setup: scripts/rtk-setup.sh; docs/DEV_ENVIRONMENT.md "rtk".
command -v rtk >/dev/null 2>&1 || exit 0
rtk pipeline --help >/dev/null 2>&1 && exit 0
RTK_TELEMETRY_DISABLED=1 RTK_SUPPRESS_HOOK_WARNING=1 exec rtk hook claude
