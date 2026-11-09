# v0.3.6

User-requested features after v0.3.5. Chapter stays open: more phases get added later. Branch `chapter/v0.3.6`, tag `v0.3.6`. First `P` is P580 (highest prior: P579).

| Phase | Title | Status |
|---|---|---|
| P580 | Database MCP `api_request`: per-host and per-method rules with wildcards | Done |
| P581 | Review tool: diff against target branch like a source control app, fetch buttons | Done |
| P582 | Handovers: select and copy text | Done |
| P583 | Agent worktree: work on an already existing branch | Done |
| P584 | Shared config: readable YAML file names and step names | Done |
| P585 | Shared config: share repos by origin, with prepare-worktree configs | Done |
| P586 | Task panel Details: open links externally | Done |
| P587 | Database MCP script runner: per-script short or long running | Done |
| P588 | Defaults: step sandbox off, step Database MCP on, hermetic off | Done |

Phases run in table order, one at a time, unless the user asks for a batch.

## P580 Database MCP `api_request`: per-host and per-method rules with wildcards

- Ask (user): API access for the agent MCP is all or nothing today (Settings, Database MCP, `apiRequests` toggle). Make it configurable per host and per method, with wildcards.
- Context: `addAPITools` in `apps/kira-space/internal/dbmcp/api_tools.go`; `docs/ARCHITECTURE.md` "Database MCP" API section (`apiRequests`, `apiApproval`). Planner decides rule shape (allow/deny, host and method patterns, precedence, default), where it lives in settings, how it interacts with `apiApproval`, and the settings UI.
- Acceptance: rules match host (wildcards such as `*.example.com`) and method (`*` or a list); a request no rule allows is refused with a clear error; existing installs keep current behavior unless they add rules.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary (CLAUDE.md Tests).
- Result:
  - Rule: `{effect: allow|deny, hosts, methods}`; list leaf `dbMcp.apiRules`, at most 50 rules, default empty (allow all, existing installs unchanged). Hosts: `*` or `HostRule` entry without `http://` (`*.domain` matches subdomains, not the apex). Methods: `["*"]` or distinct members of the `api_request` enum. Deny wins, order irrelevant; once any rule exists a request needs a matching allow rule.
  - Enforcement: `agentapi.Plan` (via `Service.Access`, read fresh per plan, nil means none) before credentials and approval, so a refused request raises no item and `Send` re-plans. Every redirect hop is checked with the request's method. Unreadable stored leaf fails closed (`apiRulesInvalid`, every request refused). `apiApproval` unchanged.
  - Moved: `HostRule` from `internal/apifiles/agenthost.go` to `internal/apiaccess/hostrule.go` (plus test); new `apiaccess/policy.go`. Wired through `storage/model`, `storage/repos`, `bridge.AgentAPIAccess`, `appwire/dbmodule.go`, prompt text of `api_request`.
  - UI: Settings, Database MCP rules list and add/edit dialog.
  - Tests passing: `apiaccess` (host rule, `TestPolicyCheck`, `TestCompileRejects`), `apifiles`, `storage/...`, `internal/llmprompts`, `dbmcp`, `dbmcpflow` all `Api*` incl. `TestApiAccessRules` (contract `dbmcp-api-rules.json`), UI `dbmcp-api-rules.spec.ts` and `dbmcp-api-settings.spec.ts`; lint and `typecheck:web` clean.
  - Deviations from plan:
    - Dialog and `apiAccessRules.ts` live in `studio/workbench/apiaccess/`, not `settings/`: guard U28 bans `Checkbox` under settings paths.
    - Methods are a `Checkbox` row, not `ToggleGroup`: guard U4 allows `ToggleGroup` only in `SecondaryTabs`.
    - Remove button uses `:pending`, not `:disabled` (guard U92).
    - Dialog has a `dirty` flag (guard U100).
    - Reused i18n keys for Allow, Deny and Remove (catalog rule C3).
    - Strings reworded for copy rules (sentences split, no colons, "couldn’t"); `*.domain` explanation moved to `hosts.hint`, new `hosts.example`, `empty.body`.
    - `api_request` prompt budget in `budget.tsv` raised 282 to 363 for the added sentence.
    - Redirect error uses `next.Host`, so it reads `redirect to localhost:PORT blocked: ...`; test asserts `redirect to localhost` and `blocked`.
    - Wails bindings were stale (gitignored, pre-existing) and failed the commit-3 hook; regenerated with `wails3 task common:generate:bindings`.

