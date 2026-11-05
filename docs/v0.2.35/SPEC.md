# v0.2.35

Branch `chapter/v0.2.35`, based on `chapter/v0.2.34` (v0.2.34 not yet merged to main). First `P` is 512 (highest in v0.2.34 is P511), last is P527.

Source: user request. Sixteen phases. Close duty after the last phase lands: one full suite run, 2 code review rounds (user ask), hand-written close. After the close and push, open the PR to main (CLAUDE.md close rules), subscribe to it, and fix failing checks until it merges. Each phase: Opus plan, Sonnet implementation.

Parallelism (user ask): up to 3 phases run at once as named streams, each in its own worktree off the same base, zero file overlap. P512 to P517 landed. Remaining waves: P518 + P519 + P524 in parallel, then P520 alone (touches every approval owner), then P521, then P522 (sequential: both edit the Studio MCP server, settings and prompt registry), then P523 alone (touches every id generator), then P527 alone (DB MCP tools, adapterhost and adapters; after P520 to P523). P525 ran ahead of P520 by user direction (frontend-only). P520 implementation starts after P526 lands. From P512 on, new Kira-generated ids follow the P523 convention where cheap.

| Phase | Title | Status |
|---|---|---|
| P512 | Tracked-item update indicators with per-concept settings | Done |
| P513 | Repo scripts reference run-module scripts (backend, YAML) | Done |
| P514 | YAML syntax highlighting and restored config dialogs | Done |
| P515 | Run-script picker in the restored repo dialog | Done |
| P516 | Reveal button on every secret field | Done |
| P517 | Multiple worktrees per repo, agent-named branches, branch rename | Done |
| P518 | Cancel ops via MCP and status bar running-ops widget | Done |
| P519 | Proof window cell editor and multi-file screenshots | Done |
| P520 | Aggregated MCP approvals: one dialog, minimise to toast, long approval timeouts | Done |
| P521 | Scripts MCP | Done |
| P522 | Free-form API request MCP with agent credentials | Pending |
| P523 | Prefixed nanoid for every Kira-generated id | Pending |
| P524 | Manual order for API collections and environments | Done |
| P525 | Uniform flat design: dialogs and outline buttons | Done |
| P526 | Standalone step library and Workflows page tabs | Done |
| P527 | DB MCP declared query mode enforced by the database | Pending |

## Rows

### P512 Tracked-item update indicators with per-concept settings

- Ask: Task shows at a glance that a tracked item changed since last seen: PR (new comments, merged, checks failed), Jira issue, branch, and the other trackable kinds.
- Meaningful vs noise is configurable in Settings, per trackable kind and per concept, independently. Examples: PR comment added, PR merged, checks passed, checks failed (two separate toggles), assignee change. Defaults: comments, merge/close, checks failed, status change on; assignee, checks passed off.
- Plan: `plans/P512.md`.
- Result: Done. Githost sync fields and snapshots with diff, watcher and bridge, per-concept Settings toggles, ADE indicators, flow and UI specs, docs. `CHANNEL.adeTaskUpdates` added. Real-hardware GitHub sync not run; comment and assignee fields covered by fakehost only.

### P513 Repo scripts reference run-module scripts (backend, YAML)

- Ask: Repo scripts (prepare worktree, check deployments, etc.) may reference a script from the Run module and run it the same way workflows do.
- Scope (user decision): normal Run scripts only; smart scripts refused with a clear message. Picker UI is P515.
- Plan: `plans/P513.md`.
- Result: Done (backend, YAML, docs). Keys `worktree.prepare_run` and `environments[].deployed_sha_run`, `model.ScriptRef`, shared `resolveScriptRef`. Smart scripts refused for repo use. Picker UI is P515.

### P514 YAML syntax highlighting and restored config dialogs

- Ask: Every config YAML view/editor gets syntax highlighting. Bring back the form config dialogs; YAML stays hand-editable, dialogs edit the same files, human-friendly as before.
- Scope (user decision): also restore the lifecycle actions the old dialogs had (connection delete and duplicate, API rename/delete/move, run collection editing).
- Plan: `plans/P514.md`.
- Result: Done in 17 commits, groups A to F. Monaco YAML highlighting on every config editor, `configfs.Form` with `useConfigFormDraft` (form and YAML edit one file), restored run, repo, connection and API dialogs with lifecycle actions (duplicate, delete, rename, move). Deviation: no manual collection or environment order, collections list by name (see P524). Duplicate environment shares secret refs.

