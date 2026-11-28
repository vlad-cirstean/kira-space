# v0.3.27

Chapter v0.3.27, branch `chapter/v0.3.27`, tag `v0.3.27`. First phase `P762` (highest phase: `P761`,
in `docs/history/07-v0.3.15.md`, chapter v0.3.26).

Code review at chapter end: 1 round.

Batch (owner request): P762, P763, P764 run as three concurrent streams, see `plans/batch.md`.
P762 ships fully. P763 and P764 are redesigns: each stops at a plan plus a mockup for the owner to
review and iterate on; implementation follows the owner's sign-off.

| # | Phase | Status |
|---|-------|--------|
| P762 | Branches MCP cannot target a worktree when a repo has several branches | Pending |
| P763 | Redesign links to show the host too (design for review) | Pending |
| P764 | Redesign task colours with a faint background, shared by task details and review window (design for review) | Pending |
| P765 | Chapter close | Pending |

## P762

The branches MCP tool fails when one repo has more than one branch: the caller cannot name the
worktree. Add worktree selection to the tool, resolve it in the backend, cover with a flow test and a
UI spec split at the IPC boundary.

## P763

Links list shows only a title or URL. Show the host too. Design first: mockup and plan, no
implementation until the owner approves.

## P764

Task colours become a faint background tint. Task details and the review window use the same tint and
design. Design first: mockup and plan, no implementation until the owner approves.

## P765

Chapter close, by hand per `CLAUDE.md`, after the owner signs off P763 and P764 and they ship.
