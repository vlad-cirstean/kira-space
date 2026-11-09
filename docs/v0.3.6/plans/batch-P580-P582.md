# Batch P580, P581, P582

User asked for 3 phases in parallel. Streams, each a git worktree off `259bd77ec` (P580 runs on the chapter checkout itself).

| Stream | Phase | Worktree | Branch |
|---|---|---|---|
| A | P580 API MCP rules | `/home/user/kira-space` | `chapter/v0.3.6` |
| B | P581 Review tool diff and fetch | `/home/user/kira-wt-p581` | `stream/p581` |
| C | P582 Handovers select and copy | `/home/user/kira-wt-p582` | `stream/p582` |

No ordering dependency: A is the Database MCP and Settings, B is the git review tool, C is handover text rendering. Each plan lists its own files; this table lists overlap candidates.

| Shared file | A | B | C |
|---|---|---|---|
| `docs/ARCHITECTURE.md` | own subsection (Database MCP) | own subsection (review) | own bullet (task panel) |
| copy catalog (`scripts/copy/*`, i18n catalogs) | new keys | new keys | new keys if any |
| `docs/v0.3.6/SPEC.md` | status and Result of own row | status and Result of own row | status and Result of own row |

All overlap is additive hunks. Landing: after every stream is verified, rebase B and C onto `chapter/v0.3.6` in order, resolve conflicts only in the listed shared files, stop on any other. Remove worktrees, push.
