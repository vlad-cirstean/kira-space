# v0.3.23

Chapter v0.3.23, branch `chapter/v0.3.23`, tag `v0.3.23`. First phase `P716` (highest phase: `P715`,
in `docs/history/07-v0.3.15.md`, chapter v0.3.22).

Code review at chapter end: 2 rounds (user request).

Branched from `chapter/v0.3.22` (HEAD `5a64b72c3`). v0.3.22's PR merged as #55 (`c7534c9f0`). Rebase
onto `main` before this chapter's PR opens.

P716 to P720 are design phases: plan proposes the design with screenshots, user approves it
before implementation starts.

| # | Phase | Status |
|---|-------|--------|
| P716 Part 1 | Remove the chain steps panel from DB and API prompts | Done |
| P716 Part 2 | Secret and plain kept variables, preview and fingerprint reveal | Done |
| P716 Part 3 | Agent awareness of kept variables, real `claude` test | Done |
| P717 | Result review for every MCP tool that returns data | Done |
| P718 | Kube MCP: JS script chaining, approval prompts like database and API | Done |
| P719 | API MCP: one-time variable values typed in the approval prompt | Todo |
| P720 | API module variables and environments: UX redesign | Todo |
| P721 | Remove development, debugging and validation leftovers | Todo |
| P722 | Window tabs: tear off and re-attach, new-window button without text | Todo |
| P723 | Remove the PR flake rerun workflow | Todo |
| P724 | Fix verify-packaging scanning built binaries; release v0.3.22 re-dispatch | Todo |
| P725 | SQL data view: column search moves into the Columns menu, scroll-to-column button | Done |

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

User chose model A (no chain UI; kept variables; agent makes the follow-up request) as the baseline and may rework it after seeing screenshots at chapter end. Plan: `plans/P716-design.md`. Parts keep the number P716: Part 1 removes the panel, Part 2 adds secret and plain kept variables with preview and reveal, Part 3 tells the agent.

Result, Part 1. Done. `ChainSteps.vue`, its three call sites (`DbQueryBody`, `DbExemptionBody`, `ApiRequestBody`), the `studio.dbMcp.approval.chain.*` strings, the chain asserts in `e2e-kept-chain.spec.ts`, the visual case 'script card with its chain' and its baseline are gone. The kept-variables panel stays. `e2e-kept-chain` and `e2e-db-script-chain` specs assert the old `approval-chain` element no longer renders.

Result, Part 2. Done. Transform scripts keep with `kept.set(name, v)` (plain) or `kept.setSecret(name, v)` (secret); names are plan-time literals, `prepare` scripts cannot keep. A `setSecret` value is masked in the script output and the agent sees only name, type, size, `secret` flag and expiry. API-made secrets bind to the request origin; a use against another origin is refused with `the secret kept as "x" belongs to <origin> and is not sent to <origin>`. DB-kept secrets bind to no origin. The held-result review lists "Will keep" with Secret/Plain badges: plain values preview, secrets show dots and reveal through `KeptVarsService.RevealStaged` behind the local-auth prompt (confirmation fallback). Cards badge the kinds the script may keep, Settings shows the kind per entry. Tests: flow `TestE2EKeptSecret` and `TestDbMcpQueryScriptKeepSecret` (contract `e2e-kept-secret`), UI spec `e2e-kept-secret.spec.ts` (4 tests), visual `approvals-kept-secret.spec.ts` (3 baselines).

Result, Part 3. Done. `DbmcpInstructions`, `run_query` and `api_request` script args and `list_kept_variables` now name `kept.set` (plain, user previews) and `kept.setSecret` (secret, fingerprint reveal), the `secret` flag in the kept summary, that API-made secrets go only to their host, and that a one-time value is never kept so no later call reads it (the sentence P719 deferred). Text stays generic for kube; kube adopts scripts in P718. `budget.tsv` raised for the five touched entries. `TestDbMcpKeptChainThroughClaude` now gives the task only (read ids, fetch bodies by those ids, ids never through the model); the model chose `kept.set` and bound the name in `params` on its own. `bun run test:claude:db -count=1`: 12 pass, none skipped.

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

