# v0.3.6

User-requested features after v0.3.5. Chapter stays open: more phases get added later. Branch `chapter/v0.3.6`, tag `v0.3.6`. First `P` is P580 (highest prior: P579).

| Phase | Title | Status |
|---|---|---|
| P580 | Database MCP `api_request`: per-host and per-method rules with wildcards | Planned |
| P581 | Review tool: diff against target branch like a source control app, fetch buttons | Planned |
| P582 | Handovers: select and copy text | Planned |
| P583 | Agent worktree: work on an already existing branch | Planned (design choice pending) |
| P584 | Shared config: readable YAML file names and step names | Planned |
| P585 | Shared config: share repos by origin, with prepare-worktree configs | Planned |
| P586 | Task panel Details: open links externally | Planned |

Phases run in table order, one at a time, unless the user asks for a batch.

## P580 Database MCP `api_request`: per-host and per-method rules with wildcards

- Ask (user): API access for the agent MCP is all or nothing today (Settings, Database MCP, `apiRequests` toggle). Make it configurable per host and per method, with wildcards.
- Context: `addAPITools` in `apps/kira-space/internal/dbmcp/api_tools.go`; `docs/ARCHITECTURE.md` "Database MCP" API section (`apiRequests`, `apiApproval`). Planner decides rule shape (allow/deny, host and method patterns, precedence, default), where it lives in settings, how it interacts with `apiApproval`, and the settings UI.
- Acceptance: rules match host (wildcards such as `*.example.com`) and method (`*` or a list); a request no rule allows is refused with a clear error; existing installs keep current behavior unless they add rules.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary (CLAUDE.md Tests).

## P581 Review tool: diff against target branch like a source control app, fetch buttons

- Ask (user):
  - The review tool shows the diff to the target branch the way a source control app does.
  - Reviewing the latest version of the branch already works. The target (base) branch must get the same treatment: review against its latest version.
  - When the local branch or base is not the latest, show easy buttons to fetch the correct branches.
- Context: `internal/gitsession/crreview.go` (`reviewHost`), `internal/gitrpc/incremental.go` (`handleReviewFileDiff`), `ChangeRequest.baseRef`. Planner establishes the current behavior for both refs first.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.

## P582 Handovers: select and copy text

- Ask (user): text in handovers cannot be selected and copied easily. Make it so.
- Context: handover rendering in the task panel Details chain (`AdeTaskChain.vue`), stage handover (P407). Same class of fix as P577.
- Tests: UI spec; backend half only if one exists.

## P583 Agent worktree: work on an already existing branch

- Ask (user): an agent cannot check out an already existing branch in its worktree. It cannot run tests on finished work or fix something on it. User believes the cause is git refusing one branch in two worktrees, and asked which solutions exist.
- Status: user has not yet chosen. Options put to the user: (1) detached checkout of the branch tip (run tests, no commits on the branch); (2) new branch forked from the existing one, merged or fast-forwarded back; (3) adopt: the worktree that holds the branch releases it (detaches) and the agent's worktree takes it; (4) point the agent at the worktree that already holds the branch (`existing` outcome of `request_branches`); (5) `--ignore-other-worktrees` (unsafe, listed for completeness). Plan starts only after the user picks.
- Context: `request_branches` and `ensureWorktree` (`docs/ARCHITECTURE.md` "Branch requests"), `internal/gitops` worktree add, `internal/gitpreflight`.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.

## P584 Shared config: readable YAML file names and step names

- Ask (user): user wants to share their YAML configs. File names (workflow and step library files) look strange and do not reflect the step name. Step names themselves are also strange.
- Context: `internal/adeflow` (`parse.go`, step library, `StepEntry.fileName`), `internal/configsync` (module `steps`, `workflows`). Planner establishes how file names and default step names are generated today, then decides the scheme (file name derived from the step name, rename handling, collisions, default step names), the migration of existing files, and sync impact (renames across the shared repo).
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.

## P585 Shared config: share repos by origin, with prepare-worktree configs

- Ask (user): repos cannot be shared. They could be shared by carrying the origin URL. The `worktree` prepare config (`prepare_script`, `prepare_run`, `prepare_timeout`) is the valuable part to share.
- Context: `docs/ARCHITECTURE.md` "Config sync (P554)" says `repos` is never syncable (files keyed by local path; worktree prepare script must never come from a repo). This phase changes that rule: key shared repo entries by normalized origin URL, map to local registered repos by origin, share only portable keys (not `path`, not machine-local values). Planner decides the exact key set, the credential and trust gate, the command-running badge and review (as for runs, steps, workflows), and updates the architecture doc rule.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.

## P586 Task panel Details: open links externally

- Ask (user): each link in the task panel Details tab gets a button that opens it externally, in the system browser.
- Context: Details tab (`details/`, `AdeTaskChain.vue`, Jira and other link lines); existing external-open path (`TestOpenLinks` in `internal/flows/appflow/misc_test.go`, `openLink` in `AdeArtifactList.vue`). Reuse that path; the renderer sends no raw URL to a browser API of its own. Planner lists every link kind in the panel and which already have the button.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
