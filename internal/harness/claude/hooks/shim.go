package hooks

import (
	"fmt"
	"path/filepath"
	"strings"
)

// statusLineThrottleSeconds bounds how often the statusline mode forwards one terminal's payload.
const statusLineThrottleSeconds = 30

// buildShim renders the shim script every hook event runs — curlPath is resolved once by
// New (exec.LookPath("curl")) and written in verbatim, so a PATH difference inside the launched
// session can never change which binary runs. Always exits 0: a non-zero exit from a PreToolUse
// hook blocks the agent's own tool call, and this shim only reports. stderr is discarded for the
// same reason — a stale socket must not print into the agent's transcript on every tool call.
//
// Refuses — a hard error, not a best-effort escape — curlPath or sockPath containing a double
// quote or a newline: an argv element that cannot be double-quoted safely. Neither an
// exec.LookPath result nor a mkdtemp-derived path ever contains either; the check exists so a
// future change here cannot silently produce a broken command.
func buildShim(curlPath, sockPath string) (string, error) {
	for _, arg := range []string{curlPath, sockPath} {
		if strings.ContainsAny(arg, "\"\n") {
			return "", fmt.Errorf("claude hooks: shim argument cannot be safely quoted: %q", arg)
		}
	}
	stampDir := filepath.Dir(sockPath)
	if strings.ContainsAny(stampDir, "\"\n") {
		return "", fmt.Errorf("claude hooks: shim argument cannot be safely quoted: %q", stampDir)
	}
	return fmt.Sprintf(`#!/bin/sh
# Kira agent hook. Always exits 0: a non-zero exit from a PreToolUse hook blocks the agent's own
# tool call. It reports, and prints the server's reply (empty except a Stop block or prompt context). stderr is discarded for the same reason — a stale socket
# must not print into the agent's transcript on every tool call.
exec 2>/dev/null
if [ "$1" = statusline ]; then
  # Statusline mode: forward rate_limits (throttled), then run the user's own statusline command so
  # its output still renders. Exits with that command's status.
  payload=$(cat)
  if [ -n "$KIRA_AGENT_HOOK_TOKEN" ] && [ -n "$KIRA_TERMINAL_ID" ]; then
    case "$payload" in *'"rate_limits"'*)
      case "$KIRA_TERMINAL_ID" in */*) ;; *)
        stamp="%[3]s/sl-$KIRA_TERMINAL_ID"
        now=$(date +%%s)
        last=$(cat "$stamp" 2>/dev/null)
        case "$last" in ''|*[!0-9]*) last=0 ;; esac
        if [ $((now - last)) -ge %[4]d ]; then
          printf '%%s' "$now" > "$stamp"
          (printf '%%s' "$payload" | "%[1]s" --silent --max-time 1 --output /dev/null \
            --unix-socket "%[2]s" \
            --header "Authorization: Bearer $KIRA_AGENT_HOOK_TOKEN" \
            --header "X-Kira-Terminal: $KIRA_TERMINAL_ID" \
            --header "Content-Type: application/json" \
            --data-binary @- \
            http://localhost/statusline) >/dev/null 2>&1 &
        fi ;;
      esac ;;
    esac
  fi
  if [ -n "$KIRA_USER_STATUSLINE" ]; then
    printf '%%s' "$payload" | sh -c "$KIRA_USER_STATUSLINE"
    exit $?
  fi
  exit 0
fi
[ -n "$KIRA_AGENT_HOOK_TOKEN" ] && [ -n "$KIRA_TERMINAL_ID" ] || exit 0
"%[1]s" --silent --max-time 2 --output - \
  --unix-socket "%[2]s" \
  --header "Authorization: Bearer $KIRA_AGENT_HOOK_TOKEN" \
  --header "X-Kira-Terminal: $KIRA_TERMINAL_ID" \
  --header "Content-Type: application/json" \
  --data-binary @- \
  http://localhost/hook
exit 0
`, curlPath, sockPath, stampDir, statusLineThrottleSeconds), nil
}
