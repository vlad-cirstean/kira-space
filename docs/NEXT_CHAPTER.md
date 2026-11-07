# Next chapter: drop v0.3.2 one-time migrations

Hand-off from v0.3.2 (inventory at `bee193a32`). The next chapter opens with this as its first phase.
The phase's last commit deletes this file.

v0.3.2 runs one-time boot moves from v0.3.1 data. The user is the only user. Once their v0.3.2 install
has booted and every move succeeded, this code only costs boot time and maintenance.

## Precondition: verify on the user's Mac

Do not start the phase until the user confirms each check. A failed check means that move did not
finish: the user relaunches v0.3.2 and reads its log first. Never delete a move that has not finished
on their machine.

Read-only commands. Never add `-w` or `-g` to `security`: those print the key.

1. Worktrees moved: `~/wt` is gone, or holds no Kira task worktree (`wtmove` leaves foreign and
   non-empty folders). `git worktree list` in each ADE repo shows no `~/wt/` path.
   `sqlite3 ~/.kira-space/kira.db "SELECT count(*) FROM ade_sessions WHERE cwd LIKE '%/wt/%'; SELECT count(*) FROM script_runs WHERE cwd LIKE '%/wt/%'"`
   prints `0` twice. `ls ~/.claude/projects | grep -- "-Users-$USER-wt-"` prints nothing.
2. Retired homes gone: `~/.kira-studio` and `~/.kira-memory` do not exist.
3. Agent secrets folded: `~/.kira-space/agent-secrets.db` does not exist;
   `~/.kira-space/backups/p557-*/agent-secrets.db` does. Agent credentials still show as set in
   Agents > Settings.
4. Keychain item renamed: `security find-generic-password -s "Kira Studio Secrets" -a "Kira Studio"`
   reports not found. `security find-generic-password -s "Kira Space Secrets" -a "Kira Space"` finds
   the item. Saved connection passwords still reveal.
5. Old MCP registrations gone: `claude mcp list` shows no `kira-db` and no `kira-ade`.
6. Leftovers cleaned: `~/.kira-space` root holds no `studio.db*`, `git.sock*`, `llm-prompts.json`,
   `connections/`, `api/`, `runs/`, `workflows/`, `steps/`, `repos/`, `agent/`.
   `~/.kira-space/backups/` holds no `p5NN-*` folder below `p557`.
7. Logs clean: the latest `~/.kira-space/logs/` file has none of `worktree move skipped`,
   `worktree move:`, `move agent secrets into kira.db`, `leftovers cleanup skipped`,
   `keychain: move the Kira Studio key`, `remove retired MCP registration`.

