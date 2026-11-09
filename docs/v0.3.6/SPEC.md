# v0.3.6

User-requested features after v0.3.5. Chapter stays open: more phases get added later. Branch `chapter/v0.3.6`, tag `v0.3.6`. First `P` is P580 (highest prior: P579).

| Phase | Title | Status |
|---|---|---|
| P580 | Database MCP `api_request`: per-host and per-method rules with wildcards | Done |
| P581 | Review tool: diff against target branch like a source control app, fetch buttons | Done |
| P582 | Handovers: select and copy text | Done |
| P583 | Agent worktree: work on an already existing branch | Planned |
| P584 | Shared config: readable YAML file names and step names | Planned |
| P585 | Shared config: share repos by origin, with prepare-worktree configs | Planned |
| P586 | Task panel Details: open links externally | Planned |

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

## P584 Shared config: readable YAML file names and step names

- Ask (user): user wants to share their YAML configs. File names (workflow and step library files) look strange and do not reflect the step name. Step names themselves are also strange.
- Context: `internal/adeflow` (`parse.go`, step library, `StepEntry.fileName`), `internal/configsync` (module `steps`, `workflows`). Planner establishes how file names and default step names are generated today, then decides the scheme (file name derived from the step name, rename handling, collisions, default step names), the migration of existing files, and sync impact (renames across the shared repo).
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.

## P585 Shared config: share repos by origin, with prepare-worktree configs

- Ask (user): repos cannot be shared. They could be shared by carrying the origin URL. The `worktree` prepare config (`prepare_script`, `prepare_run`, `prepare_timeout`) is the valuable part to share.
- Context: `docs/ARCHITECTURE.md` "Config sync (P554)" says `repos` is never syncable (files keyed by local path; worktree prepare script must never come from a repo). This phase changes that rule: key shared repo entries by normalized origin URL, map to local registered repos by origin, share only these keys: `worktree` (`prepare_script`, `prepare_run`, `prepare_timeout`), `integration_branches`, `environments`, `review.base_candidates`. `path`, `base_path`, `nickname` and the git preference keys (`graph`, `stash`, `pull`, `git_host`, `checkout`) stay local. A repo with no origin or several origins does not sync. Planner decides the exact key set, the credential and trust gate, the command-running badge and review (as for runs, steps, workflows), and updates the architecture doc rule.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.

## P586 Task panel Details: open links externally

- Ask (user): each link in the task panel Details tab gets a button that opens it externally, in the system browser.
- Context: Details tab (`details/`, `AdeTaskChain.vue`, Jira and other link lines); existing external-open path (`TestOpenLinks` in `internal/flows/appflow/misc_test.go`, `openLink` in `AdeArtifactList.vue`). Reuse that path; the renderer sends no raw URL to a browser API of its own. Planner lists every link kind in the panel and which already have the button.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
