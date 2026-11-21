# v0.3.23

Chapter v0.3.23, branch `chapter/v0.3.23`, tag `v0.3.23`. First phase `P716` (highest phase: `P715`,
in open chapter v0.3.22's `SPEC.md`).

Code review at chapter end: 2 rounds (user request).

Branched from `chapter/v0.3.22` (HEAD `5a64b72c3`), whose PR is not open yet. Rebase onto `main`
before this chapter's PR opens. PR is held until v0.3.22's PR merges (CLAUDE.md "PR CI").

P716 to P720 are design phases: plan proposes the design with screenshots, user approves it
before implementation starts.

| # | Phase | Status |
|---|-------|--------|
| P716 | Rework MCP chaining for DB, API and kube: kept variables, no chain UI | Todo |
| P717 | Result review for every MCP tool that returns data | Todo |
| P718 | Kube MCP: JS script chaining, approval prompts like database and API | Todo |
| P719 | API MCP: one-time variable values typed in the approval prompt | Todo |
| P720 | API module variables and environments: UX redesign | Todo |
| P721 | Remove development, debugging and validation leftovers | Todo |
| P722 | Window tabs: tear off and re-attach, new-window button without text | Todo |
| P723 | Remove the PR flake rerun workflow | Todo |

## P716

User decision: removing the old chain steps panel and reworking MCP chaining is its own phase, first
in the chapter. Rework chaining for all MCP tools that support it (DB, API, kube). Removes the existing
`ChainSteps` panel ("Earlier / This step") from the DB and API prompts; P715 (v0.3.22) ships it. Kept
variables are secret or plain; the user can preview both. Secrets are revealed behind the same
local-auth fingerprint prompt the app uses for other secrets (`apps/kira-space/internal/localauth`,
credential-reveal flow, spec `apps/kira-space/tests/ui/studio/credential-reveal.spec.ts`). All of this
holds under both models below.

Two models. The plan proposes both, compares them (clicks, clarity, backend cost, risk) and recommends
one; user decides.

- A. No chain UI. A script stores a result in a kept variable and the agent makes a follow-up request
  that reads it. No chain panel, no chain steps.
- B. User alternative: "keep the chaining but have it like its own prompt. I see the first step, I can
  also review the results. Then I can see the next and allow it. I can also go back to the previous one.
  It has to be simple and uniform with what I already have." Each step is its own approval prompt, same
  dialog and look as every other prompt. The dialog lets the user review each step's result (existing
  result review), move forward to the next step's prompt and approve it, and go back to a previous step
  (read-only view of what was approved and its result). Uniform with the existing queue (counter, Deny
  all) and P715's design; no separate chain panel.

Under B the plan states what the backend must support: how steps are linked (by kept values or an
explicit chain id), what the old `ChainSteps` code gives and what is reused, and whether a previous
step's result is still available after review (retention, secrets masked, memory cap, what "go back"
shows once the result was released to the agent).

Result review is P717; the design here shows it under both models, uniform across DB, API and kube.

P717 (result review) and P718 (kube) depend on this phase: they follow the model the user picks, so
neither ships a panel from the old `ChainSteps` UI.

Agent awareness (user: "The AI should be aware of the chaining capability by storing those variables
and secrets"). Agent-facing text in `internal/llmprompts/registry.go` (checkprompts rules: terse,
within budget) tells the AI: a script can store a result in a kept variable, plain or secret, and a
follow-up request reads it; DB, API and kube support this; `list_kept_variables` shows what is
stored (names, kinds, source, expiry, never secret values); secret values are never shown to it.
Today (read in `registry.go`): the connect-time `DbmcpInstructions` (sent through `ServerOptions`
Instructions, `server.go`) never mentions kept variables or chaining; only the `run_query`,
`api_request` and `list_kept_variables` tool descriptions do, and no kube tool description does.
So the agent learns it per tool, not at connect time: the plan adds it to the instructions, the
DB/API/kube tool descriptions and `list_kept_variables`.

Deliverable. Design phase: plan with screenshots (before: real frontend; proposal: prototype of A and B), user
approves, then implementation.

Tests. Flow test: script keeps a plain and a secret value, follow-up request reads both, secret
absent from AI-facing output. UI spec: same scenario, preview of plain and secret, fingerprint prompt
on secret reveal, old chain panel gone from DB and API cards; under B also step forward and back; result review in the same scenario for DB, API and kube. Prompt text changes: run the matching row of
`docs/DEV_ENVIRONMENT.md` "Real `claude` tests" (`bun run test:claude:db`); the agent finds and uses
kept variables unprompted.

