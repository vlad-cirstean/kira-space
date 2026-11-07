#!/bin/sh
# PostCompact hook: compaction condenses CLAUDE.md down to a one-line gist in the summary, so the
# working agreement's actual rules (communication style, phase process, testing bar, etc.) are gone
# right after. Re-inject CLAUDE.md and the general file it imports verbatim rather than a hand-picked
# excerpt, since any excerpt goes stale the moment either file changes.
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
CLAUDE_MD="$ROOT/CLAUDE.md"
AGREEMENT="$ROOT/docs/WORKING_AGREEMENT.md"

if [ ! -f "$CLAUDE_MD" ] || [ ! -f "$AGREEMENT" ]; then
  exit 0
fi

cat "$CLAUDE_MD" "$AGREEMENT" | jq -Rs '{hookSpecificOutput: {hookEventName: "PostCompact", additionalContext: ("Full CLAUDE.md plus the docs/WORKING_AGREEMENT.md it imports, reloaded after compaction (their rules apply to chat replies too, not just docs/commits):\n\n" + .)}}'