## P581 Review tool: diff against target branch like a source control app, fetch buttons

- Ask (user):
  - The review tool shows the diff to the target branch the way a source control app does.
  - Reviewing the latest version of the branch already works. The target (base) branch must get the same treatment: review against its latest version.
  - When the local branch or base is not the latest, show easy buttons to fetch the correct branches.
- Context: `internal/gitsession/crreview.go` (`reviewHost`), `internal/gitrpc/incremental.go` (`handleReviewFileDiff`), `ChangeRequest.baseRef`. Planner establishes the current behavior for both refs first.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result: Plan `plans/P581.md`. Diff semantics already matched a source control app (three-dot, merge base of base and branch to branch tip); the gap was ref freshness, so nothing in `rangeFilesRecords` or `ReviewFileDiff` changed. New `ReviewSync` (read only facts per side against the remote twin: `current`, `behind`, `ahead`, `diverged`, `remote`, `noRemote`), `ReviewFetch` (default remote plus twin remotes, prune, askpass through `RunRemote`) and `ReviewUpdateLocal` (fast-forward only: `merge --ff-only` in the worktree that has the ref, else `update-ref` with old value check). `remoteTwin` is shared with the board's `fillUpstream`. Review title bar shows `AdeReviewSync`: Fetch button with last fetch age, `Update branch` and `Update base` on `behind`, `Diverged` and `not pushed` badges, change request base mismatch badge, error line. Flow tests `TestReviewSync` and `TestReviewSyncStackedBase`, UI spec `ade-v2-review-sync.spec.ts`, contract `ade-review-sync`. Ran: Go `adeflow` ReviewSync tests, `ade`, `gitsession`, `gitops`, `bridge` packages, `ade-v2-review*` UI specs (4 new, 102 existing pass), typecheck, lint. Out of scope as planned: Git sidebar review base picker, branch-onto-base update, fork branches without upstream, retargeting a locked git-ui mount.

## P582 Handovers: select and copy text

- Ask (user): text in handovers cannot be selected and copied easily. Make it so.
- Context: handover rendering in the task panel Details chain (`AdeTaskChain.vue`), stage handover (P407). Same class of fix as P577.
- Tests: UI spec; backend half only if one exists.
- Result: read-only `AdeNotesEditor` and the Run dialog handover and notes cards opt into `select-text` (root cause: body `select-none`, TipTap `contenteditable="false"` missed the `base.css` opt-in). Each chain entry gets a `useCopy` copy icon button (`ade-handover-copy`, body verbatim). UI spec `ade-v2-handover.spec.ts` covers selection, clipboard write and the Run dialog card. No backend half: no IPC, wire or Go change. Docs: ARCHITECTURE Unified notes bullet.

## P583 Agent worktree: work on an already existing branch

- Ask (user): an agent cannot check out an already existing branch in its worktree. It cannot run tests on finished work or fix something on it. User believes the cause is git refusing one branch in two worktrees, and asked which solutions exist.
- Decision (user): option 3, adopt. The agent checks out the existing branch; when another worktree holds it, Kira asks the user to detach it there first, then checks out.
- UI (user): a task's branch row shows the branch name, never `HEAD`. When two tasks use the same branch, the row shows that. A worktree whose branch was detached away shows a detached icon; hover names the task and worktree where the branch is attached now. Attached versus detached must be visible at a glance.
- Context: `request_branches` and `ensureWorktree` (`docs/ARCHITECTURE.md` "Branch requests"), `internal/gitops` worktree add, `internal/gitpreflight`.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result: Plan `plans/P583.md`. `request_branches` entry `existing: true` (outcome `checkedOut`) checks out a branch that exists in git. Item kinds `checkout` and `adopt`; adopt detaches the holder (task, main or other worktree) after user approval, then checks out. Migration 0094 `ade_task_branches.adopted`; unique live-name index now `AND adopted = 0`. Donor-busy and in-progress git operation refuse before the prompt and at apply; dirty holder files only warn. Undo on failure restores the donor. Ownership fix first: a row's worktree is its own folder (`rowWorktree`), so archive, rebase, automation and `ensureWorktree` no longer follow the branch name into another task's folder; `ensureWorktree` reattaches a detached own folder. Board: `Branch.detached` and `Branch.shared`; rows keep the stored name, with detached and shared glyphs in the Plan and Main tab. Flow test `TestBranchAdopt` (contract `ade-branch-adopt`), UI spec `ade-v2-branch-adopt.spec.ts`. Docs: ARCHITECTURE "Shared branches and detached worktrees (P583)".
  - Deviations from plan:
    - `SetBranchNameShared(id, name, adopted)` replaces the planned adopt-name helper, so a draft row is named and flagged in one write and undo can clear it.
    - No extra review-task check: `spaceWritable` already refuses.
    - `ensureWorktree` cannot run under the repo mutex, so `takeLocked` holds it for detach and row steps only.
    - Copy catalog keys renamed for rules C-depth and period: `item.checkout.label`, `item.adopt.label`, `item.adoptMain.label`, `detachedTask.tooltip` (and `detachedMain`, `detachedOther`, `detachedNone`), `shared.tooltip`, `dirty.hint`, `mainCheckout.hint`.
    - Shared glyph shows only while the row has a worktree. Reserved-name refusal reads "belongs to the repo itself". `branchInfo` with no worktree says to call `request_branches` with `existing: true`.
    - UI spec asserts the glyph tooltip text through its `aria-label` (same string); hover tooltip collided with the row tooltip.
    - Adopt dialog item has no separate holder line; the holder sits in the item label. The holder test id element holds only dirty and main-checkout hints.

