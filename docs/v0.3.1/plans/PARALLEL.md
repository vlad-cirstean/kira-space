# v0.3.1 parallel batch: P551, P552, P554

Base: head after the three plan cherry-picks (`c9b8062be`). 3 streams, each own worktree off base.

- Stream A: P551 (site, Build with Kira, install notice). Owned files: `plans/P551.md` section 13.
- Stream B: P552 (SQLite schema). Owned files: `plans/P552.md`.
- Stream C: P554 (GitOps). Owned files: `plans/P554.md`.

No ordering dependency between streams. P553 (query builder) starts only after P552 lands.

Shared files:

| File | Streams | Edit |
|---|---|---|
| `connfiles/*`, `apifiles/*` | B, C | additive hunks. B: SQL-literal and scan changes. C: config-sync logic |
| `repocfg/migrate.go` | B, C | same split |
| `adeflow/reader.go` | B, C | same split |
| `configfs/table.go` | B, C | same split |
| `scripts/check-conventions.ts` | A, B | A: N15 message. B: STRICT rule. Additive |
| `apps/kira-space/playwright.config.ts` | A | `screenshots` project only |
| `packages/theme` i18n `en.json` | A, B, C | additive keys |
| `docs/ARCHITECTURE.md` | A, B, C | additive hunks |
| `docs/DEV_ENVIRONMENT.md` | A, B, C | additive hunks |
| `docs/v0.3.1/SPEC.md` | A, B, C | own Result row only |
| `docs/pending-changes/.github__workflows__pages.yml.patch` | A | file exists (ubuntu-24.04 pin, root cause of placeholder-only Pages shots). A extends it with cache key hunks; never overwrite |

Landing: each stream independently verified, then rebase each onto `chapter/v0.3.1` in any order. Resolve conflicts only in files listed above; stop on any other. Remove worktrees, push.
