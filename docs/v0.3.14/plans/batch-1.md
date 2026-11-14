# Batch 1: P634, P635, P636

User asked to batch these three phases. Each runs as its own stream: own worktree, own Opus plan,
own Sonnet implementer. Base commit `fa00f7bf2` on `chapter/v0.3.14`.

| Stream | Phase | Plan | Worktree | Branch |
|---|---|---|---|---|
| A | P634 CI job sharding | `P634-ci-sharding.md` | `/home/user/kira-space` | `chapter/v0.3.14` |
| B | P635 agent config journey | `P635-agent-port-journey.md` | `/home/user/kira-space-stream-b` | `stream-b-v0.3.14` |
| C | P636 MySQL `DELIMITER` | `P636-*.md` (Stream C planner) | `/home/user/kira-space-stream-c` | `stream-c-v0.3.14` |

Stream A commits straight onto the chapter branch; B and C rebase onto it.

## File ownership

Each file has one owner. A stream touching a file it does not own stops and reports: ownership
boundary was wrong.

Stream A (P634):

- `scripts/go-test-split.sh`
- `.github/workflows/*`
- `docs/PACKAGING.md`
- `docs/DEV_ENVIRONMENT.md`
- `package.json` `test:go:*` scripts, if the split script's call changes

Stream B (P635):

- `internal/agentport/**`
- `apps/kira-space/internal/bridge/agentport.go`
- `apps/kira-space/internal/flows/harnessflow/port_test.go`, `port_journey_test.go`,
  `testdata/port/**`
- `apps/kira-space/tests/contract/agent-port-*.json`
- `apps/kira-space/frontend/src/ade/v2/port/**`
- `apps/kira-space/frontend/src/ade/v2/settings/AdeSettingsResult.vue`
- `apps/kira-space/frontend/src/bridge/index.ts`
- `apps/kira-space/tests/ui/ade-agent-port.spec.ts`
- `apps/kira-space/tests/ui/support/ipcChannels.ts`, `support/mockRuntime.ts`

Stream C (P636; its plan refines the list inside these areas):

- Console statement splitter: `packages/shared/domain/sqlSplit.ts`, `sqlLex.ts`, and their unit
  specs under `apps/kira-space/tests/unit/studio/sql-split*.spec.ts`
- Console frontend: `apps/kira-space/frontend/src/studio/views/console/**`
- Go MySQL adapter and data plane: `apps/kira-space/internal/adapters/mysqlfamily/**`,
  `apps/kira-space/internal/adapters/sqltext.go`
- Its flow test (dbflow console) and UI spec (console), plus their contracts and testdata

## Shared files

Additive hunks only, each stream in its own section. Rebase conflicts are resolved here and nowhere
else.

| File | Stream A | Stream B | Stream C |
|---|---|---|---|
| `docs/v0.3.14/SPEC.md` | P634 Status `Done`, `### P634` under Results | P635 Status, `### P635` Result | P636 Status, `### P636` Result |
| `docs/ARCHITECTURE.md` | CI section if the plan needs it | "Agent config import and export (P454)": `Backups`, `Restore`, tests line | console splitter / MySQL console section: `DELIMITER` |
| `packages/theme/src/i18n/en.json` | none | `space.ade.port.backupList.*`, `backend.core.agentPort.restore.*` | console keys, if any |

SPEC.md edits happen only in the landing step, by the stream's implementer, never mid-stream.

`ipcChannels.ts`, `mockRuntime.ts` and `bridge/index.ts` belong to B. If C needs a new channel, it
lands as an additive entry and moves into this table first.

## No ordering dependency

- A changes CI config and the split script only; no product or test code. B and C test commands
  are unchanged by it. `harnessflow` sits in A's `go` shards, `dbflow` and `mysqlfamily` in its
  container shards; both are matched by package path, so new tests in those packages land in an
  existing shard (unmatched packages go to the remainder shard per P634).
- B touches agent config (Go `agentport`, ADE settings UI). C touches the SQL console (TS splitter,
  MySQL adapter, console UI). No shared package, symbol, store or bound service.
- No stream reads another's output. Each verifies independently from base `fa00f7bf2`.

## Landing

1. Each stream finishes, passes its own verification, commits clean (no `--no-verify`).
2. Stream A is already on `chapter/v0.3.14`.
3. Rebase B, then C (or whichever is done first), onto current `chapter/v0.3.14`:
   `git -C <worktree> rebase chapter/v0.3.14`. Resolve conflicts only in shared files above;
   any other conflict stops the landing.
4. After each rebase: `go build ./...`, `bun run typecheck`, `bun run lint`, and the stream's own
   targeted tests. Then fast-forward the chapter branch:
   `git -C /home/user/kira-space merge --ff-only <stream branch>`.
5. Implementer fills its SPEC.md row and Result, commits on the chapter branch.
6. Remove stream worktrees (`git worktree remove`), delete the stream branches, push
   `chapter/v0.3.14`.
