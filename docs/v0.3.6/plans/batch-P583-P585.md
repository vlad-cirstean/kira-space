# Batch P583, P584, P585

Streams, each a git worktree off the chapter HEAD at batch start.

| Stream | Phase | Worktree | Branch |
|---|---|---|---|
| A | P583 Agent worktree: existing branch | `/home/user/kira-wt-p583` | `stream/p583` |
| B | P584 Readable YAML file and step names | `/home/user/kira-wt-p584` | `stream/p584` |
| C | P585 Share repos by origin | `/home/user/kira-wt-p585` | `stream/p585` |

Ordering risk: P584 (step and workflow file names) and P585 (repos in config sync) both touch `internal/configsync`. Planners run first and list their files. Orchestrator compares the lists before any implementer starts. Large or semantic overlap means P585 runs after P584, sequentially, not in parallel.

Shared-file candidates (additive hunks only): `docs/ARCHITECTURE.md`, copy catalog (`packages/theme/src/i18n/en.json`), `docs/v0.3.6/SPEC.md` (own row), `bridge/index.ts`, wire files, mock runtime.

Landing: rebase each stream onto `chapter/v0.3.6` in any order, resolve conflicts only in the listed shared files, stop on any other. Remove worktrees, push.