### P515 Run-script picker in the restored repo dialog

- Ask: Repo dialog (restored by P514) lets the user pick a Run script for the prepare worktree and deployed-SHA hooks, using the P513 keys; params editable. Depends on P513 and P514.
- Plan: `plans/P515.md`.
- Result: Done in 4 commits. `RunScriptRefPicker` and `RepoScriptField` (Command and Script tabs) in the repo dialog for prepare and deployed-SHA hooks; `ParamValues.MarshalJSON` keeps one-or-list shape. Flow `TestRepoScriptPicker` and UI spec `repo-script-picker`.

### P516 Reveal button on every secret field

- Ask: Every place a secret is entered or shown has a reveal (eye) toggle, including dialogs restored by P514.
- Plan: `plans/P516.md`.
- Result: Done in 3 code commits plus docs. Shared `SecretInput` with eye toggle at every secret entry and display, no Go change. Revealed value drops on `set` change. Spec `secret-reveal` plus call-site specs.

### P517 Multiple worktrees per repo, agent-named branches, branch rename

- Ask: A task may hold several worktrees for one repo (several branches in one repo). Worktree dir name is a nanoid. Branch name is a required argument of the MCP create tool, chosen by the agent. MCP tool can rename an existing branch, gated by an approval dialog.
- Plan: `plans/P517.md`.
- Result: Done. Several worktrees per repo (`wt_` nanoid dirs, migration 0075), `request_branches` takes agent-chosen names (breaking), `rename_branch` MCP tool with approval dialog. Flow tests and UI specs for multi-worktree and rename. Real `claude` test not run.

### P518 Cancel ops via MCP and status bar running-ops widget

- Ask: Agent can stop a running query via MCP, and any request listed in the ops panel. Status bar widget lists running ops with a stop action per op.
- Plan: `plans/P518.md`.
- Result: Done in 7 commits. `kira-db` tools `list_operations` and `cancel_operation` (ops on MCP-exposed connections plus `run:<id>` script runs; one refusal text for unknown or hidden ids; no early-cancel memory). Shared status bar running-ops widget in both apps, Stop per row (Space hides background rows, disables non-cancellable ones). Flow tests `TestDbMcpCancelOperation`, `TestDbMcpCancelScriptRun`; UI spec `running-ops-status` in both apps. Real `claude` rows not run.

### P519 Proof window cell editor and multi-file screenshots

- Ask: Proof window gets the cell editor (read-only is fine). Screenshots are split into several files, not one long PNG.
- Plan: `plans/P519.md`.
- Result: Done as planned (D1-D6). Proof window shows a read-only cell editor under table and key-value results. Save-all writes one PNG per tab into a fresh folder; stacked copy-all dropped. Pre-existing `cell-editor.spec.ts` failures fixed (wildcard `connectionConfig:form` mock). Flow `proofflow`, UI specs `proof-window`, `proof-chart`. Real `claude` row not run.

### P520 Aggregated MCP approvals: one dialog, minimise to toast, long approval timeouts