By hand, after the checks pass (user's call, no code):

- Delete `~/.kira-space/backups/p557-*` once 3 and 4 pass.
- NEEDS USER DECISION: `~/.kira-space/config/workflows/.p526-backup/`. P557 removed its reader
  (`MigrateInlineSteps`) but kept it out of the leftovers allowlist. It is dead. Default: user deletes
  it by hand; no code.

## Phase row (paste into the new SPEC table as its first row)

```
| P<first> | Drop v0.3.2 one-time migrations | Pending |
```

### P<first> Drop v0.3.2 one-time migrations

- Ask: v0.3.2 moved the user's v0.3.1 data once; the user ran it. Delete every one-time move,
  cleanup, its tests, flags, env overrides and doc text.
- Precondition: every check in `docs/NEXT_CHAPTER.md` "Precondition" confirmed by the user. Record
  the confirmation in the plan.
- Scope: the inventory below. Nothing else. `0091_p557_drop_legacy.sql` and the baseline guard stay.
- Tests: deletion only, so no new flow or UI test. Delete the move tests listed. Run go build, vet,
  golangci-lint, and `go test` for every touched package plus `flows/appflow`, `flows/harnessflow`,
  `flows/dbmcpflow`, `flows/adeflow`, `flows/memoryflow`, `internal/secrets`,
  `internal/harness/claude/...`, `internal/sqlitex`.
- Order: first phase of the chapter. One commit per item group below, in the order given.
- Single implementer, no streams: items share `appwire/onetime.go`, `main.go` and `flowharness/harness.go`.
- Plan: `plans/P<first>.md` (Opus, starts from this inventory, re-checks it against the current tree).
- Last commit: `git rm docs/NEXT_CHAPTER.md`.

## Inventory

Line numbers are at `bee193a32`; find by symbol.

### 1. Retired MCP registration removal (P560)

Delete:

- `apps/kira-space/internal/appwire/legacymcp.go` (`legacyMCPNames`, `NewLegacyMCPRemover`, `removeLegacyMCP`).
- `RunOneTimeMoves` param `legacyMCP` and its `go removeLegacyMCP(...)` (`appwire/onetime.go`).
- Call arg `appwire.NewLegacyMCPRemover()` (`apps/kira-space/main.go:88`) and `a.McpInstaller` (`flowharness/harness.go:443`).
- `harness.MCPRemover` interface (`internal/harness/mcp.go:101-104`).
- `mcpinstall.Installer.Remove` (`internal/harness/claude/mcpinstall/install.go:174-187`). Its only caller is `removeLegacyMCP`.
- Fake: `McpInstaller.Removes` field, `Remove`, `RemoveCalls` (`apps/kira-space/internal/flowharness/shellfakes.go:285, 308-320`).
- Test: `apps/kira-space/internal/flows/dbmcpflow/legacymcp_test.go`.

Keep: `isNotRegisteredError` (used by `register`), `Command`/`StdioCommand` remove-then-add text.

Gone check: `rg -n 'MCPRemover|LegacyMCP|legacyMCP|RemoveCalls|kira-db|kira-ade' -g '!docs/HISTORY.md'` is empty.

### 2. Leftovers cleanup (P557, P559)

Delete:

- `apps/kira-space/internal/leftovers/` (whole package: allowlist, `firstKeptBackup`, `StudioHomeDefault`, `MemoryHomeDefault`, `.kira-studio`/`.kira-memory` names, `KIRA_HOME` and `KIRA_MEMORY_HOME` reads).
- `leftovers.Run(...)` call and the `cipher.Status().Available` gate before it in `RunOneTimeMoves`.
- Test: `apps/kira-space/internal/flows/appflow/leftovers_test.go` (helpers `touch`, `exists` are local to it).
- `oldHome` (`~/.kira-memory`) assertions in `apps/kira-space/internal/flows/memoryflow/home_test.go` (`TestMemoryHomeLazy`). Keep the test; it still guards lazy creation of `<space>/memory`.

Docs:

- `docs/DEV_ENVIRONMENT.md:243` (`KIRA_HOME` sentence) and `:1091` (`KIRA_HOME` table row): delete.
- `README.md:278`, `apps/kira-space/README.md:207`, `site/src/content/docs/docs/space/memory.mdx:112`: delete the `~/.kira-memory` sentence.
- `apps/kira-space/README.md:21-22`: drop "First launch folds the retired Studio home in…" (stale since P557).
- `docs/ARCHITECTURE.md:393`: drop "A retired ~/.kira-studio home is deleted once at boot…".
- `docs/ARCHITECTURE.md:425`: delete the `internal/leftovers` bullet.
- `docs/ARCHITECTURE.md:1697`: drop the "`internal/leftovers` deletes the git.sock…" clause.
- `docs/ARCHITECTURE.md:2925`: drop "An old ~/.kira-memory is ignored (leftovers deletes it once)."

Gone check: `rg -n 'leftovers|KIRA_HOME\b|KIRA_MEMORY_HOME|\.kira-studio\b|\.kira-memory' -g '!docs/HISTORY.md' -g '!**/testdata/**' -g '!docs/design/**'`
is empty (`scripts/check-conventions.ts` "P267-ui-leftovers" is an unrelated plan name; memory-eval
`testdata` and `docs/design` mockups are frozen corpora, keep).

### 3. Agent secrets file fold (P557)

Delete:

- `internal/agentcfg/movesecrets.go` (`MoveSecretsFile`, `legacySecretsFile`, `checkpointFile`, `copySecretRows`, `parkSecretsFile`, the `backups/p557-` writer).
- `sqlitex.ReadOnlyURI` (`internal/sqlitex/sqlitex.go:75-77`): sole caller is `copySecretRows`. Keep `escapeDSNPath` (used by `BuildDSN`).
- Its call in `RunOneTimeMoves`. With items 1 to 3 gone `RunOneTimeMoves` is empty: delete it, its calls in `main.go:88` and `flowharness/harness.go:443`.
- Test: `apps/kira-space/internal/flows/harnessflow/secretsmove_test.go`.

Keep: `cipher := secrets.New()` in `main.go` (feeds `appwire.Build`); harness cipher availability check. Fix the `main.go:87` comment ("moves the v0.3.1 Keychain item") in item 4.

Docs:

- `docs/ARCHITECTURE.md:414`: drop "the one-time agent-secrets move (`agentcfg/movesecrets.go`)" from the raw-SQL list.
- `docs/ARCHITECTURE.md:419` heading "Databases, one-time moves and secrets": rename "Databases and secrets".
- `docs/ARCHITECTURE.md:421`: drop "`agent-secrets.db` folded into `kira.db` (P557)" clause; keep "sealed agent credentials are `secret_values` rows".
- `docs/ARCHITECTURE.md:423-424`: delete both bullets (`RunOneTimeMoves` order, `MoveSecretsFile` mechanics).
- `docs/ARCHITECTURE.md:1918`: drop "P557 moved the old `agent-secrets.db` in once…".

Gone check: `rg -n 'MoveSecretsFile|ReadOnlyURI|agent-secrets|RunOneTimeMoves|p557-' -g '!docs/HISTORY.md'` is empty.

### 4. Keychain item rename (P557)

Delete:

- `internal/secrets/keychain_move_darwin.go` (`legacyKeychainService = "Kira Studio Secrets"`, `legacyKeychainAccount`, `moveLegacyKey`, `moveKeyIn`, `readKey`, `addKey`, `deleteKey`; all move-only).
- `internal/secrets/keychain_move_darwin_test.go`.
- `moveLegacyKey` call in `loadOrCreateKey` (`internal/secrets/keyring_darwin.go:31-34`): body becomes `return loadOrCreateKeyIn(keychainService, keychainAccount)`; drop the now-unused `slog` import if nothing else uses it. Trim the doc comment's "A v0.3.1 item is moved…" sentences (`:28-29`).
- `apps/kira-space/main.go:87` comment.

Keep: `keychainService`, `keychainAccount`, `keychainLabel`, `keyBytes`, `loadOrCreateKeyIn`, `keychain_darwin_test.go`, nocgo and non-darwin stubs.

Docs: `docs/ARCHITECTURE.md:432` (name history) and `:433` (move mechanics): delete both; `:429` already states the current item.

Gone check: `rg -n 'moveLegacyKey|moveKeyIn|legacyKeychain|Kira Studio Secrets' -g '!docs/HISTORY.md'` is empty.
Darwin-only files compile only on a Mac: the user runs `go vet ./internal/secrets && go test ./internal/secrets -run Keychain` there.

### 5. Worktree move (P563)

Delete:

- `apps/kira-space/internal/wtmove/` (whole package).
- `RunWorktreeMove` (`appwire/onetime.go:37-47`). With item 3 done, delete `appwire/onetime.go` whole.
- `main.go:102-106` `os.UserHomeDir` block that calls it. Keep the settings read (`logging.SetLevel` and Build use it).
- `flowharness/harness.go:448-456`: `RunWorktreeMove` call, `userHome`; the `settings` read too if nothing else then uses it (compiler says). `gitclient` import stays (line 496).
- `internal/harness/claude/projectdir.go` whole: `MoveProjectDir` and `ProjectDirName` (only callers are `wtmove` and its test).
- Test: `apps/kira-space/internal/flows/adeflow/wtmove_test.go` (`TestWorktreesMoveOnce`; helpers `gitIn`, `dbExec`, `dbString`, `worktreeRecord`, `sortedRecords` are local to it).
- Optional: `ade.PathSegment` was exported only for `wtmove`; revert to unexported `pathSegment` (callers: `worktreedir.go`, `taskdir.go`, `sandbox.go`, `setup.go`).

Keep: `ade.WorktreesDir`, `ade.WorktreesBase` (board, sandbox), `porcelain.WorktreeListArgs`/`ParseWorktreeList` (git module), the regenerated contracts and fixtures (they show the current `kira-space-wts` path, not the move).

Docs: `docs/ARCHITECTURE.md:2494`: keep the base sentence and the base-path sentence; drop everything from "Boot runs a one-time move" to "Next chapter deletes `wtmove`."

Gone check: `rg -n 'wtmove|RunWorktreeMove|MoveProjectDir|ProjectDirName|onetime\.go|<home>/wt\b|~/wt\b' -g '!docs/HISTORY.md'` is empty.

### 6. Boot-order and stale doc text (last commit before deleting this file)

- `apps/kira-space/main.go:47` startup-order comment: drop "one-time moves ->".
- `docs/ARCHITECTURE.md:2446`: delete the `Reader.MigrateInlineSteps` paragraph (code deleted in P557).
- `docs/ARCHITECTURE.md:3070`: delete the `modelstore.RemoveRetired` line (code deleted in P559).
- `site/src/content/docs/docs/start/how-it-works.md:19`: "Backups of one-time upgrades" becomes "Config sync backups" (`configsync` still writes `backups/configsync-*`).
- `git rm docs/NEXT_CHAPTER.md`.

Final check: `rg -n -i 'one-time move|one-time boot|next chapter delete|MigrateInlineSteps|RemoveRetired' -g '!docs/HISTORY.md'` is empty.

## Keep, not one-time

- `SchemaTooOldError`, `SchemaTooOld`, the baseline rule in `sqlitex.Migrate` and the startupfail
  "predates v0.3.1" alert. A standing guard: every database older than the lowest migration has no
  forward path, whoever owns it. It reads no old data and moves nothing. Re-baselining at v0.3.2 is a
  separate decision, not this phase.
- `0091_p557_drop_legacy.sql` and `storage/migrate_p557_test.go`. Every fresh database still applies
  0091 after the 0090 baseline; the migration list is history (SPEC P557).
- `configsync.Recover` and `backups/configsync-*`: ongoing crash recovery, not a move.
- `docs/pending-changes/.github__workflows__release.yml.patch`: already applied (`b02f983dd`); nothing left.
