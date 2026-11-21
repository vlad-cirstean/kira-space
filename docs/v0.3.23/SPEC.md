# v0.3.23

Chapter v0.3.23, branch `chapter/v0.3.23`, tag `v0.3.23`. First phase `P716` (highest phase: `P715`,
in open chapter v0.3.22's `SPEC.md`).

Branched from `chapter/v0.3.22` (HEAD `5a64b72c3`), whose PR is not open yet. Rebase onto `main`
before this chapter's PR opens. PR is held until v0.3.22's PR merges (CLAUDE.md "PR CI").

P716, P717 and P718 are design phases: plan proposes the design with screenshots, user approves it
before implementation starts.

| # | Phase | Status |
|---|-------|--------|
| P716 | Kube MCP: JS script chaining, approval prompts like database and API | Todo |
| P717 | API MCP: one-time variable values typed in the approval prompt | Todo |
| P718 | API module variables and environments: UX redesign | Todo |
| P719 | Remove development, debugging and validation leftovers | Todo |

## P716

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

Scope. (1) Kube read tools take the same `script` argument and kept-variable chaining as API and DB,
reusing `jsscript.go`, `kept_script.go` and the same refusal texts, caps and switch; no second
script mechanism. (2) Kube approval card gets P715's redesign (verdict row, connection line, kind
badges, highlighted statement and scripts, collapsed Details, stock buttons with shortcut chips).
Reference `plans/P715.md` on `stream/p715` (v0.3.22), do not re-derive it; its plan covers DB and API
only, so P716 maps each element to the kube card. Starts after P715 lands on `chapter/v0.3.22` and
this branch picks it up.

Deliverable. Plan with screenshots (before: real frontend; proposal: prototype in P715's style),
user approves, then implementation.

Tests. Flow test (`internal/flows/dbmcpflow/kube_test.go` or sibling): kube read with transform,
kept chain into an API or DB call and back, refusal paths (scripts off, `kept.set` in prepare,
secret read). UI spec (`tests/ui/studio/kubernetes-mcp.spec.ts` or sibling): same scenario, card
shows scripts, kept reads and verdict row. Visual baseline for the kube card.

Open questions. Prepare stage for kube: DB has none because the statement must be what the user
approves; kube args are structured, so does a prepare that rewrites namespace, selector or name fit,
or transform only like DB? Do scripts run under approval `allow` with no card, as for API?

## P717

User: "For the variables in the api mcp, i want it to support also one time usage that i input there
in the prompt. Like providing a one time token that i don't need persisted longer than that. By
longer i mean the 30 min or whatever is the variables persisted for this usecase."

Today. `api_request` declares `inputs`; the user types values in the approval card; `keepInputs`
(`internal/dbmcp/api_inputs.go`) stores each as a secret kept variable (`SourceUser`), bound to the
request origin, for later requests to reuse. Retention: setting `dbMcp.keptRetentionMinutes`,
default 60 min (`internal/storage/model/settings.go`), range 5 to 240
(`internal/storage/model/dbsettings.go`), store cap `MaxTTL` 240 min, fallback `DefaultTTL` 1 h
(`internal/keptvars/entry.go`). So the user's "30 min" is 60 min by default.

Scope. A typed value can be marked one-time: used for this one request only, never `Put` in the kept
store, wiped after send, never in op log, approval payload, result or anything given to the AI. A
script that reads a one-time value cannot keep anything derived from it. Plan states how one-time
differs from kept (lifetime, reuse, review, what the agent sees).

Deliverable. Plan with screenshots of the card input (before: real frontend; proposal: prototype on
P715's design), user approves, then implementation.

Tests. Flow test (`internal/flows/dbmcpflow/`, next to `e2e_user_value_test.go`): one-time value
reaches the request, store stays empty, next request asks again, value absent from result and op
log, derived `kept.set` refused. UI spec (next to `tests/ui/studio/approvals-api-input.spec.ts`):
same scenario from the card.

Open questions. Who picks one-time: user per input in the card (toggle), agent per input declaration,
or both? Does "variables in the api mcp" also mean API module `{{name}}` environment variables, or
only `inputs`?

## P718

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

## P719

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

Deliverable. Findings file `plans/P719.md`, committed before any removal: per item file:line,
consumers, verdict keep or remove. Remove everything with no live consumer. Items with a live
consumer (CI job, a gate such as the details bench, docs reference) go to the user for a decision,
never silently removed or kept. Docs referencing a removed item are updated in the same commit.

Tests. No new tests. Verify: build, typecheck, lint, `lint:dead` (knip), `go mod tidy -diff`, Go
tests of touched packages.

Open questions. Per live-consumer item, listed in the findings file for the user.