- Ask (user, revised): Tools stay synchronous; async dropped (no handles, Tasks, stdin injection). One approvals dialog per app holds every pending agent approval: Studio DB MCP statements, result previews, Kubernetes reads, proof edits; Space branch requests and renames; later P521 scripts and P522 API requests as new item kinds. Shown by default. Close does not deny: it minimises to a top-right toast ("N things awaiting your approval", countdown, reopen button) that updates as items come and go and disappears when empty. Per-item approve or deny, plus approve all and deny all over visible items. Each call returns as soon as its own item is answered.
- Item presentation: target (name, kind, colour), operation type with tone, agent or session origin, then payload. SQL stays formatted. API items (P522) parse like SQL: method and URL, headers, query, body by content type, masked auth, resolved variables.
- Studio approval timeout 120 s to 10 min; Kira's `MCP_TOOL_TIMEOUT` kept consistent; per-server MCP `timeout` written by Studio's install and `agentmcp`'s per-run config, so Claude's 60 s default cut no longer applies.
- Space: concurrent `request_branches`/`rename_branch` calls for one task merge into one open request (unique item ids, per-item remote rename, per-call origin).
- P525 rules apply (outline buttons, no header icons). Async research kept as record: `plans/P520-async-research.md`.
- Plan: `plans/P520.md`.
- Result: Done. Shared `internal/approvals` queue (kinds `db.query`, `db.response`, `kube.read`, `proof.review`, `branch.request`; `Kind` is the subject plus an optional `Subject`), bound as `ApprovalsService`, one routed prompt `approval:<id>` per item. Shared frontend in `packages/workbench/src/approvals/`: kind registry, per-item cards with countdown, Approve and Deny, origin line, app-wide dialog, X or Escape minimises to the top-right toast and never denies, Approve all and Deny all. Studio renderers for the four kinds; Space `BranchRequestBody`. DB MCP `ApprovalTimeout` 10 min with client origin; per-server MCP `timeout` in Studio install and `agentmcp`; Space merges concurrent branch calls into one item (`c<call>i<k>` ids, per-item remote rename). Flow tests record `approvals-studio`, `approvals-space` and the reworked dbmcp and ade contracts; UI specs `approvals` in both apps, rewritten dbmcp, kubernetes-mcp, prompts, copy-coverage and ade-branch specs; Studio visual baselines for dialog and toast.
- Deviations: no cwd in the origin header (the client sends none; Known open items). Hook-coupled frontend and backend commits grouped, since the typecheck hook fails any split. No fake-clock expiry flow: the 10 min bound is asserted as `ExpiresAt - CreatedAt` plus a unit test with an injected clock. The adeflow timeout assertion stays in the `agentmcp` unit test. Frontend kinds have no `title` hook (Go `Item.Title` is the prompt title). `useCountdown` replaces `usePendingDecision`, which steals focus. Toast pulse dropped (U23). Space specs seed the list answers instead of pushing, since the first fetch raced the push.
- Pre-existing failures fixed in passing: llmprompts counts and contract, gitflow `commentCount` contract, G4 in `adeflow/migrate.go`, G1 in `scripts/lifecycle.go`. `apps/kira-space` Go build needs `frontend/dist-mobile`, absent in this sandbox.
- Checks: lint, typecheck, knip, check-catalog, Studio and Space UI specs (Space full suite 1135 passed), Go flows for touched packages. Real `claude` row not run.


### P521 Scripts MCP

- Ask: Agents run a script and fetch its logs via MCP. Log fetch capped at a max line count; simple filter (planner picks, e.g. tail/head and substring). Per-script MCP access is explicit opt-in. Per-script choice to show an approval prompt or not. Scripts get descriptions, as connections have. Tools stay synchronous; approval prompts are an item kind in the P520 approvals surface.
- Plan: `plans/P521.md`.
- Result: Done in 10 commits. Opt-in `mcp` block per normal script, four tools (`list_scripts`, `run_script`, `get_script_run`, `get_script_logs`), headless `mcp` runs, filtered capped log reads, approval item `script` with masked secrets and client origin, MCP tab, Settings list, flow and UI specs. Deviation: P520 not on base, so the approval dialog has a `v-if` branch for the script subject; P520 moves it into its registry.

### P522 Free-form API request MCP with agent credentials

