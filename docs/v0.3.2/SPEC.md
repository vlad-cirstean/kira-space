# v0.3.2

Branch `chapter/v0.3.2`, based on `main` (v0.3.1 merged). First `P` is 555 (highest in v0.3.1 is P554).

Source: user request. The table is open: the user adds more phases to this chapter. Last `P` is undecided until the user closes the table. Close duty after the last phase lands: one full suite run, 1 code review round, hand-written close. After the close and push, open the PR to main (CLAUDE.md close rules), subscribe to it, and fix failing checks until it merges. Each phase: Opus plan, Sonnet implementation.

| Phase | Title | Status |
|---|---|---|
| P555 | Keychain install image and Pages build | Pending |
| P556 | Experimental settings menu, Memory behind a toggle | Pending |
| P557 | Drop backward compatibility, single baseline migration | Pending |
| P558 | Repositories page in ADE, replacing the Git module dialog | Pending |
| P559 | Memory home under the Kira Space home | Pending |
| P560 | Agent MCP tools: use-case pass, Kira Space and Kira Studio rename | Pending |
| P561 | Review window file list follows the shown file | Pending |
| P562 | Trim connection dialog explanations | Pending |
| P563 | Worktrees move to ~/kira-space-wts | Pending |

More rows follow as the user adds them. Table order is execution order.

## User to-dos (note, not phases)

- Re-run the Release workflow for v0.3.0.
- Verify the folder prompt text (site Install block, `docs/start/install.md`) on a real Mac.
- After P560 lands: run `claude mcp remove --scope user kira-db` once, then click Install in Settings, Database MCP (or per P560 user decision 5).

## Rows

### P555 Keychain install image and Pages build

- Ask: add the user-supplied Keychain prompt image as `site/src/assets/install/keychain-access.webp`. Source: ready WebP, cropped to the dialog, transparent rounded corners, 868x372. No folder prompt image exists or will exist: the folder permission prompt is prose only. Remove the folder image requirement everywhere (content, Install component, placeholder fallback, site spec, DEV_ENVIRONMENT). Keep the build guard for the required image (now only `keychain-access.webp`); never weaken it.
- Baselines: re-record the site visual baselines so they show the real Keychain image and no folder placeholder. Method: site Playwright WebKit on Linux, `docs/DEV_ENVIRONMENT.md` (site visual baselines).
- Why Pages fails: last run 42348265274 failed solely with "Missing install image site/src/assets/install/keychain-access.webp" at `/`. `site/src/lib/installShots.ts` fails under `CI=true` on a missing image, by design.
- Done: `CI=true bun run build` in `site` passes; affected site specs pass; Pages workflow run for `main` green after merge (confirmable only after merge).
- Plan: `plans/P555.md`.
- Result: pending.

### P556 Experimental settings menu, Memory behind a toggle

- Ask (verbatim): "Add an experimental menu in settings and hide memory module under a toggle."
- Scope: new Experimental pane in Kira Space Settings, built as a list of flag rows. First row: `experimental.memory`, default off. Off hides every Memory UI surface: the Memory mode tab, the Memory settings pane, the editor "Distill selection" action. Memory data, memory.db, the `memory-mcp` server and its agent registration stay untouched.
- Existing installs: off too, including ones with a `memory.db`. No boot-time write.
- Tests: backend flow test plus UI spec, same scenario, one contract. Existing Memory specs enable the flag first. Settings and title-bar visual baselines re-recorded on Linux.
- Plan: `plans/P556.md`.
- Result: pending.

### P557 Drop backward compatibility, single baseline migration