Result. Done. User decisions: review covers `kube_list_resources`, `kube_describe` and `kube_pod_logs`; metadata tools, `kube_discover` and scripts get none (a script's own result keeps its run approval); one cluster setting covers the three; the Kubernetes switch sits next to the Databases switch in the Studio MCP Server pane; screenshots are reviewed at chapter end.

Backend. `internal/dbmcp/held.go` is the one held-result path: `holdResult`, `heldRefusalText` and one `heldCall` per family; `heldRefusal`, `apiHeldRefusal` and `apiSentSuffix` are gone, DB and API texts byte-identical (`TestHeldRefusalTexts`). Kube: `McpSettings.Review` (`inherit|review|send`, column `mcp_review`, migration 0105), `dbMcp.kubeReviewResponses`, `reviewResolver`, new `kube.response` approval kind and `kubeResponsePayload`, `kube.read` tick, `kube_list_clusters` `reviewResults`, `DbmcpToolKubeReview` suffix on the three tools (`budget.tsv` updated), `api.response` payload gains `masked` secret names. Cluster turned off during the read or the review withholds.

Frontend. `ResultReview.vue` takes `views` with one slot each; `ReviewTable.vue` (P715 grid) and `ReviewLog.vue` are shared; `KubeResponseBody.vue` shows Table, Manifest or Log beside JSON. `ReviewResultOption.vue` has a kube family. Cluster dialog gets "Hold results for review", the Studio MCP Server pane a Kubernetes switch, the audit row a `kube.response` label. Mask notes: DB `Masked columns`, API `Secrets masked`, kube Secret redaction line. `check-ui-primitives.sh` U62 exemption moved to `ReviewTable.vue`.

Deviations. The describe screenshot with a manifest and JSON tab uses a Pod: the kube fake has no ConfigMap, and the JSON tab and `kube-review-manifest` view are the same code. The Server pane keeps two plain switches, not a FieldSet. Unverified: no run on a real cluster (fake only). `llmpromptsflow` `TestList` count raised to 170 (this phase added one prompt; one more was already missing from the constant).

Tests. Flow `kube_review_test.go` (contract `kube-review`): list, describe (Secret and Pod) and logs held and sent byte-exact, Withhold text with one read, over-cap withheld unseen, settings matrix (inherit, review, send, discover never held), tick on a prompt card, cluster turned off in review; `TestKubeMcp` records the dialog's review save. UI `approvals-kube-review.spec.ts` (9), `kubernetes-mcp.spec.ts` review field, `dbmcp-preview.spec.ts` Kubernetes switch, `approvals-alignment.spec.ts` kube.response and kube.read option: all pass; `approvals-api-response`, `dbmcp-preview` and the DB/API UI specs pass after the refactor. Visual `kube-review.spec.ts` (6 new baselines, all opened); `approvals`, `approvals-prompt` and `approvals-kept-secret` baselines unchanged (the `approvals-api-input-once` countdown mask flaked once, passed on rerun). Real `claude`: `bun run test:claude:db -count=1`, 13 pass including new `TestKubeMcpThroughClaudeReviewWithheld` (the agent reports the result withheld, pods read once). Final screenshots: scratchpad `p717-final/`.

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

Result. Done. Decisions (user delegated, reviews screenshots at chapter end): scripts on `kube_list_resources`, `kube_describe`, `kube_pod_logs` only; prepare may change any arg but never `clusterId`, the tool or the verb; kube scripts read plain kept values only (secret or user-typed refused with the shared `keptReadNotAllowed` text) and may keep plain and secret; no `{{kept}}` templates in kube args; scripts run under approval `allow` with no card; no cluster colour line. Backend: `kube_script.go` (prepare, transform, envelope `{kind:"script", output, input:{operation,rowCount,truncated}, kept}`), `kubeRun` reuses `apiScripts`, `keptRun`, `watchKept` and the P717 held path (staged keeps commit after Send, dropped on Withhold, timeout and every other exit). With scripts on, args a prepare can set (kind, name, namespace, pod) leave the tool schema `required`; `kubeRun` enforces them on the final request. Card: `KubeRequestLine` (kubectl line from the final args, dotted underline on args the prepare set), kept panel, both scripts, may-keep badges, Read verdict with scripts fact, Details; scripted review shows Agent gets, raw table, script and Will keep with fingerprint reveal. Fixed `TransformScriptBlock` showing "reads kept variables" for a prepare-only read. Tests on contract `e2e-kube-script`: flow `TestE2EKubeScript`, UI `e2e-kube-script.spec.ts`; three visual baselines in `approvals.spec.ts`; `kubernetes-mcp.spec.ts`, `approvals-alignment.spec.ts` and `approvals-api-script.spec.ts` updated. `TestKubeMcpKeptChainThroughClaude` gives the task only plus "never write the pod's name yourself"; the model kept the pod with a kube transform and read it in the `kube_pod_logs` prepare. `bun run test:claude:db`: 19 results, none skipped. Also refreshed the stale `api-http` contract (`secretSet`).

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

Result. Done. Each typed value on the API card gets a compact Use once / Keep N min choice; the agent's
`reuse` hint seeds it, the user decides. A one-time value lives in an in-memory overlay
(`agentapi.Input.Once`), never `Store.Put`, is masked in the echo, and its bytes are zeroed after the
send and on every exit; Go strings from masking and JSON decoding stay in process memory until GC
(documented in ARCHITECTURE). A script that read one cannot `commit` anything derived
(`errDerivedOnce`); the card shows "Not kept". Only `inputs` are in scope; module `{{name}}` variables
are P720. Card redesign after review: P715 prompt unchanged, then one row per value still needed, one
badge "Sends: <names>", no extra headings, hints or warnings. Flow `e2e-user-value-once` and UI spec
`approvals-api-input-once` share one contract; visual shot `approvals-api-input-once`. Run:
`dbmcpflow`, `apiflow`, `agentapi`, `dbmcp`, `keptvars`, `llmprompts` pass; 3 UI specs (16 tests) pass;
`bun run test:claude:db` passes (`TestDbMcpKeptChainThroughClaude` among 12). One full-package
`dbmcpflow` run under load failed `TestE2EHeldWritesAPI` (`rawBytes` 747 vs 748, then a withhold
timeout); three isolated reruns and a second full run passed.

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

Result. One Variables tab (⌥⌘E) replaces the Environments tab, the variable-set tab and the request
overview: a grid with a row per name and a column per file (collection default, then each
environment), edited in place. Backend: `ApiConfigService.EditVariables` (set, unset, rename, lock,
unlock, describe, share, move), `ApiVariable.secretSet`, import `secretsUnset`, duplicate copies
sealed values. Frontend: `VariablesView`, `VariablesMatrix`, `VariableCell`, `SecretCellInput`
(P516 eye), `EnvironmentColumnMenu`, `RequestVariablesPanel` (quick fix of a missing name), narrow
mode below 760 px. Legacy `variables` and `environments` tabs reopen as the Variables tab (drop at
v0.4.0). Flow: `apiflow.TestApiVariablesMatrix` writes contract `api-variables-matrix`; UI:
`api-variables-matrix.spec.ts` replays it; visual `studio/variables.spec.ts` (grid, narrow, request
popover). Other UI specs moved to the grid; specs of removed UI (typed secret in the dialog, drag
reorder, overview list) deleted or rewritten.

Defaults used for the plan's open questions 1 to 8: as written in `plans/P720.md`.

Deviations. Rename and colour reuse the dialog's Environment section. No separate
`VariableRowMenu.vue`: the row menu lives in `menu.ts`. Narrow mode picker also offers the collection
column. A lock click on a mixed row makes it uniform. With no collection or environment the empty
state offers New environment and New collection; with an environment the grid shows even when empty.
`form.submit` and `form.cancel` are native Enter and Escape, not registry shortcuts. Share with
agents on a secret with no detectable host opens Agent access. Inline fields trim values. The
new-environment input commits on blur when non-blank. Environment reorder is Move left and Move right
in the column menu; drag is gone. Rename shows an info alert with Find usages, not a toast. The
history popover ignores focus-outside so it survives the menu's focus restore. The dialog's bulk
editor is the only dotenv editor.

Unverified. Wails bindings not regenerated (`wails3` missing here, bindings gitignored; typecheck
passes). Real `claude` tests not run: no Claude integration code changed.

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

**Result.** Unconditional part implemented per `plans/P721.md`, four commits.

- C1: dropped 5 dead exports in `studio/api/variablesForm.ts`, 1 in `state/tabDomain.ts`; un-exported
  `apiAgentAccessKey`, `ReviewColumn`, `KubeMcpReview`.
- C2: removed the `*proto*` case in `scripts/hook-scope.sh` (staging
  `studio/workers/parse/protocol.ts` now yields `mobile tests unit web`, no `proto`), three stale
  `.gitignore` entries, dead `packages/db-fixtures` paths in the S3 and SQS seed comments.
- C3: deleted `tests/claude/details_bench_freeze_test.go` plus `benchRoles` (its only consumer),
  `testdata/smoke/`, the mongo and sqlite benchmark files.
- C4: removed DEV_ENVIRONMENT "Comment sweep (P320)"; fixed README test layout and ARCHITECTURE
  Playwright project list (ten projects, perf in its own config).

Verified. `lint:dead` (knip) clean with bindings generated, `lint`, `typecheck`, `lint:claude`,
`go mod tidy -diff` (both modules) clean; `go vet` and `go test` of mongo and sqlite adapters pass;
free bench tests (`TestDetailsBenchFrozenServes`, `ArmRServes`, `ReviseServes`, `Bounds`) ran and
passed, not skipped. Hook ran normally on every commit, no `--no-verify`. `bun run build` not run:
under 6 GB disk free; left to CI.

Open, user decides (default keep, nothing removed): A1a TS perf and heap probes, A1b Go RAM and perf
probes plus `cmd/g1measure` and `AppProcessSet`, A1c `pprofserve` (`KIRA_PPROF`), A1d untested
`__kiraGridTuning` overrides, A2 P643 spike code in `tests/claude`, A3 rename `KIRA_SPIKE_DUMP`
(only if A2 lands). Details in `plans/P721.md`.

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

**Result.** Implemented per `plans/P722.md`.

- Go: atomic `TabsRepo.Move`, `TabsService.Move`/`Inbound`/`Targets`/`Capabilities`, window geometry,
  MRU focus order and drop hit-test, `Registry.Hold`/`Adopt`/`Release`/`Reown` for Runs terminals.
  `Forget` is the package function `bridge.ForgetTabWindow`.
- Frontend: Sortable tear-off with hint, tab menu move items, inbox install, terminal snapshot via
  `@xterm/addon-serialize` 0.14.0, icon-only new-window button (aria-label, `window.new` chip kept).
- Fixed a Sortable quirk: `ignoreNextClick` set at fallback drag start swallowed the next click after
  a release outside the window; `sortableReorder` now dispatches one.
- Contract files `tab-move.json` and `terminal-move.json` use ':' as key suffix separator.
- Tests: flow `TestTabMove`, `TestTerminalMove` (real PTY), coalescer unit test, all `-race`; UI
  `tab-window-move.spec.ts` (14). UI specs use `data` tabs (a restored http tab is dirty) and the
  files window for the non-movable case. No visual baselines added.
- Unverified: macOS, Windows, real Linux X11 cross-window drag, mixed-DPI. Manual checklist in plan
  section 11. Playwright cannot drive a native cross-window drag.

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

Result. Deleted `.github/workflows/pr-rerun-flaky.yml`. `docs/PACKAGING.md` §7: dropped it from the
status list; "Chapter PR to release (P330)" lost the rerun sentences, `pr.yml` folded into the merge
step, steps renumbered to 4. `scripts/retry.sh` kept. Grep finds no leftover reference outside
`docs/history/` and `docs/v0.3.23/`. `workflow_run` workflows run from the default branch, so the
removal takes effect only once this chapter's PR merges to `main`; until then the old workflow still
reruns failed PR runs. Pushing a workflow change needs a token with the `workflow` scope.

## P724

Cause. Release run 44942506459 (tag `v0.3.22`, commit `c7534c9f0`) failed in `build (macOS)`, step
"Verify packaging", so `publish` was skipped and v0.3.22 has no release. `scripts/verify-packaging.sh`
S10 runs `grep -rnE 'browser_download_url|releases/(latest/)?download' apps/ packages/ internal/` and
requires exactly one hit, `internal/appupdate/checker.go`'s `indexURL`. After `bun run package` the
Go binary embeds that constant, so grep also reports `Binary file apps/kira-space/bin/Kira Space
matches` and the same for `Kira Space.app/Contents/MacOS/Kira Space`: 3 hits, `verify-packaging:
FAILED`. Local and PR CI runs have no built binary, so the check passed there.

v0.3.21 (run 44840061445, commit `03d33b5ca`). Not the same failure. Its "Verify packaging" passed:
S10 then required zero hits (`releases/download` only) and the checker used the API, so no source or
binary carried the string. `build` and `publish` succeeded and v0.3.21 is published. The run reads
`failure` only because `install smoke (macOS)`, which runs after publish, got HTTP 403 from the GitHub
API on all three install attempts (rate limit; P707 moved `install.sh` to the release index in
v0.3.22). P707 (v0.3.22) added `indexURL` and the exactly-one rule, which no pre-release run could
exercise against a binary.

Fix. Scan source only. One helper in `verify-packaging.sh` greps text files (`grep -I`) with build and
vendor dirs excluded (`bin`, `dist`, `dist-mobile`, `node_modules`, `bindings`); S10 and S2 (same
latent shape: scans `apps/` incl. `bin/` and `frontend/dist`) use it. Invariant stays: exactly one
release-download reference in source, `checker.go`'s `indexURL`. New S18 self-test runs the same scan
and S10 verdict against a tiny temp fixture holding fake binaries and a `dist` bundle that carry the
URL, so a missing build can no longer hide the bug. `checks (macOS)` in `pr.yml` gains
`bun run verify:packaging`, so S18 runs under macOS grep in PR CI before any release.

Verify. No macOS build: `bun run verify:packaging` passes on Linux with S18; a simulated release tree
(dummy `bin/Kira Space` and `.app` binaries carrying the URL) reproduces the CI failure with the old
script and passes with the new one; removing `-I` or the `bin` exclusion makes S18 fail. macOS grep is
proven by `checks (macOS)` on this chapter's PR.

Post-merge. Re-dispatching `release.yml -f tag=v0.3.22` cannot succeed: `build` checks out the tag's
commit `c7534c9f0`, which carries the old S10. Moving the tag to a fixed `main` commit would make
v0.3.22 ship v0.3.23's code. v0.3.22's changes ship in v0.3.23, whose release builds with the fix.
`docs/PACKAGING.md` §7 "Recovery" covers this case ("Real failure: delete the tag ...; the fix ships
as the next chapter"). The orchestrating session reports it; the user decides whether to delete the
`v0.3.22` tag (`git push --delete origin v0.3.22`) or leave it unreleased. No re-dispatch unless the
user asks.

Deliverable. One plan `plans/P724.md`, one Sonnet pass. No UI tests.

Result. Commits on `stream/p724`: `cdfb3275a` fix (helper `source_grep`, `s10_verdict`, S2 and S10 on
them, S18, header comment), `41b99f034` ci (`bun run verify:packaging` in `checks (macOS)`, comment),
docs commit (`PACKAGING.md` S2/S10/S18 bullets, §5 list, CI job row). Hooks passed unbypassed.

Verify. `bun run verify:packaging` on Linux: `all checks passed`, S18 silent. Simulated release tree
(text dummies carrying the URL in `bin/Kira Space` and the `.app` binary): old script prints both S10
failures and `FAILED`; new script has no S10 failure (only my incomplete dummy `.app` tripped A7
`Assets.car missing`). Mutation, local only, restored: dropping `--exclude-dir=bin` makes S10 fail on
the dummies; dropping `--exclude-dir=dist` makes `S10 scan self-test — build output counted:
...frontend/dist/assets/index.js` fail. Not reproducible on Linux: dropping `-I`. GNU grep 3.11 sends
`binary file matches` to stderr (the script discards it), so the 3-hit failure appears only with
macOS grep; binary dummies passed with the old script on Linux and removing `-I` left S18 green. The
`-I` guard is proven only by `checks (macOS)` on this chapter's PR and v0.3.23's release. Dummies
and temporary `bun install`/bindings copy removed.

## P725

User: "The column search thing in sql data view have it as part of the column projection where i
have a searchbox too and an eye icon to scroll to the specific column".

Today. P658 (v0.3.16) put a "Column name" field at the right end of the data view's filter toolbar
(`FilterToolbar.vue`, testid `column-filter-input`, chord `data.filterColumns` ⇧⌘F). Typing narrows
the grid's rendered columns (`columnFilter.ts` `visibleColumnOrder`, runtime `columnFilter`, applied
by `SlickGridHost.vue` after a debounce). The Columns menu (`ColumnsMenu.vue`, toolbar button
`toolbar-columns`) lists every column with a drag handle and a visibility checkbox; it has no search
box and no way to reach a column in the grid.

Reading chosen. Move column search into the Columns menu and drop the grid narrowing:

- Remove the toolbar field, its status text, runtime `columnFilter`, `visibleColumnOrder` and the
  host's filter apply path. The grid always renders the projection in stored order.
- Columns menu gets a search box on top. It filters the menu's rows only (same matching as P658:
  case-insensitive substring, whitespace or comma terms OR-ed). No match shows "No match".
