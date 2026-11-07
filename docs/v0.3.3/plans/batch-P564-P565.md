# Batch: P564 + P565

Two parallel streams, each its own worktree off the same base commit. No ordering dependency.

| Stream | Phase | Plan | Branch | Worktree |
|---|---|---|---|---|
| A | P564 | `P564.md`, then `P564-rename.md` | `stream/P564` | `/home/user/kira-wt/P564` |
| B | P565 | `P565.md` | `stream/P565` | `/home/user/kira-wt/P565` |

Ownership: A owns everything outside `site/`. B owns `site/`, `apps/kira-space/tests/screenshots/support/shoot.ts`, `packages/workbench/src/testing/screenshots/capture.ts`.

Shared files, additive hunks that rebase cleanly:

| File | A edit | B edit |
|---|---|---|
| `docs/ARCHITECTURE.md` | delete one-time-move text, repo URL | site section (P280) bullets |
| `docs/DEV_ENVIRONMENT.md` | `KIRA_HOME` lines, repo URL | site/screenshots section |
| `README.md` | memory sentence, repo URLs incl. github.io | none |

Cross-dependency: `site/tests/site.spec.ts` asserts the README github.io link. A updates README. B's implementer rebases onto A before running that spec; never edits README.

Landing: both streams implemented and verified, rebase each onto `chapter/v0.3.3`, resolve only listed shared files, stop on any other, remove worktrees, push.
