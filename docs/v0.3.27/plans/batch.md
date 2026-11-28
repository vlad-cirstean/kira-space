# Batch: P762, P763, P764

Owner asked for all three at once. No ordering dependency: P762 touches the branches MCP tool and its
backend resolver; P763 the links surface; P764 task colour tokens, task details and review window.
Each stream gets its own worktree off the same base, its own Opus plan, its own Sonnet pass.

| Stream | Phase | Worktree | Output |
|--------|-------|----------|--------|
| A | P762 | `kira-space-P762` | plan, implementation, flow test, UI spec, commits |
| B | P763 | `kira-space-P763` | plan, mockup (HTML), no product code |
| C | P764 | `kira-space-P764` | plan, mockup (HTML), no product code |

Shared files: none expected. B and C write only under `docs/v0.3.27/plans/`; the plans and mockups
land on the chapter branch after review. Stream A rebases onto the chapter branch.