- Ask (user, revised): One free-form request tool, no access to saved requests (no list, get or save). The agent builds method, URL, headers, query and body, and may reference environment variables by name (`{{base_url}}`). The user approves each request in the P520 approvals surface, item kind `api.request`: method and URL prominent, environment, host, headers and params, body formatted by content type, masked auth, "uses credential: NAME" lines, tone by method risk. Tools stay synchronous.
- Credentials (user-approved design): a variable is usable by agents only when marked available, per environment, in the environment file (default off); agents see names, never values. Each opted-in secret has an allowed-host list (default from the environment's base-URL variable), enforced per host. A credential bound for an unlisted host, directly or by redirect, is blocked with an error, never a prompt. Responses and echoes are masked (`secrets.Masker`) and size-capped before they reach the agent. No reveal path. Managed in the Environments view beside Secrets, form and YAML over one file (P514).
- Global opt-in and approval mode in Settings, Database MCP; tool descriptions editable in Settings, Prompts. Covers timeouts, size caps, TLS, no cookie sharing between calls, masked op log.
- Runs after P521 (shared MCP server, settings and prompt files).
- Plan: `plans/P522.md`.

### P523 Prefixed nanoid for every Kira-generated id

- Ask: Every id Kira generates, of any kind, in the Kira apps is a nanoid with a short prefix. The prefix names the feature the id belongs to. External ids (GitHub, Jira, Docker, etc.) stay unaltered.
- Plan decides: inventory of all generators (uuid, ulid, autoincrement-as-id, hand-rolled), one shared id helper and prefix registry per language, handling of stored existing ids (migrate or leave, with reasoning), lint guard against new unprefixed generators. Uses an existing nanoid library.
- Plan: `plans/P523.md`.

### P524 Manual order for API collections and environments

- Ask: Restore manual (drag and drop) ordering of API collections, API items within collections, and environments. P514 dropped it; they list by name now.
- Order persists. Backend order field plus migration in `apifiles` if needed. UI drag and drop. Paired Go flow test and UI spec.
- Plan: `plans/P524.md`.
- Result: Done in 7 commits, no migration. Order is app state in `sort_order`: new rows append, rename keeps the place, forget reindexes. `ReorderCollections` and `ReorderEnvironments` restored (`apifiles.Store`). Collection drag, Move up/down and Alt+Arrow in the tree; grip drag and keyboard reorder for environments. Flow `TestApiOrder`, UI spec `api-order`. `api-lifecycle` contract regenerated, diff is collection order only.

### P525 Uniform flat design: dialogs and outline buttons

- Ask: Dialogs adopt the flat design. Filled buttons become the outline (or toolbar) style used overall. Fix any other design inconsistency found (radius, borders, shadows, spacing, header/footer layout, icon buttons, destructive styling, inputs, tokens). Both apps and shared packages.
- Runs before P520 (user direction, frontend-only). P520 to P522 UI adopts the P525 rules.
- Plan: `plans/P525.md`.
- Result: Done in 8 commits. No filled button remains: `dialog-primary` is the accent outline, `toolbar-primary` and `danger` removed, ADE actions use tone outlines (`ACTION_CLASS`), indicators use `SOLID_CLASS`. One `floatingSurface` recipe, no header icons, toggle state via aria. Guards U106 to U108. Visual baselines re-recorded.

### P526 Standalone step library and Workflows page tabs

- Ask (user): "Stops should be mostly stand alone and then an workflow to be able to add stuff on top. Also the workflow page needs tabs for statuses steps and then workflows".
- Reading: "Stops" means steps. Steps become standalone library files (`<home>/steps/<id>.yaml`); a workflow references them and adds on top (per-workflow overrides, extra prompt, its own routes, inline workflow-only steps). Workflows page gets tabs Statuses, Steps, Workflows. Existing inline steps move into the library once, with a backup. Space ADE only, no SQL migration.
- Plan: `plans/P526.md`.
- Result: Done in 7 code commits plus docs, no SQL migration. Step library `<home>/steps/*.yaml`, workflow references with overrides, `routes` and `prompt_extra`, diff writer, one-time migration with `.p526-backup`, Statuses/Steps/Workflows tabs, library editing (form and YAML), flow tests `TestStepLibrary*`, UI spec `ade-v2-workflow-steps`. Library edits apply to new work only.

### P527 DB MCP declared query mode enforced by the database

- Ask (user): "The db mcp tool should use dedicated read and write methods that actually enforce the permissions on db level or by using 2 connections one for read and one for write depending on the db."
- Scope (user decisions): one tool, `run_query`, gains a required `mode` (read, write, ddl). Read runs on a per-connection read lane (second connection, same credentials, engine session-level read-only); write and ddl run on the main connection under that mode's own permission. The classifier only refuses a statement more powerful than its mode. `list_connections` reports allowed modes and approval per mode. No second credentials; Mongo and Redis reads stay app-checked (known limit). Runs after P523.
- Plan: `plans/P527.md`.
