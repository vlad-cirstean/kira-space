# v0.3.7

User-requested features after v0.3.6. Chapter stays open: more phases get added later. Branch `chapter/v0.3.7`, tag `v0.3.7`. First `P` is P590 (highest prior: P589). Batch size: 3 phases at once (user request); grouping in `plans/batches.md`.

| Phase | Title | Status |
|---|---|---|
| P590 | Kira Studio approval prompt: skip when the database connection cannot be established | Todo |
| P591 | Proof tool: behavior not code, larger explanation, terse guidance | Todo |
| P592 | Jira: faster loading | Todo |
| P593 | Database MCP: customizable explain threshold | Todo |
| P594 | Open GitHub links inside the app: fix | Todo |
| P595 | `api_request` rules: default action ask or deny | Todo |
| P596 | Masking and the Privacy tab: make it understandable | Todo |
| P597 | Proof: export all screenshots to a Desktop folder, location customizable | Todo |

Phases run in table order. Batches of 3 consecutive phases run as streams.

## P590 Kira Studio approval prompt: skip when the database connection cannot be established

- Ask (user): the Kira Studio prompt should not show if the database connection cannot be established.
- Context: `internal/dbmcp` approval flow (`ApprovalBroker`, `run_query` path). Planner finds where the prompt is raised relative to connecting, and whether the approval can wait for a successful connect.
- Acceptance: a request against a connection that cannot connect returns the connection error without raising an approval item.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result:

## P591 Proof tool: behavior not code, larger explanation, terse guidance

- Ask (user):
  - The proof tool states that a proof is about functionality and behavior, not code.
  - The proof window gives the explanation more space by default and a bigger font; it is the most important part of a proof.
  - Tool text guides the agent to keep the explanation terse but clear.
- Context: `internal/dbmcp/proof_tools.go` (`addProofTools`), proof window UI. Prompt budget rules in `internal/llmprompts`.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result:

## P592 Jira: faster loading

- Ask (user): Jira is still very slow. Consider a more efficient approach.
- Context: `apps/kira-space/internal/issuetracker/jira`. Planner measures first (calls per view, payload size, pagination, caching, `acli` process spawn cost) and picks the approach with evidence.
- Result:

## P593 Database MCP: customizable explain threshold

- Ask (user): the explain threshold cannot be customized.
- Context: `ExplainThreshold` in `bridge/dbmcp.go` `serverConfig`; `ConnectionMcpTab.vue` (`auto_explain`). Planner decides the scope (global, per connection, or both) and where the setting lives.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result:

## P594 Open GitHub links inside the app: fix

- Ask (user): opening GitHub links in the app itself is broken.
- Context: link handling for GitHub URLs (issues, pull requests) in the task panel and elsewhere; P586 moved Details links to the system browser. Planner reproduces first.
- Tests: UI spec; backend half only if one exists.
- Result:

## P595 `api_request` rules: default action ask or deny

- Ask (user): API allow lists block everything else once a rule exists. With the default "ask for everything", the model is allow and deny rules plus a default case that is ask or deny.
- Context: P580 rules (`internal/apiaccess/policy.go`, `dbMcp.apiRules`, `apiApproval`). Today any rule makes unmatched requests refused; replace by an explicit default.
- Acceptance: default is `ask` or `deny`; existing installs keep behavior.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result:

## P596 Masking and the Privacy tab: make it understandable

- Ask (user): masking is confusing; the Privacy tab is not understandable.
- Context: Privacy tab, `MaskRules`, `MaskReview` in `serverConfig`, column review flow. Planner explains the current model first, then proposes a redesign the user can read.
- Tests: flow test plus UI spec where behavior changes.
- Result:

## P597 Proof: export all screenshots to a Desktop folder, location customizable

- Ask (user): export all screenshots creates a folder by default and saves it on the Desktop. The save location is customizable from the export window.
- Context: proof screenshot export (`ProofSaveJpegArgs`, save flow).
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result:
