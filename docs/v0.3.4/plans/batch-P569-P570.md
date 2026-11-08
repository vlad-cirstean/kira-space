# Batch P569 + P570

Two streams, each its own git worktree off the same base commit (chapter/v0.3.4 head). No ordering dependency: P569 never touches `internal/agentmcp/` or `apps/kira-space/internal/ade/`; P570 touches neither proof nor menu code. Plans: `P569.md` (Stream A), `P570.md` (Stream B). Each plan's file-ownership table is authoritative.

## Shared files (additive hunks, resolve on landing)

| File | Stream A (P569) | Stream B (P570) |
|---|---|---|
| `docs/ARCHITECTURE.md` | "Validation proof (P420)" section, native-menu clause | `request_branches` bullet, "Branch requests (P408)", one bullet in "Task base and rebase runs (P241)" |
| `internal/llmprompts/registry.go` | `DbmcpToolProofSave` block | one `branches.base` arg line in `HeadlessToolRequestBranches` |
| `internal/llmprompts/testdata/budget.tsv` | `dbmcp.tool.proof_save*` lines | one line after `headless.tool.request_branches.arg.branches.branch` |
| `site/src/content/docs/docs/reference/mcp-tools.md` | `proof_save` line | `request_branches` line |
| `packages/theme/src/i18n/en.json`, `apps/kira-space/main.go`, migrations `embed.go` and `schema.sql`, generated bindings | Stream A only (B declared none) | none |

Generated bindings: regenerate after rebase, never hand-merge. If B adds a migration, whichever stream lands second renumbers.

## Landing

Both implemented and independently verified, then rebase each stream branch onto `chapter/v0.3.4` in any order, resolve conflicts only in the files above, stop on any other. Remove worktrees. Chapter stays open (no close phase).