## P584 Shared config: readable YAML file names and step names

- Ask (user): user wants to share their YAML configs. File names (workflow and step library files) look strange and do not reflect the step name. Step names themselves are also strange.
- Context: `internal/adeflow` (`parse.go`, step library, `StepEntry.fileName`), `internal/configsync` (module `steps`, `workflows`). Planner establishes how file names and default step names are generated today, then decides the scheme (file name derived from the step name, rename handling, collisions, default step names), the migration of existing files, and sync impact (renames across the shared repo).
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result: Plan `plans/P584.md`. Stem = `slug(name)` (`-2`, `-3`, max 60); Form and graph saves rename the file when the slug stops matching, rewrite `step:` references and `ade_tasks.workflow_id`, journal `<home>/ade/rename.json`, recovered at `Start`; shared files (`configsync.HasBase`) keep their stem; YAML saves never rename. Inline step ids are name slugs (follow the name until first save, `stp` prefix dropped from both registries); new steps are named by kind. One-time migration (`names-v1.done`, backups `backups/readable-names-<ts>`). Flow tests `readablenames_test.go` (contract `ade-readable-names`), UI spec `ade-readable-names.spec.ts`, unit `adeflow/rename_test.go`. Ran: adeflow, ade, configsync, flows/adeflow, typecheck, lint rules, UI specs ade-readable-names, ade-ids, ade-v2-workflows, -workflow-layout, -workflow-steps, consistency. Deviations: `kind.<k>.option` already reads "Agent step", so no `kindName` key; a fourth `stp_` spec (`ade-v2-workflow-steps`) updated; reserved ids (`next`, `end`, `stop`, `done`) never taken as step ids; graph rename remounts the editor on the new file, so the selected step resets; migration backup dir is `<BackupDir>-<stamp>`.

## P585 Shared config: share repos by origin, with prepare-worktree configs

