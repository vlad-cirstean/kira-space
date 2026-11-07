# v0.3.2 review round 1

Base `e7356a352` (v0.3.1 squash merge). Head `d491fbe6a`. 78 commits, P555-P563. One Opus reviewer, three dimensions: architecture/security, correctness, performance. Report only; fixer owns changes.

Counts: 1 Medium, 6 Low.

## A. Repos page leave guard

### R1-1 Medium: `open()` on Repos page skips leave guard

- File: `apps/kira-space/frontend/src/ade/v2/state/adeReposUi.ts:45` (`open`).
- Defect: `switchAdeView('repos')` returns true when view is already `repos`. `open()` then sets `selectedRepoId` and tab directly, never calling `leave()`.
- Failure: user edits repo settings file on Repos page, unsaved. Clicks "Configure" on another repo (`RepoList.vue:210`, `reposUi.open({repoId})`) or GitStart entry. `RepoFilePane` remounts; edits lost without prompt; `dirty` may stay stale.
- Fix: when view is already `repos`, route repo/tab change through `leave()` (reuse `pickRepo`/`setTab` path). Add `ui` spec: dirty file, `open()` other repo, guard prompts.

## B. Worktree move (P563) misses or never retries some state

Root cause: `wtmove` keys old paths on current DB state and gates all work on one `needed()` check, so anything not matched first boot is never revisited.

### R1-2 Low: old path derived from current `code_repos.name`

- File: `apps/kira-space/internal/wtmove/wtmove.go:159` (`oldPath := filepath.Join(old, seg, r.dir)`).
- Defect: segment comes from repo's current name, not name at worktree creation.
- Failure: repo renamed or re-imported since worktree was made. Git record path differs; worktree stays under `~/wt` silently. Sessions keep old paths, `needed()` (`:96`) stays true, git runs every boot.
- Fix: match git worktree records under `<home>/wt/*/<worktree_dir>` by basename, not rebuilt path. Log unmatched rows once.

### R1-3 Low: rebase run `spec_json` keeps old worktree paths

- Files: `apps/kira-space/internal/wtmove/wtmove.go:265` (`rewrite`); readers `apps/kira-space/internal/ade/rebase.go:709` (`recordAbort`), `:739` (`reverifyRecovered`), `rebaseContext`.
- Defect: `rewrite` updates sessions, terminal tabs, local-file tabs, script runs. Not `ade_runs.spec_json`, whose `Stack[].Worktree` holds absolute paths.
- Failure: rebase run interrupted or left with operation in progress across upgrade boot. `reverifyRecovered` and later `recordAbort` call `verifyRebase` on `~/wt` paths; facts show "could not read" instead of real git state. Rebase prompt context may cite dead paths. Rebase runs are not resumable (`runResumable`), so no agent runs in old path.
- Fix: include `ade_runs.spec_json` in `rewrite` (JSON path replace over `$.Stack[*].Worktree`), same transaction.

### R1-4 Low: transcript folder move never retried

- File: `internal/harness/claude/projectdir.go:25` (`MoveProjectDir`); caller in `wtmove.go` `moveOne`.
- Defect: rename error or "target exists" only logs. After `~/wt` cleanup and path rewrite, `needed()` is false; move never runs again.
- Failure: Claude transcript folder stays under old encoded cwd; resume of that conversation from new cwd finds no history.
- Fix: on failure, keep old session paths (skip `rewrite` for that worktree) so next boot retries; or merge into existing target folder.

## C. Per-boot cost of one-time moves

Root cause: one-time work does its expensive setup before checking whether anything is left to do.

### R1-5 Low (performance): git discovery on every boot

- File: `apps/kira-space/internal/appwire/onetime.go:40` (`RunWorktreeMove`).
- Defect: builds `gitclient` Discovery and runs `Status` before `wtmove.Run` checks `needed()`. Build discovers again later.
- Failure: every boot after move pays extra git lookup (process spawn) for no work.
- Fix: export cheap `wtmove.Needed(db, home)` and return early; or resolve git lazily inside `Run`.

### R1-6 Low (performance): legacy MCP removal spawns `claude` twice every boot

- File: `apps/kira-space/internal/appwire/legacymcp.go:26` (`removeLegacyMCP`).
- Defect: runs `claude mcp remove --scope user` for `kira-db` and `kira-ade` every boot until next chapter deletes it.
- Failure: two `claude` process spawns per boot after first success; log noise when absent.
- Fix: record done marker in settings after success, or skip when `claude mcp get` shows server absent.

## D. Inline script output size

### R1-7 Low: `capLogBytes` keeps one line of any size

- File: `apps/kira-space/internal/dbmcp/script_tools.go:376` (`capLogBytes`).
- Defect: always keeps at least one line, ignoring 64 KiB cap. Line can reach `linewriter.MaxLine` (16 MiB). Predates chapter, but P560 made `run_script` inline output by default.
- Failure: script prints one huge line (minified JSON, base64). MCP result carries up to 16 MiB; blows agent context.
- Fix: truncate single kept line's text to `scriptLogsMaxBytes`, mark truncated.

## Reviewed clean

- P557 baselines: kira 90, review 8, memory 8 match chain byte-for-byte per plan proof; `SchemaTooOldError` logic; `0091` drop; `migrate_p557_test.go`. Memory `0009` dropped by user decision (P559).
- `leftovers`: allowlist names match current `config/` paths (`ade/runs` is not top-level `runs`); `safe()`, `within()`, `inUse()` app.lock guard. `retiredHome` compares resolved vs unresolved path; `inUse` covers it.
- `MoveSecretsFile`: copy with `ON CONFLICT DO NOTHING` plus verify, park after; no secret material logged.
- Keychain `moveKeyIn`: constant-time compare, errors name service only, failed move leaves key unavailable rather than regenerated.
- Startup order (`main.go`, `flowharness`): moves before cipher gate and board build.
- P556 experimental gate: settings pane, distill action gated by context key; no bypass found.
- P559 lazy ingest.
- P560: `proof_save` compensating `DiscardDraft`; `report_artifacts` proof kind; operations/cancel merge unchanged semantics; legacy MCP remove user scope only; `--disallowedTools` exact `mcp__kira-studio` match.
- P561 reveal: loop terminates (`useCappedList` grows synchronously); no watcher loop (`followFile` returns early on same path).
- P562 i18n trims deliberate per plan; security caveats kept.
- P555 site image.
- Standing rules: no `<style>`, `defineComponent` or plain `<script>` in changed Vue files. New tests are flow tests plus one migration test; fit test bar.
- `full-run.md`: all green.
