# v0.3.6

User-requested features after v0.3.5. Chapter stays open: more phases get added later. Branch `chapter/v0.3.6`, tag `v0.3.6`. First `P` is P580 (highest prior: P579).

| Phase | Title | Status |
|---|---|---|
| P580 | Database MCP `api_request`: per-host and per-method rules with wildcards | Planned |
| P581 | Review tool: diff against target branch like a source control app, fetch buttons | Planned |
| P582 | Handovers: select and copy text | Planned |

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