- Each row gets a button that scrolls the grid to that column (SlickGrid `scrollColumnIntoView`). It
  keeps the menu open and leaves the checkbox alone. Disabled with a tooltip for a column the grid
  does not render (unticked in the committed projection).
- ⇧⌘F stays: it opens the Columns menu with focus in the search box. Its chip moves to the Columns
  button tooltip. Registry id `data.filterColumns` kept.
- Drag reorder keeps working while the list is filtered (moves by column name).

Icon. The user asked for an eye. In this toolbar `eye-closed` already means mask preview, and across
the app eye and eye-closed mean show or hide. Next to a visibility checkbox an eye reads as a second
visibility toggle. Plan uses codicon `target` (locate) instead. User may override at chapter end: a
one-word swap.

Scope. Frontend only. No backend half: search and scroll never call the bridge; the UI spec asserts
no extra `data:read` and an unchanged projection. Search text is menu-local, cleared on close.

Deliverable. One plan `plans/P725.md`, one Sonnet pass. UI spec rewritten
(`data-view-column-search.spec.ts`), shortcut-hint and module-state specs updated, unit spec for the
removed filter deleted, `data-grid.png` baseline regenerated in PR CI. Throwaway screenshots for the
user at chapter end.

Result. Commits on `stream/p725`: refactor! (toolbar field, `columnFilter.ts`, runtime `columnFilter`
and host narrowing removed; Columns menu search and per-row scroll button; chord and chip moved to
the Columns button), test (spec rewrite and updates), fix (button alignment). Icon: user asked for an
eye, so the reveal button uses codicon `eye` (overrides the plan's `target`). It sits right-aligned
and dim, apart from the left checkbox, with tooltip "Scroll to column" and aria-label "Scroll to
{column}". Checkboxes got an explicit `aria-label` so the button does not leak into their name.
Mask preview keeps `eye-closed`, untouched. Disabled with "Not shown in the grid" for an unrendered
column. `SlickGridHost` exposes `revealColumn` (`scrollColumnIntoView`). Search uses `useFocus`
(VueUse), `useSortableReorder` gets `ids` of the filtered rows and a `filter` for the button.
Specs run once, all passed: `data-view-column-search` (renamed from `data-view-column-filter`, new
contract), `shortcut-hints`, `module-state`, `studio/data-view`, `studio/tooltips` (41 tests; one
cold-start flake of "flips below the trigger" passed on rerun and four more runs). Typecheck, lint
rules and hooks passed. `data-grid.png` (visual) not regenerated locally: the toolbar lost the field;
regenerate in PR CI. Screenshots (dark default theme): session scratchpad `p725/`. Pre-existing bug
fixed: a drag reorder in the Columns menu committed `columnOrder` but the grid kept the old header
order until its next page load. Confirmed on chapter head `d8a026f11` (before P725) with a temp
spec: the swap showed in the menu, not in the headers. Fix: `SlickGridHost` watches
`tab.state.columnOrder`, stages an open edit, clears the selection and calls `applyPageAndOrder`.
The spec asserts the grid header order after the drag.