- Ask (verbatim): "I'm the only user of this app, I updated to this latest published version 0.3.1, any migration should be dropped. I don't only mean reconciling all sqlite migrations into 1, but dropping any scripts, paths etc dedicated for backwards compat."
- Scope: every install is exactly v0.3.1 (tag `v0.3.1`, merge `e7356a352`). Squash each SQLite chain into one baseline at the version v0.3.1 records: kira.db 90, review.db 8, memory.db 8. An older non-empty DB refuses to open with a pointed error. Delete the home, config and DB folds, the old-Studio checks, one-time DB-to-file exports, legacy cleanups, the frontend key migration, `install.sh --app=studio` and its release job. Keep the migration runner, `schema_version`, `SchemaTooNewError` and the foreign-keys-off step type for future migrations. Keep the v0.3.1 updater contract (`--app=space`, `--wait-pid`, `--notify-fd`).
- Added by the user: fold `agent-secrets.db` into kira.db `secret_values` (scope `agent-credential`, owner `agent`, same cipher, only through `secrets.Store`, values never in logs or the op log); rename the macOS Keychain item from "Kira Studio" to "Kira Space" keeping the existing master key. Both are one-time boot data moves of live v0.3.1 data, idempotent, source kept until verified. Out of scope: `~/.kira-memory` move and the add-repos dialog move (separate phases).
- Zero data loss on the user's v90 kira.db: baseline proven byte-for-byte against a chain-built DB, and a chain-upgraded DB with data opens unchanged.
- User decisions (plan "Decisions"): delete all four old config keys (each verified pure back-compat); kira `0091` drops old JSON state (numeric review comment ids in `ade_runs.spec_json`, http tabs with `bodyMode: 'json'`), `schema_quarantine` and stale marker rows, then both readers go; delete `install.sh --app=studio` and its release job (pending-changes patch); drop `memory_legacy_ids` and the legacy-uuid lookup (memory baseline without it, memory `0009` drops it from the user's DB).
- User decision (irreversible, authorized): a one-time boot cleanup, isolated in `apps/kira-space/internal/leftovers`, deletes every file and folder the post-P557 code never reads: dead names inside `~/.kira-space` by an exact allowlist, the pre-P557 `backups/p5xx-*` folders (not `backups/p557-*`) and the retired `~/.kira-studio` (P559 adds `~/.kira-memory`, user-authorized: they do not use Memory). It runs only after the baseline check, the agent-secrets move and the Keychain move succeed in the same boot. Allowlist in the plan.
- Order: runs after P556 lands (P556 already removed its Memory upgrade seed, `4ef78a6b1`). Single implementer, no streams.
- Plan: `plans/P557.md`.
- Result: implemented (row stays Pending until the Mac keychain test and the release.yml patch land).
  - One baseline per DB at v0.3.1: kira 0090 plus `0091_p557_drop_legacy.sql`, review 0008, memory 0008 (P559 deleted `0009_p557_drop_legacy_ids.sql`). Proof at base `72907efb1`: chain-built and baseline-built DBs match exactly for all three (kira 62 tables, 116 indexes, 8 seed rows, v90; review 9, 10, 0, v8; memory 15, 23, 9 triggers, 1 seed row, v8). Real-data copies open unchanged (memory 8 to 9 via `0009`, since deleted). Older DB refuses with `SchemaTooOldError`, newer with `SchemaTooNewError`.
  - Deleted: home, config and DB folds, exports, `MigrateOnce`, old-Studio alert, `--app=studio`, four old config keys, adeflow legacy status names, legacy frontend storage keys, install.sh Studio path.
  - Moved once: `agent-secrets.db` into kira.db `secret_values` (original parked in `backups/p557-<UTC ts>/`); Keychain item "Kira Studio" to "Kira Space" with the same key; boot cleanup of dead leftovers.
  - Pending patch: `docs/pending-changes/.github__workflows__release.yml.patch` (`.github/workflows/` cannot be pushed from a session).
  - User precondition before first v0.3.2 launch: back up `~/.kira-space/kira.db`, `review.db`, and `agent-secrets.db`. No memory backup: `~/.kira-memory` is deleted at first boot.
  - Unverified here: darwin keychain tests (`go test ./internal/secrets -run Keychain`) need a real Mac; the darwin cgo files compile only there.
  - Left out of the leftovers allowlist: `config/workflows/.p526-backup` (not an authorized location). Needs a user decision.
  - Next chapter: delete (each one commit):
    - `agentcfg.MoveSecretsFile` (`internal/agentcfg/movesecrets.go`) and `flows/harnessflow/secretsmove_test.go`.
    - `moveLegacyKey` and `moveKeyIn` (`internal/secrets/keychain_move_darwin.go`), `keychain_move_darwin_test.go`, and the `moveLegacyKey` call in `loadOrCreateKey`.
    - `apps/kira-space/internal/leftovers/` (Studio home and, since P559, `~/.kira-memory`) and `flows/appflow/leftovers_test.go`.
    - `appwire/onetime.go` (`RunOneTimeMoves`) and its calls in `main.go` and `flowharness/harness.go`.
    - P563: `apps/kira-space/internal/wtmove/`, `claude.MoveProjectDir` (`internal/harness/claude/projectdir.go`) and `flows/appflow/wtmove_test.go`.
    - `sqlitex.ReadOnlyURI` when nothing else uses it.
    - `backups/p557-*` folders (user deletes by hand).
    - `docs/pending-changes/.github__workflows__release.yml.patch` (whoever applies it).
    - `0091_p557_drop_legacy.sql` stays as history.

### P558 Repositories page in ADE, replacing the Git module dialog

- Ask (verbatim): "Adding git repos dialog should be moved from the git module to the ade module as a normal panel like workflow is".
- Scope: `repo/ReposDialog.vue` becomes `ade/v2/repos/AdeReposPage.vue`, a Repositories tab in the ADE top bar beside Agent config and Workflows. It gets its own in-memory store (`adeReposUi`) and the same unsaved-edits leave guard. The Git module stops mounting the dialog; its panel header button goes. GitStart and the repo row "Configure repository…" jump to the ADE page (default; user decision 1). ADE with no repos opens the page from its empty state. No binding or bound-service change.
- Tests: no new backend behaviour, so no new flow test. UI specs open the page from the ADE tab; testids `repos-dialog-*` become `ade-repos-*`. Visual: `git-repos-dialog` baseline deleted, new `ade-repos-page` baseline recorded on Linux.
- Order: runs after P557. Shares only `en.json` and ARCHITECTURE with P556/P557, different keys and sections.
- Single implementer, no streams. Plan lists 2 NEEDS USER DECISION items, each with a default.
- Plan: `plans/P558.md`.
- Result: pending.

### P559 Memory home under the Kira Space home

- Ask (verbatim): "Having a separate db is fine. But It should be in the kira space folder so add this as a phase too".
- Scope: memory.db stays its own SQLite file but moves to `<Space home>/memory/memory.db` (default `~/.kira-space/memory/`), with `models/`, `distilled/` and the import runner log and lock beside it; never under `config/`. One resolver, `kirapaths.MemoryHome`. `KIRA_MEMORY_HOME` dropped (default; user decision 1); memory follows `KIRA_SPACE_HOME`. No migration or move code: nothing reads an old `~/.kira-memory`; the one-time leftovers cleanup deletes it (user-authorized, same guards as `~/.kira-studio`). Agent sandbox allows the memory subtree only, never the Space home. Lazy creation: `memory-mcp` start no longer opens memory.db (the ingest engine opens on first ingest call), so a Claude session start with Memory off creates nothing; the boot-time retired-model cleanup goes. Memory migration `0009` deleted (default; user decision 2). No backend gate on the Memory flag (default; user decision 3).
- Tests: one backend flow test (boot and `memory-mcp` start create no memory home; first tool call creates `<space>/memory/memory.db`; `~/.kira-memory` never appears). No UI spec: no bound method or UI change. Sandbox goldens regenerated. Real `claude` memory rows run.
- Order: runs after P557 (both touch Go memory code, migrations, `appwire`, `config/paths.go` and docs). Shares only ARCHITECTURE with P558, different sections.
- Single implementer, no streams. Plan lists 3 NEEDS USER DECISION items, each with a default.
- Plan: `plans/P559.md`.
- Result: pending.

### P560 Agent MCP tools: use-case pass, Kira Space and Kira Studio rename

- Ask (verbatim): "add a phase in this chapter to analise both and tighten them up drop redundancies. Most importantly is to think how actual usecases look like and try to make the mcp tools follow that. I want these to be clear streamlined efficient to use. Also rename kira db to kira studio and kira ade to kira space. This can be started now in parallel".
- Scope: the agent board server and the database server; `kira-memory` untouched. Rename `kira-ade` to `kira-space`; `kira-db` to `kira-studio` (session server `kira-studio-session`); labels "Kira DB" and "Kira ADE" follow. Tools designed from use cases: `kira-space` 16 to 13 (branch_status into task_info, attach_proof into report_artifacts, list/cancel operations into `operations`); `kira-studio` 26 to 18 (`describe`, explain into run_query, one-call `proof_save` with task auto-attach, script logs into get_script_run, `kube_discover`, `operations`). finish_step/handover, git/sandbox push, browse_table/run_query stay apart (guardrails in plan). Headless `db_mcp` steps see only the audited session server. Texts shortened under a lowered `budget.tsv`. No back-compat code: the old user-scope `kira-db` entry is a user to-do. No binding or bound-service change.
- Tests: flow tests for every merged tool (adeflow, dbmcpflow, proofflow); UI specs only for the renamed audit toggle and agent settings text; goldens and contracts regenerated; real `claude` ADE, smart script, DB MCP, prompt registry, hermetic and agent config rows once per chapter.
- Order: requested in parallel. Own stream beside P558 and P559 (batch rule, own worktree); P557 implementation already landed, so no Go overlap. Shared files: `en.json`, ARCHITECTURE, DEV_ENVIRONMENT, README, each a different section.
- Single implementer, no streams inside the phase. Plan lists 5 NEEDS USER DECISION items, each with a default.
- Plan: `plans/P560.md`.
- Result: pending.

### P561 Review window file list follows the shown file

- Ask (verbatim): "In the Review window. Go to next file also moves the left panel with the files."
- Reading: request, not bug. When the centre diff moves to another file, the left Files list selects that file and scrolls it into view, expanding collapsed folders and the render cap as needed. Today cross-file next/previous change and "Next file to review" move the highlight only, never scroll, and miss a file inside a collapsed folder. Tab clicks and working-file comment jumps do not move the list at all.
- Scope: one source of truth, the active centre tab. `AdeReviewFiles.vue` watches it and calls new `ReviewApi.revealFile(section, path)`, which moves git-ui's list cursor without opening anything. FileTree reveals its selected row whenever the selected path changes (expand parents, raise cap, `revealElement` nearest, no focus move). The committed highlight clears while the cursor sits in the Uncommitted section. Frontend only: `packages/git-ui` and the review window host. Git module sidebar gets the reveal but no tab watcher (default; user decision 1). A filter-hidden file stays hidden (default; user decision 2).
- Tests: no backend change, so no flow test. UI specs (WebKit Playwright): cross-file next/previous reveals and expands, tab click follows, one highlight across sections, working comment jump highlights; selection and in-view assertions. No visual baseline expected to change; three review and git visual specs rerun on Linux with Inter to confirm.
- Order: after P560 in table order. Concurrent with P559 and P560: no Go, no `en.json`; shares only one ARCHITECTURE sentence (review navigation paragraph) and SPEC.
- Single implementer, no streams. Plan lists 2 NEEDS USER DECISION items, each with a default.
- Plan: `plans/P561.md`.
- Result: pending.

### P562 Trim connection dialog explanations

- Ask (verbatim): "trym the explainations in the connection edit dialog. Keep what they do and what they mean, but only the essential not it s so extremely verbose and worse repeated for each button etc. Do it now".
- Scope: copy only, connection edit dialog (`ConnectionFileDialog.vue` and its tabs) and the `en.json` values behind them. 90 explanatory strings (about 1,870 words) inventoried with a verdict and new text each. Trim to the essential meaning; one home per repeated idea (exemption rules, throttle detail, masking enforcement, row threshold, unclassifiable statements). MCP tab stops rendering three texts shown elsewhere; one key (`mcp.tablesEnforceFail`) folds and is deleted. Labels, behaviour and test ids of rendered controls unchanged. Security caveats stay, compressed. NewConnectionDialog untouched. Edge-case detail stays in site `studio/database-mcp`.
- Tests: no backend change, no flow test. Touched UI specs updated to the new phrases (connection-mcp-access, -expensive, -throttle, -timeout, -masked) plus verify-only specs and copy-wording, copy-coverage, consistency. `check-catalog --unused` clean. No visual baseline shows the edit dialog; new-connection visual rerun on Linux with Inter to confirm no diff.
- Order: after P561 in table order. Concurrent with P560: shares only `en.json` (different keys).
- Single implementer, no streams. No NEEDS USER DECISION items.
- Plan: `plans/P562.md`.
- Result: 55 values trimmed, `mcp.tablesEnforceFail` deleted (90 strings become 89). Changed values 1,511 words become 918. MCP tab stops rendering three texts (`connection-mcp-rows-threshold`, throttle description, `connection-mcp-enforce-masking`); masking text lives on Privacy only. Six values reworded for catalog C6 (no lowercase sentence start after `;` or `:`). Specs updated: connection-mcp-access, -expensive, -throttle, -timeout, -masked, plus `connection-mcp-tables` (`refused` became `refuses any`). Pre-existing failure fixed: `dbmcp-review-mode` "changing one mode saves the per-mode key" expected unchanged keys in the edit payload; failed on base too, now asserts only `mcp.review.read`. Ran: check-catalog --unused, check-copy, typecheck, lint (commit hooks), 11 targeted UI specs plus copy-wording, copy-coverage, consistency, all green. Visual: `connection-dialog` new-connection spec passes on Linux with Inter, no diff, no re-record. No doc edit needed.

### P563 Worktrees move to ~/kira-space-wts

- Ask (user decision): ADE task worktrees move from `<userHome>/wt/<repo>/<wt_id>` to a visible folder `<userHome>/kira-space-wts/<repo>/<wt_id>`. Not under `~/.kira-space`; no `KIRA_SPACE_HOME` link. Base derives from the user home: one constant (`ade.WorktreesDir`); tests override through `TaskBoardDeps.HomeDir`.
- Scope: `worktreeDirFor`, `requestedWorktreeDirs` (sandbox) and the board's `worktreeBasePath` use the new base. Per-repo `worktree.base_path` keeps today's meaning: it pre-fills the Git module's New worktree dialog only and never applied to ADE worktrees; its description now says so. No global base setting (not requested).
- One-time move (`internal/wtmove`, called from `RunOneTimeMoves` before Build, so before any run, terminal or agent): one `git worktree move` per worktree, never a file copy, so uncommitted work and git's two-way paths stay valid. Only folders of `ade_task_branches.worktree_dir` rows under the old base. Locked: unlock, move, relock with the same reason (default; user decision 3). Target exists, folder missing, or folder unknown to git: report, leave, retry next boot. Foreign content in `~/wt` untouched; `~/wt/<repo>` and `~/wt` removed only when empty. No marker: no row folder under the old base and no stored old path means done.
- Stored absolute paths rewritten after each move: `ade_sessions.cwd` (TUI resume stats it), terminal tab `state_json.cwd`, `local-file` tab `path`, `script_runs.cwd`. History stays: op log, sandbox policy rows, prompts, logs. review.db keys on repo id and branch: unaffected. Claude transcript folder `~/.claude/projects/<encoded cwd>` renamed so `--resume` finds the conversation (default; user decision 1); OpenCode resume falls to the existing lost-conversation path.
- Sandbox: worktrees stay explicit write subpaths under the user home, outside the Space home, as with `~/wt`; no profile or golden change.
- Tests: one backend flow test with a real repo (dirty, locked, foreign folder, target exists, missing folder, DB and transcript rewrite, idempotent second boot). No UI spec: no bound method or UI behaviour change; contracts carrying `<home>/wt` regenerate. UI mock fixtures and the screenshot story move to `~/kira-space-wts` (default; user decision 2); baselines re-recorded only where pixels change.
- Order: after P562 in table order. Shares with P560: `en.json` (different key), ARCHITECTURE (different section), `tests/contract/` (different files; regenerate after rebase). Single implementer, no streams. Plan lists 3 NEEDS USER DECISION items, each with a default.
- Plan: `plans/P563.md`.
- Result: pending.

## Close

1 code review round unless the user says otherwise. After the last phase: one full suite run with all failures fixed, review round, hand-written close per CLAUDE.md, PR. The table is open: add checklist items for each new phase.

### Site and images check (mandatory)

Runs after the last phase and the full suite run, before the code review. Close does not start until every item passes.

- [ ] `site/src/assets/install/keychain-access.webp` present, cropped to the dialog, with alt text; orchestrator views it. Folder prompt is prose only, no image or placeholder.
- [ ] `CI=true bun run build` in `site` passes with no placeholder.
- [ ] Site visual baselines re-recorded on Linux WebKit, showing the real Keychain image, no placeholder box.
- [ ] Pages workflow run for `main` green after merge.
- [ ] Docs match final behaviour: README, ARCHITECTURE, PACKAGING, DEV_ENVIRONMENT, site docs.

### P556 Experimental gate check (mandatory)

- [ ] Fresh home: no Memory mode tab, no Memory settings section, no "Distill selection"; Settings has an Experimental section with the Memory switch off.
- [ ] Turning the switch on and saving shows all three; off hides them again, and a window on Memory falls back to Git.
- [ ] Existing `memory.db` at boot: flag still reads off.
- [ ] Opening Settings with the flag off never opens memory.db (no Memory pane mounted).
- [ ] Visual baselines re-recorded on Linux with Inter; ARCHITECTURE notes the gate.

### P557 Baseline check (mandatory)

- [ ] Each DB package has one baseline: kira `0090` (plus `0091_p557_drop_legacy.sql`), review `0008`, memory `0008`. Baseline proof test passed at the base commit (result in plan).
- [ ] Copy of a chain-upgraded v90 DB with data opens: version 90, row counts and content hash unchanged, `integrity_check` ok, `foreign_key_check` empty.
- [ ] v82 DB refuses with the schema-too-old startup alert; v91 refuses with schema-too-new.
- [ ] `grep -rn "homefold\|configfold\|dbfold\|MigrateOnce\|app=studio\|\.kira-studio" apps internal scripts` returns nothing outside keychain names and `internal/leftovers`.
- [ ] Leftovers: flow test seeds a populated home with decoys; allowlisted items go, every decoy survives, a symlink out is refused, `~/.kira-studio` is skipped under an env override or a held lock; second run removes nothing.
- [ ] Pending release.yml patch committed under `docs/pending-changes/`; README, ARCHITECTURE, PACKAGING, DEV_ENVIRONMENT carry no fold or `--app=studio` text.
- [ ] Agent secrets: flow test moves rows from a seeded `agent-secrets.db` into kira.db, file lands under `backups/p557-*`, second boot no-op, conflicting value refuses; no plaintext anywhere in kira.db or logs.
- [ ] Keychain: user runs `go test ./internal/secrets -run Keychain` on the Mac (old only moves, both equal deletes old, both differ refuses, neither no-op); first real launch keeps connection passwords and agent credentials readable.

### P558 Repositories page check (mandatory)

- [ ] ADE top bar has a Repositories tab; the page imports, lists, configures, hides and removes repos, and manages scan folders and other files.
- [ ] Unsaved settings-file edits ask before leaving the tab or picking another repo.
- [ ] Git panel has no repositories dialog and no Manage repositories button; GitStart and "Configure repository…" land on the ADE page (or per user decision 1).
- [ ] `grep -rn "repos-dialog\|reposDialog\|ReposDialog" apps/kira-space` returns nothing.
- [ ] `ade-repos-page` baseline recorded on Linux with Inter; `git-repos-dialog` baseline deleted; ARCHITECTURE updated.

### P559 Memory home check (mandatory)

- [ ] `memory.Home()` is `<KIRA_SPACE_HOME>/memory`; memory.db, `models/`, `distilled/`, runner log and lock land there.
- [ ] `rg -n 'KIRA_MEMORY_HOME|\.kira-memory' --hidden -g '!docs/HISTORY.md' -g '!docs/v0.3.2/**'` returns only the "ignored, can be deleted" doc lines, the leftovers code and its tests (or per user decision 1).
- [ ] Fresh home, Memory off: app boot plus a `memory-mcp` start (`printf '' | <bin> memory-mcp`) leave no `memory/` folder and no `~/.kira-memory`; flow test `TestMemoryHomeLazy` passes.
- [ ] Sandbox profile allows the memory subtree only; goldens show `.kira-space/memory`, never the Space home root.
- [ ] Memory migrations: baseline `0008` only (or per user decision 2).
- [ ] README, ARCHITECTURE, DEV_ENVIRONMENT, site `space/memory` and `reference/settings` name the new path; no site baseline changed.

### P560 Agent MCP tools check (mandatory)

- [ ] `kira-space` registers 13 tools, `kira-studio` 18 with every flag on; counts asserted in tests.
- [ ] `rg -n 'kira-ade|kira-db|Kira ADE|Kira DB' -g '!docs/HISTORY.md' -g '!docs/v0.3.2/**'` returns only the user to-do lines.
- [ ] No merged tool name left outside history and eval corpora (plan "Checks" rg).
- [ ] Every merged tool's capability reachable per the plan's capability map; flow tests pass.
- [ ] Headless `db_mcp` step argv lists `kira-studio-session`, not `kira-studio`; `internal/memory` unchanged.
- [ ] `budget.tsv` lowered for every rewritten text; goldens and contracts regenerated.
- [ ] Real `claude` ADE, smart script, DB MCP, prompt registry, hermetic and agent config rows pass.
- [ ] ARCHITECTURE, DEV_ENVIRONMENT, README and site MCP pages name the new servers and tools.

### P561 Review file list follow check (mandatory)

- [ ] Review window: next change past the last hunk opens the next file; its row is selected and scrolled into view, its collapsed folder expanded. Previous does the same backwards.
- [ ] Clicking another centre diff tab, or a working-file comment jump, moves the list selection to that file and scrolls it in; no tab opens or closes.
- [ ] Only one row looks selected when the cursor moves between committed and uncommitted files.
- [ ] `rg -n "revealFile" packages apps/kira-space/frontend` shows the interface, git-ui implementation and `AdeReviewFiles.vue` caller.
- [ ] Review and git visual baselines unchanged on Linux with Inter (or re-recorded with a stated reason); ARCHITECTURE notes the follow.

### P562 Connection dialog copy check (mandatory)

- [ ] Every key in `plans/P562.md` inventory carries its planned text (or a minimal C-rule reword); `check-catalog --unused` clean; `rg -n "tablesEnforceFail" apps packages` empty.
- [ ] MCP tab no longer renders the throttle description, masking enforcement or Settings row-threshold text; each still renders in its one home (Advanced, Privacy, Settings).
- [ ] Exemption rules appear only in the MCP exemption note; no per-control "approve it in a dialog" text left.
- [ ] Touched connection UI specs pass with the new phrases; no meaning left unasserted.
- [ ] New-connection visual baseline unchanged on Linux with Inter (or re-recorded with a stated reason).

### P563 Worktrees move check (mandatory)

- [ ] New task worktree lands in `<userHome>/kira-space-wts/<repo>/wt_<id>`; board `worktreeBasePath` shows that base; no `"wt"` path join left in `apps/kira-space/internal/ade` outside tests.
- [ ] Flow test `TestWorktreesMoveOnce` passes: modified and untracked files survive, `git worktree list` shows only the new path, locked worktree moved and still locked with its reason, foreign `~/wt/other` kept, blocked target and missing folder reported and left, second boot changes nothing.
- [ ] `ade_sessions.cwd`, terminal tab cwd, local-file tab path and `script_runs.cwd` point at the new path; TUI resume of a moved session starts in the new folder; Claude transcript folder renamed (or per user decision 1).
- [ ] `rg -n '~/wt\b|<home>/wt\b' -g '!docs/HISTORY.md' -g '!docs/v0.3.2/**'` returns only `wtmove` code and its test.
- [ ] Repository config "Worktree base path" text says it pre-fills the Git module dialog only; site `space/git` and the ADE page name `~/kira-space-wts`; ARCHITECTURE documents base and move.
- [ ] Removal listed in P557 "Next chapter: delete".