- Ask (user): repos cannot be shared. They could be shared by carrying the origin URL. The `worktree` prepare config (`prepare_script`, `prepare_run`, `prepare_timeout`) is the valuable part to share.
- Context: `docs/ARCHITECTURE.md` "Config sync (P554)" says `repos` is never syncable (files keyed by local path; worktree prepare script must never come from a repo). This phase changes that rule: key shared repo entries by normalized origin URL, map to local registered repos by origin, share only these keys: `worktree` (`prepare_script`, `prepare_run`, `prepare_timeout`), `integration_branches`, `environments`, `review.base_candidates`. `path`, `base_path`, `nickname` and the git preference keys (`graph`, `stash`, `pull`, `git_host`, `checkout`) stay local. A repo with no origin or several origins does not sync. Planner decides the exact key set, the credential and trust gate, the command-running badge and review (as for runs, steps, workflows), and updates the architecture doc rule.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result: Plan `plans/P585.md`. New module `repos` in config sync, entry-level, modes local and combined, keyed by normalized origin (`internal/gitremote`, moved from `memory.NormalizeRemote`). Shared decoder in `repocfg/shared.go`, `RepoFiles` seam implemented in `appwire/configsync_repos.go`, per-target merge base, `unmatched` row kind, Runs commands badge on `worktree/prepare` and `environments/*`, target fingerprint checked at Apply, `Repos.Reload()` after commit, `RefreshAsync` on repo import, `RepoSharedText` bound plus Copy for shared config button. Flow `TestConfigSyncRepos` (contract `config-sync-repos`), UI `config-sync-repos.spec.ts`, unit `repos_test.go`. Ran: Go configsync, gitremote, memory, configsyncflow, repoflow, all flows, conventiontest; UI `config-sync`, `config-sync-repos`, `repo-config-file` (16 pass); `config-sync.json` unchanged. Deviations: labels "Repository settings" and "No local repository" (catalog lint C5, not "repo"); flow step 7 deactivates never-applied `environments/qa` (deactivating an applied entry is a delete verdict); added script credential word scan (`credentialProblem` only caught whole values); `wireAdeTask` gained an `onReposChanged` param; `RepoFiles` gained `SharedText`; `NewFile` and shared schema live in `shared.go`, not `schema.go`; fixed pre-existing G3 slog scope in `storage/repos/settings.go`.

## P586 Task panel Details: open links externally

- Ask (user): each link in the task panel Details tab gets a button that opens it externally, in the system browser.
- Context: Details tab (`details/`, `AdeTaskChain.vue`, Jira and other link lines); existing external-open path (`TestOpenLinks` in `internal/flows/appflow/misc_test.go`, `openLink` in `AdeArtifactList.vue`). Reuse that path; the renderer sends no raw URL to a browser API of its own. Planner lists every link kind in the panel and which already have the button.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result: Plan `plans/P586.md`. New `details/AdeLinkOpenExternal.vue` (`TooltipIconButton` `link-external`, testid `ade-details-open`, hidden without a url) calls `control.openExternal` to `LinkService.OpenExternal`; no new bridge method, no renderer browser API. First in `#actions` of `AdeDetailsLink` (every link row: Jira slot, host slot, detected and stored rows, read-only and archived included, hidden while editing) and of `AdeTimelineEntry` (artifact events). Change request titles keep the in-app host view; the button opens the same url externally, so forge and Azure urls open too. Rows without a url (bare key) show no button. Out of scope: handover and note prose links (inline text, Cmd/Ctrl-click kept) and `AdeTaskIssueLine` (no url of its own; the Jira row above carries the button). Copy key `space.ade.details.openExternal.tooltip`. Flow `TestDetailsOpenExternal` (contract `ade-details-open`), UI `ade-v2-details-open.spec.ts` (5 pass). Ran: flows adeflow `TestDetailsOpenExternal|TestDetailsLinks`, appflow `TestOpenLinks|TestTerminalLinkOpen`; UI details-open, details-links, details-list, task-panel, panel, artifacts, copy-coverage (96 pass); typecheck web and tests, lint, `lint:dead`, golangci-lint adeflow. Deviations: flow picks stored artifacts by `source: user` (the listing also holds derived slot rows); `openDetails` moved to `support/adeV2.ts` and shared by details-links and details-open specs (lint N8).

## P587 Database MCP script runner: per-script short or long running