Open questions. A or B (user decides after the plan's comparison). Under B: how steps are linked, and
whether "go back" shows a released result.

## P717

User: "review results should work for all MCP tools that handle data: DB, API, kube." Own phase, after
P716 (chaining; review is shown under both of its models) and before kube (P718).

Today (read in code). DB and API let the user review a result before the AI gets it.
`ReviewResultOption.vue` is the `footerAction` of `db.query`, `db.exemption` and `api.request` in
`frontend/src/studio/approvals/kinds.ts`: a "Review result" checkbox, shown checked and disabled when
connection or environment settings already turn review on (`reviewOffered`, `reviewResult` in the
payload). The server runs the call, holds the result (`reviewResponse` in `internal/dbmcp/tools.go`,
size and wait limits, withheld on timeout) and queues a second item rendered by `DbResponseBody.vue`
or `ApiResponseBody.vue` with `ResultReview.vue` (Send to AI / Withhold, table or text view, staged
kept values). Defaults: per connection `McpReview` read, write and DDL (`reviewFor`, `reviewForMode`
in `permissions.go`; `browse_table` uses its mode alone); API `apiReviewFor` (`api_review.go`; a script
reading a secret kept value forces review). Kube has none (`kube_tools.go`, `KubeReadBody.vue`); an
earlier planner called kube review its own phase and the user confirms: it is this phase, for every
data-returning tool.

Tools registered with `mcp.AddTool` (read in `internal/dbmcp`): `list_connections`, `list_children`,
`describe`, `browse_table`, `run_query` (`server.go`); `api_list_environments`, `api_request`
(`api_tools.go`); `kube_list_clusters`, `kube_discover`, `kube_list_resources`, `kube_describe`,
`kube_pod_logs` (`kube_tools.go`); `list_scripts`, `run_script`, `get_script_run` (`script_tools.go`);
`operations` (`ops_tools.go`); `list_kept_variables` (`kept_tools.go`). Call sites of the review hold
today: `run_query`, `browse_table` (masked mode), `api_request`. The audit confirms the rest by tool.

Scope. (1) Audit: per tool, what data it returns to the AI, whether review exists, and whether review
fits (metadata-only lists may stay unreviewed; the plan argues each, user decides). (2) One UX and one
component for all: "Review result" checkbox in the prompt footer, result dialog, Send to AI / Withhold,
table, text, JSON and log views, secret masking info. (3) With a transform `script`, review shows what
the AI would receive, raw view alongside (as `api-review-raw`). (4) Settings and defaults: what exists
(`McpReview`, API review setting, `reviewResolver`), whether kube and others get an always, per call or
by rule setting, and how it reads in Studio MCP settings. (5) Backend: one held-result path with the
same size and wait limits and refusal texts for every tool, no per-tool copy. P716's chaining models
(A or B) show review the same way.

Deliverable. Design phase: plan with the audit table, screenshots (before: real frontend; proposal:
prototype in P715's style) for DB, API and kube, user approves, then implementation.

Tests. Flow test per tool class (`internal/flows/dbmcpflow/`): call held for review, Send releases the
result, Withhold and timeout withhold it, transformed result is what review shows and the AI gets,
masking as the plan defines. UI spec (`tests/ui/studio/`, next to the DB and API review specs): same
scenario from the prompt: tick Review result, result dialog views, Send to AI and Withhold, same result
as the flow test. Visual baselines for new dialogs. Prompt text changes: run the `bun run test:claude:db`
row of `docs/DEV_ENVIRONMENT.md` "Real `claude` tests".

Open questions. Which metadata-only tools get review. Defaults per tool (always, per call, by rule).

## P718

User: "Add js script chaining for kube mcp as well. Make sure the prompt dialogs are like for db and
api."

Today. Kube tools live in `apps/kira-space/internal/dbmcp/kube_tools.go`: five read tools
(`kube_list_clusters`, `kube_discover`, `kube_list_resources`, `kube_describe`, `kube_pod_logs`),
every read gated by `kubeRun` (exposure, `ApprovalCluster` approval unless approval is `allow`,
then the read). No script argument. API chaining: `api_script.go` (`script.prepare`, `script.input`,
`script.transform`, `apiScripts`). DB: `query_script.go` (`run_query` script is a result transform
only: user approves the literal statement that runs). Kept variables: `kept_script.go`
(`keptRunFor`; `kept.get` reads fixed at plan time, `kept.set` in transform only, writes staged
until the response is final). Switch: `scriptsOn` in `jsscript.go` (Studio MCP settings). Card:
`KubeReadBody.vue`, kind def in `frontend/src/studio/approvals/kinds.ts`; API and DB cards show
scripts with `JsScriptBlock.vue` and kept reads with `KeptVariablesPanel.vue`.

Result review: the kube card integrates P717's result review (all five kube tools); no kube-only
review mechanism here.

Chaining rework (P716): kube follows the model the user picks in P716. Scripts and
kept variables; no panel from the old chain steps UI in the kube card. Kube tool descriptions and
`DbmcpInstructions` tell the AI it can store a result in a kept variable and read it in a follow-up
request (see P716 "Agent awareness").

Scope. (1) Kube read tools take the same `script` argument and kept-variable chaining as API and DB,
reusing `jsscript.go`, `kept_script.go` and the same refusal texts, caps and switch; no second
script mechanism. (2) Kube approval card gets P715's redesign (verdict row, connection line, kind
badges, highlighted statement and scripts, collapsed Details, stock buttons with shortcut chips).
Reference `plans/P715.md` on `stream/p715` (v0.3.22), do not re-derive it; its plan covers DB and API
only, so P718 maps each element to the kube card. Starts after P715 lands on `chapter/v0.3.22` and
this branch picks it up, and after P716 and P717.

Deliverable. Plan with screenshots (before: real frontend; proposal: prototype in P715's style),
user approves, then implementation.

Tests. Flow test (`internal/flows/dbmcpflow/kube_test.go` or sibling): kube read with transform,
kept chain into an API or DB call and back, refusal paths (scripts off, `kept.set` in prepare,
secret read). UI spec (`tests/ui/studio/kubernetes-mcp.spec.ts` or sibling): same scenario, card
shows scripts, kept reads and verdict row. Visual baseline for the kube card. Prompt text changes: run the matching row of `docs/DEV_ENVIRONMENT.md`
"Real `claude` tests" (`bun run test:claude:db`); the agent finds and uses kept variables unprompted.

Open questions. Chaining model per P716 (A or B). Prepare stage for kube: DB has none because the statement must be what the user
approves; kube args are structured, so does a prepare that rewrites namespace, selector or name fit,
or transform only like DB? Do scripts run under approval `allow` with no card, as for API?

## P719

User: "For the variables in the api mcp, i want it to support also one time usage that i input there
in the prompt. Like providing a one time token that i don't need persisted longer than that. By
longer i mean the 30 min or whatever is the variables persisted for this usecase."

Today. `api_request` declares `inputs`; the user types values in the approval card; `keepInputs`
(`internal/dbmcp/api_inputs.go`) stores each as a secret kept variable (`SourceUser`), bound to the
request origin, for later requests to reuse. Retention: setting `dbMcp.keptRetentionMinutes`,
default 60 min (`internal/storage/model/settings.go`), range 5 to 240
(`internal/storage/model/dbsettings.go`), store cap `MaxTTL` 240 min, fallback `DefaultTTL` 1 h
(`internal/keptvars/entry.go`). So the user's "30 min" is 60 min by default.

Chaining rework (P716): one-time inputs fit the same model. No
chain UI for them; a one-time value is never kept, so no follow-up request can read it. The `inputs`
text in `api_request` and `DbmcpInstructions` says so (see P716 "Agent awareness").

Scope. A typed value can be marked one-time: used for this one request only, never `Put` in the kept
store, wiped after send, never in op log, approval payload, result or anything given to the AI. A
script that reads a one-time value cannot keep anything derived from it. Plan states how one-time
differs from kept (lifetime, reuse, review, what the agent sees).

Deliverable. Plan with screenshots of the card input (before: real frontend; proposal: prototype on
P715's design), user approves, then implementation.

Tests. Flow test (`internal/flows/dbmcpflow/`, next to `e2e_user_value_test.go`): one-time value
reaches the request, store stays empty, next request asks again, value absent from result and op
log, derived `kept.set` refused. UI spec (next to `tests/ui/studio/approvals-api-input.spec.ts`):
same scenario from the card. If prompt text changes, run the `bun run test:claude:db` row too.

Open questions. Who picks one-time: user per input in the card (toggle), agent per input declaration,
or both? Does "variables in the api mcp" also mean API module `{{name}}` environment variables, or
only `inputs`?

## P720

User: "The api module variables and env handling editing etc is horrible, come with a more intuitive
and practical solution. Also improve the design."

Today. Environments are config files (P473). `EnvironmentsView.vue` lists them in a tab (activate,
open variables, new, duplicate, delete, drag order). `VariableSetView.vue` shows a collection's or
environment's variables read-only (`VariableRow.vue`: "the file is the editor"). Editing happens in
`ApiVariablesDialog.vue`: tabs environment, variables, agent, YAML for an environment, variables and
YAML for a collection; a typed secret goes through a write-only call after the file saves.
`VariablesOverviewPanel.vue` is a read-only popover beside `EnvironmentSelect.vue` on request tabs.
Bulk editing: `BulkVariablesEditor.vue`. Stores: `state/variables.ts`, `state/apiVariablesDialog.ts`;
form mapping: `variablesForm.ts`.

Scope. Audit the flow from code and the running UI: concrete pain points (clicks per common task,
read-only view vs dialog edit, hidden state, collection vs environment scope and shadowing, secret
handling). Propose a better model and layout. Same file format and secret storage unless the plan
argues a change and the user approves it.

Deliverable. Plan with pain-point list, screenshots (before: real frontend; proposal: prototype),
user approves, then implementation. shadcn-vue, Tailwind, Pinia, TanStack Query per CLAUDE.md.

Tests. Flow test for any new or changed backend call (file write, secret set). UI spec per common
task (add, edit, secret, switch environment, override in environment); visual baselines re-recorded
and inspected.

Open questions. Settled in the plan with the user: inline editing in the variables tab vs dialog;
where agent access settings live after the redesign.

## P721

User: "Check the code for code used for debugging or validating stuff for development and remove all
of them. To give an example we still had cheetah grid from an experiment. I think we have benchmarks
probes in there too. And so on."

Known. Cheetah Grid prototype is already gone (history `07-v0.3.15.md`: "Removed P165 Cheetah Grid
prototype"); no `cheetah` outside `docs/history/`. Candidates seen: `perf:probe` script
(`package.json`, `apps/kira-space/playwright.perf.config.ts`); details spike tests
(`apps/kira-space/tests/claude/details_spike*_test.go`, `KIRA_SPIKE`, `KIRA_SPIKE1B_*`,
`KIRA_SPIKE_DUMP`, DEV_ENVIRONMENT "Real `claude` tests"). Details bench (`details_bench*_test.go`,
`test:claude:details`, P714 gate) is a live consumer and also reads `KIRA_SPIKE_DUMP`.

Scope. Audit across Go, frontend, packages, scripts: experiment dependencies (`package.json` files,
`go.mod`, `knip.json` / `lint:dead`), spikes, probe and benchmark code, debug flags and env vars,
dump and trace helpers, dev-only routes, commented-out code, test files no suite runs, fixtures
without readers.

Deliverable. Findings file `plans/P721.md`, committed before any removal: per item file:line,
consumers, verdict keep or remove. Remove everything with no live consumer. Items with a live
consumer (CI job, a gate such as the details bench, docs reference) go to the user for a decision,
never silently removed or kept. Docs referencing a removed item are updated in the same commit.

Tests. No new tests. Verify: build, typecheck, lint, `lint:dead` (knip), `go mod tidy -diff`, Go
tests of touched packages.

Open questions. Per live-consumer item, listed in the findings file for the user.

## P722

User: "Remove the text from the new window button. And also allow tabs in the studio, api and runs
to be dragged out of the window to create a new window. Like it can be done in apps like chrome. And
you can drag them back to attach them to another window and will attach there."

Design-first phase: Opus planner audits, then decides the design; user approves before
implementation.

Audited facts.

- New-window button: find where the text label lives (`menu.go` "New Window ⇧⌘N", `OpenNewWindow` in
  `internal/shell/openwindow.go`, the frontend button). Goal: icon-only with tooltip, keep
  `aria-label`, shortcut still shown per N19.
- Tabs are owned per window in the DB: `internal/appstorage/tabs.go` (`ErrTabOwnedElsewhere`), bridge
  tabs Save/List per `windowKey`, `createTabsStore` in
  `packages/workbench/src/state/createTabsStore.ts`, `studioTabsHooks`. A window hydrates tabs only
  on boot.

Constraints.

- A move is one backend call that re-keys the tab record atomically between windows, plus a live
  notify to the target window (it must add the tab without a reload).
- Non-persistable tabs (terminal, script-run) and in-memory page stores (`dropResources`) do not
  survive a move. Plan decides how runs and terminals re-attach (backend session keeps running) or
  restricts the move to persistable tabs.
- Tab ids stay unique across windows. Order and active selection are settled in both windows.

Open questions.

- Drag out: tab `dragend` outside the window opens a new window at the cursor via
  `OpenNewWindowKeyed`, tab pre-registered for that key.
- Drag into another window: HTML5 DnD does not cross separate native webviews reliably. Candidate is
  backend-mediated drag: frontend reports drag start and end, Go resolves the window under the global
  cursor. Needs cursor position per platform (macOS, Windows, Linux). Prove with a spike before the
  design is approved.
- Tear out of a window's only tab: close the source window or keep it empty?
- Incognito tabs: movable or pinned to their window?

Scope. Studio, api and runs tabs. Same tab strip behavior for all three.

Tests. Split per CLAUDE.md. Flow test: re-key plus open-window backend path (atomic move, ownership
error, new window pre-registered). UI spec: in-page drag handling (drag-out signal, drop target,
order and active tab). Playwright cannot drive native cross-window drag. Manual checklist for the
user on macOS, Windows and Linux: tear off, re-attach to another window, drop on desktop, tear off
the last tab, new-window button icon, tooltip and shortcut.

Spike (`plans/P722-spike.md`): drag-back is feasible via frontend pointer-up coordinates plus Go hit-test of live windows; not feasible on Linux Wayland (window positions report 0,0). The planner starts from that file.

## P723

User: "Remove the flake restarter. I think it s uselesss as you re already monitoring prs for ci
failures".

Today. `.github/workflows/pr-rerun-flaky.yml` (P330, P365) is a `workflow_run` job on `PR`
completion. It runs `gh run rerun --failed` on a failed run, or a cancelled run whose jobs hit their
timeout, up to 3 attempts, only for the PR's open head and newest run. The session already watches PR
CI (CLAUDE.md "PR CI") and root-causes each failure, so the blind rerun adds nothing.

Scope. Delete the workflow file. Update every reference found by grep (docs/history and docs/history-runs
stay, read-only):

- `docs/PACKAGING.md` §7 status line (about line 358): drop `pr-rerun-flaky.yml` from the workflow list.
- `docs/PACKAGING.md` §7 "Chapter PR to release (P330)" step 3 (about lines 389 to 391): drop the
  `pr-rerun-flaky.yml` sentences (rerun while `run_attempt < 3`, cancelled-run timeout rule, P365).
  Renumber steps 4 and 5 and any reference to them.
- Re-grep before editing: `pr-rerun`, `flake rerun`, `run_attempt`, `rerun` over `.github/`, `scripts/`,
  `docs/` (not history), `CLAUDE.md`, `README.md`.

Checked, no change needed. `pr.yml` (only `scripts/retry.sh` install retries and the
cancel-in-progress group), `pr-auto-merge.yml`, `release-on-merge.yml` (its `workflow_run` of `PR`
waits for green, independent), `release.yml`, `release-retention.yml`, `pages.yml` and `cache-warm.yml`
name no rerun workflow. No guard or lint script lists workflows (`scripts/verify-packaging.sh` greps
only `release.yml`). `docs/ARCHITECTURE.md` and `docs/DEV_ENVIRONMENT.md` have no reference.
`scripts/retry.sh` stays: it retries install steps, not CI runs.

Standing rule kept. CLAUDE.md "PR CI" already says to root-cause and fix every failing check, never
skip, disable or quarantine a test to get green; no edit. Doc wording check in the plan: no remaining
text may imply CI reruns failed jobs to green.

Constraint. `workflow_run` workflows execute from the default branch, so the removal takes effect only
once this chapter's PR merges to `main`. Until then the old workflow keeps rerunning failed PR runs,
this chapter's included. Note it in the Result.

Deliverable. One plan `plans/P723.md`, one Sonnet pass. No UI tests, no new tests.

Verify. Final grep finds no `pr-rerun-flaky` or flake-rerun reference outside `docs/history/`;
lint (`bun run lint` as in `docs/DEV_ENVIRONMENT.md`) and the repo's workflow and guard checks
(`scripts/verify-packaging.sh`, pre-commit hook) pass.