- Ask (user): the MCP script runner (`run_script`, `get_script_run`) makes the AI wait a fixed 50 s for a script (`scriptWaitMax` in `apps/kira-space/internal/dbmcp/script_tools.go`). A script can be a long running process that runs for hours. Whether a script is short or long running must be configurable in the script itself.
- Context: `run_script` and `get_script_run` (`docs/ARCHITECTURE.md` "Database MCP"), the script (run/automation) YAML and editor in Runs, the agent-facing tool descriptions in the prompt registry (`internal/llmprompts`, `budget.tsv`). Planner establishes the current wait, polling and result flow first, then decides the script setting (for example a short/long mode or a wait limit), how a long script returns at once with a run id the AI polls or follows, how output and exit state stay readable while it runs, schema and validation, UI for the setting, and config sync impact (module `runs`).
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result: Plan `plans/P587.md`. Run file key `mcp.mode`: `short` (default) or `long`, written only when `long` (existing and shared files byte-identical; config sync `runs` needs no change). A long script: `run_script` and `get_script_run` default `waitSeconds` 0 (still 0-50 when passed), return at once with `mode: long`, run id, `opId` and a `next` naming `get_script_run` and `operations` cancel; `get_script_run` returns the last 50 output lines at any state; exit state reads as for a short script. `list_scripts` and run results carry `mode`; mode is read from config per call, no column. Timeout default 30m for short, 24h for long (`scripts.LongMCPTimeout`, `MCPAccess.TimeoutText`). `scripts.repo` normalizes `mode` to `short` on load like `run`. Validation `/mcp/mode`; schema, rule doc, runs intro, TS mirror, prompt descriptions and budget lines updated. UI: Short/Long RadioGroup in `RunMcpTab.vue` (`runForm.ts` unsets `short`), "Long running" Badge in `ScriptsMcpSection.vue`. Flow `TestDbMcpScriptLongRunning` (contract `dbmcp-script-long`), `TestRunsForm` (`mcpLong`, `mcpModeInvalid`); UI `dbmcp-script-long.spec.ts` (3), `automations-mcp.spec.ts` (+2). Docs: ARCHITECTURE "Scripts tools", site `mcp-tools.md`. Ran: Go scripts, scriptruns, llmprompts, dbmcp, bridge, dbmcpflow, termflow; golangci-lint on changed dirs; `typecheck:web`, `typecheck:tests`, `lint`, `lint:dead`; UI 13 pass (`dbmcp-script-long`, `automations-mcp`); real `claude` `test:claude:db -run TestDbMcpScript`. Deviations: copy labels use a comma not a colon (catalog rule C6); `scripts/repo.go` mode normalization added so the TS enum accepts listed scripts; flow contract keeps script ids unmasked (slugs) so the UI row ids match.

## P588 Defaults: step sandbox off, step Database MCP on, hermetic off

- Ask (user): "In the workflow config make sandbox off by default. Kira db on by default, and for the global setting have hermetic mode off by default."
- Context: step `sandbox` (`adewire.DefaultSandbox`, `adeflow` parse, writer and new-step, `stepSandbox.ts`), step `db_mcp` (`parse.go` `stepExtras`, `applyDbMcp`), `agent.hermetic` (`internal/appsettings/agent.go`, `settings.ts`, `settingsDomain.ts`). Direct implementation, no Opus plan (user request: change is simple).
- Tests: Go adeflow, ade, adewire, appsettings; flow contracts regenerated for the default flip; UI specs that assert the three defaults.
- Result: no plan (user asked for direct implementation). All three defaults were as the user described.
  - Step sandbox: `adewire.DefaultSandbox()` is access off for every kind (was write for agent and review). `DefaultSandboxFor`, `NormalizedFor(kind)` and `IsDefaultFor(kind)` lost the kind (`Normalized()`, `IsDefault()`, `stepSandbox.ts` `defaultSandbox()`). A `sandbox` block without `access` and rebase runs use new `adewire.WriteSandbox()`, so an explicit block still confines. The writer drops the block when the policy is off. Inspector shows a hint, not a warning, for off on agent steps (key `offAgent.hint`, `offWarning.body` removed); the node badge hides for the default.
  - Step `db_mcp`: an absent key parses as on where it applies (agent without `smart_script`, review, manual with `session`); `db_mcp: false` is off. Existing files without the key now get the tools. The writer drops the key when on and writes `db_mcp: false` when off; library override restore (`explicitOff`) writes `true`. New agent and review steps (Go `withTimeout`, `workflowForm.ts`) and new manual sessions start on; switching a smart step back to prompt sets it on.
  - `agent.hermetic`: default false in `appsettings.DefaultAgent`, `settings.ts` and `settingsDomain.ts`. Unchanged on purpose: nil `Hermetic` readers stay on (scriptruns and agentcfg serve Studio, always hermetic) and a settings read error still launches hermetic (fail closed).
  - Running tasks keep their snapshot (stored access and `dbMcp`) and show `workflowOutdated` until they end; no migration.
  - Tests: contracts regenerated (diff is only `access` write to off, `dbMcp` false to true, hashes). `ade` test boards get a `fakeDbMcp`. Sandbox, rename and context flows name `access: write`; hermetic flows call `SetHermetic(true)`. Fixed on the spot, failing at the phase start: `journeyflow` restart (P584 file rename) and `llmpromptsflow` TestList (266 prompts).
