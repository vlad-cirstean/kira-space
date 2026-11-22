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
| P719 | API MCP: one-time variable values typed in the approval prompt | Done |
| P720 | API module variables and environments: UX redesign | Done |
| P721 | Remove development, debugging and validation leftovers | Done |
| P722 | Window tabs: tear off and re-attach, new-window button without text | Done |
| P723 | Remove the PR flake rerun workflow | Done |
| P724 | Fix verify-packaging scanning built binaries; release v0.3.22 re-dispatch | Done |
| P725 | SQL data view: column search moves into the Columns menu, scroll-to-column button | Done |
| P726 | Approval cards: aligned value rows, request, values and scripts in separate boxes | Done |
| P727 | Window tabs: select several tabs, close them or move them to another window | Done |
| P728 | Approval prompts and scripts rework | Done |
| P729 | Removals, Variables tab and kube result views | Done |
| P730 | Switch, checkbox and radio on-state style | Done |
| P731 | Auto-extracted task links from notes and handovers | Done |
| P732 | Script variables `plain` and `secret`, formatted scripts, approval prompt polish | Todo |
| P733 | Kira Studio MCP tools: efficient descriptions, fewer tools, leaner results | Todo |

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

User decisions on the gated items:

- Keep A1a (TS perf and heap probes) and A1b (Go probes, `cmd/g1measure`): kept for future
  measuring. ARCHITECTURE Known open item about WebKit memory stays unchanged.
- Remove A1c (`KIRA_PPROF`, `internal/pprofserve`), A1d (five `__kiraGridTuning` knobs), A2 (P643
  spike entry points, spike-only helpers, files arm, accuracy tests, testdata) and A3 (rename
  `KIRA_SPIKE_DUMP` to `KIRA_BENCH_DUMP`). Moved to P729. Details in `plans/P721.md`.

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

## P726

User, on the API card with one-time values (P719 shots): "that's a variable length string before
the input making each input miss aligned. Also add a small separator between what is about the
request and the variables section so that's easier on the eye. And i want the same separation with
pre after scripts too. I see it as the current box that's inside the dialog but like 3 or 4 of them".

Today. `ApprovalItemCard.vue` (packages/workbench) draws one bordered box (`approval-body`) around
the kind's `content`. API, DB, kube read and the three result reviews stack everything inside it:
kept variables, request line, body, headers and variables, prepare script, transform script, typed
values. `UserInputsPanel.vue` lays each value row out as a wrapping flex line: label, masked field,
Use once / Keep choice. Label width varies, so the fields start at different x.

Reading chosen (no questions to the user):

- Value rows share one grid: a label column as wide as the longest label (capped, wraps past the
  cap), a field column, a choice column. Every field and every choice group lines up for 1 to 4
  rows. An opened "Enter a new value" row puts its field in the same column.
- The card body becomes several stacked boxes, each the existing box style, 8px apart: kept
  variables, request (line, body, headers, variables), values to type, prepare script, transform
  script. Result reviews: result views (with the masking note), script, Will keep. A box shows only
  when it has content. Existing section headings stay the box titles (Prepare script, Transform
  script, Result script, Kept variables this request uses, Will keep). The request and values boxes
  stay untitled (P719: no extra headings). Verdict row, warnings, Details and footer stay outside.
- One shared `ApprovalBox` component; the card's own box becomes it. A kind opts in with a registry
  flag; script runs, proof review, Space and phone kinds keep the single box unchanged.

Scope. Frontend layout only: no payload, IPC or copy change, no new catalog keys, no backend half.

Deliverable. One plan `plans/P726.md`, one Sonnet pass. New UI spec for box structure and value-row
alignment, `approvals-alignment.spec.ts` updated, affected approval visual baselines regenerated.
Throwaway screenshots for the user at chapter end.

Result. Done. `ApprovalBox.vue` (workbench) frames one section; the card uses it for the whole body unless the kind sets registry flag `sections` (`db.query`, `db.exemption`, `db.response`, `kube.read`, `kube.response`, `api.request`, `api.response`). Sectioned bodies draw their own boxes, 8px apart, a box only when it has content: API request kept, request (line, body, headers, variables), values, prepare, transform; DB statement (with Parameters) between kept and Result script; kube read kept, request, prepare, transform; reviews (`ResultReview` takes `testidPrefix`) result (tabs plus mask/footnote notes), script, Will keep, with loading and error state in one `-box-state` box. Existing headings are the box titles; no copy keys added. `script.run`, `proof.review`, Space and phone kinds keep the single `approval-body` box. `UserInputsPanel` rows share one 3-column grid (label up to `max-w-48`, field, choice) through subgrid rows; an opened stored row puts its field and choice in the same columns. Choice groups line up on the left; their right edges follow the label width ("Keep 5 min" vs "Keep 60 min"). `scripts/check-ui-primitives.sh` U36 exemption moved from the card to `ApprovalBox.vue`. Tests: new `studio/approvals-sections.spec.ts` (11: value-row alignment for 1 to 4 rows, opened stored row with computed `display: grid`, box order and 8px gaps for API, DB, kube read, kube review, unopted kinds), `approvals-alignment.spec.ts` (A2 and A8 measure per box; shared geometry in `support/approvalGeometry.ts`), and a stale assertion in `approvals-api-kept-script.spec.ts` fixed (the reads note shows only for a transform read; the contract item reads in prepare). Ran 161 approval UI specs (`approvals*`, `studio/approvals-*`, `dbmcp-approval*`, `kept-variables*`, `e2e-kept-*`, `e2e-user-value`, `e2e-kube-script`, `e2e-api-review-chain`, `ade-mobile-approval`): all pass after the fix. `fc-match sans-serif` printed Inter and unchanged baselines still matched, so nine baselines were regenerated locally: `approvals-api-input-once`, `approvals-kube-read-script`, `approvals-kube-read-list`, `approvals-kube-review-script`, `approvals-kept-login`, `approvals-kept-review`, `approvals-kept-review-revealed`, `approvals-kube-read-review-option`, `approvals-kube-review-list`; the four visual specs pass on rerun. Mobile `tests/mobile/approvals.spec.ts` not run (phone kinds untouched, not built for this pass).

## P727

User, after P722: "For 722 it looks great. But i want the posibility to select multiple tabs and
move them or close them. Make this possible as a followup".

Follow-up to P722 (tab tear-off, `plans/P722.md`). Plan: `plans/P727.md`. One Sonnet pass.

Scope.

- Multi-select in every `TabStrip` whose host can close or move tabs: main workbench strip (Studio
  data and API, Runs, Git, every module; ade review and opened-files windows share its host) and
  the Kubernetes resource strip. Proof window strip (no close, no move) stays single.
- Gestures, browser style: Cmd-click (macOS) or Ctrl-click (others) toggles, Shift-click selects
  a range from the anchor, plain click clears and activates, Esc in the strip clears. Pinned tabs
  never join. Selected chips use the neutral selection token (N18).
- Selected set, from the tab menu: "Close N tabs" (dirty question once for the set, as Close
  others), "Move N tabs to new window", "Move N tabs to window ▸ Window N". ⌘W closes the set when
  the active tab is in it. No new chord.
- Dragging a selected tab drags the set: out of the window moves all movable selected tabs; inside
  the strip reorders them as one block. Sortable has no MultiDrag here (vue-draggable-plus bundles
  Sortable without it and exports no class to mount it on); Sortable drags the grabbed chip, the
  strip applies the set on drop.
- Backend: `TabsService.Move` takes a list. One transaction re-keys every row; any refusal moves
  nothing. Live Runs terminals: hold all, one move, re-own all, target adopts each; a failure
  releases every hold. Target appends in source order; the source's active tab, if moved, else the
  first, becomes active.

Tests. Flow `TestTabMoveMany` (real PTY, two live terminals), P722 flow tests on the list shape, UI
spec `tab-multi-select.spec.ts`, P722 UI spec on the list shape, one unit spec for block reorder.
Throwaway screenshots for the user at chapter end; manual checklist for macOS, Windows, Linux X11.

Result. Done.

- Backend: `TabsService.Move` takes a list (100 tabs, 16 MiB limits); `TabsRepo.Move` runs one
  transaction, any refusal moves nothing. Several live terminals: hold all, one move, re-own all,
  target adopts each; failure releases every hold.
- Frontend: `tabSelection` Pinia store keyed by selection scope; Primary-modifier toggle, Shift
  range, plain click and Esc clear; grey selection token; multi menu; ⌘W closes the set; one shared
  dirty question, per-tab prompts for API tabs; drag applies the set on drop via pure `moveBlock`.
  Kubernetes strip included.
- Also fixed: 3 unit specs (repo-tab slots, tabs-save-serialized x2) failing on chapter HEAD. Cause:
  spread copy of `control` in `state/tabs.ts`; now prototype delegation.
- Tests run once at end: flow `TestTabMoveMany` and P722 flow tests with `-race`, contract
  `tab-move-many.json`; 63 UI tests across `tab-multi-select`, `tab-window-move` and related P722
  specs; `move-block.spec.ts`; unit suite 1426 pass, 0 fail; four typechecks; hooks pass.
- Screenshots (throwaway, not committed): 6 PNGs in the session scratchpad `p727-final/`. Confirm
  shot shows the API per-tab dialog.
- Not covered: pinned-repo-tab UI case (plan 4.4 item 1).
- Manual platform checklist (plan section 10, 8 items): unverified here. macOS, Windows, Linux X11
  and Wayland checks remain for the user.

## P728

User, at the chapter-end screenshot review: approval prompts and scripts need one coherent rework.
All items below user-approved.

Scope.

- Script API rename for API, SQL and kube scripts, one namespace: `kira.storeValue(name, value)`
  (plain), `kira.storeSecret(name, value)` (secret), `kira.getValue(name)` (replaces `kept.get`; in
  kube scripts plain values only). Replace `kept.set`, `kept.setSecret`, `kept.get` everywhere:
  engine bindings, tool descriptions and llmprompts, script help and templates, approval card text,
  docs, fixtures and contracts, tests. `{{kept.x}}` template syntax stays unchanged. After the
  rename, run the real `claude` test row from DEV_ENVIRONMENT "Real `claude` tests"
  (`bun run test:claude:db`); probe auth first.
- Restyle the Details section (`packages/workbench/src/approvals/ApprovalDetails.vue` and per-kind
  Details components: Db*, Api*, Kube*, ScriptRun, ProofReview, DbExemption, TaskCreate) once in
  shared components: formatting and highlighting like the boxes above it (method, URL, query, SQL,
  manifest highlighted via `ApprovalCode`; values split cleanly). Same restyle for the prompt header
  line (connection, cluster, namespace for DB, API, kube): easier to read, values visibly separated,
  namespace as a visible chip. Applies to every approval prompt and to the review dialogs. Approvals
  get their own `KeyValueList` density and style variant; non-approval screens unchanged.
- Kept-variables box (`KeptVariablesPanel.vue`, `KeptVariableRow`): compact rows like the "Will
  keep" box. Add expiry (`kept 1m ago · expires in 59m`) and "used in" on one small grey line,
  shortened "Kept by". Add a small x on each kept value (card and review boxes) to delete it
  (`KeptVarsService.Clear`).
- Chip: remove "Runs N scripts". Show a chip only when variables are involved (e.g. "Uses: pod",
  "Keeps: token").
- Prepare script result: show as a named list (`pod = web-1`, `tail = 2`) in the same compact row
  style, above the final command. No invisible variables: every value a prepare script produces
  appears interpolated and highlighted in the final command text. Add a test that fails when a
  prepared argument is missing from the command text. Only `kira.store*` writes are invisible by
  design.
- One-time values (P719): remove the "Use once / Keep N min" toggle; every typed value is kept for
  the retention time. Retention stays configurable (`dbMcp.keptRetentionMinutes`, default 60, range
  5 to 240). Add a small grey line under the inputs, "Kept for N min" (follows the setting), linking
  to Settings > Studio MCP > Variables (same target as "Manage kept variables").
- Review dialogs (API, SQL, kube): rename the "Agent gets" and "Response" tabs to "After" and
  "Before" (After = what the agent gets, Before = raw response). Format toggles (Table, Manifest,
  Logs, JSON) stay. Add a collapsible "Request" section (shadcn-vue `Collapsible`, collapsed by
  default) showing the request box the approval prompt showed (SQL: query; kube: command or
  manifest), reusing P728's restyled request components, above the result.
- Refresh affected visual baselines. Flow and UI spec pairs per CLAUDE.md Tests where behaviour
  changes.

Streams: P728 (Stream A) and P729 (Stream B) run in parallel, no ordering dependency; shared files (i18n catalogs, docs) are listed in each plan's ownership table.

Deliverable. Plan `plans/P728.md`, Opus-authored, then Sonnet implementation.

Result. Done. Script kept API renamed to `kira.storeValue`, `storeSecret`, `getValue`; one-time typed values dropped, every typed value is kept. Approval cards: compact density, shared detail parts, kept rows with expiry, used in and delete (x), staged "Will keep" x (`KeptVarsService.DropStaged`, agent sees `keptNotSaved` "the user chose not to keep it"), variable chips "Uses: names" and "Keeps: names" replace the scripts chip, prepared values listed and marked (`PreparedList`, `data-prepared`), no invisible prepared value. Reviews: tabs After and Before, collapsed first Request box (`ReviewRequest`) fed by new `request`, `params`, `placeholders`, `kept`, `tool`, `changed` payload fields. Flow + UI pairs: `e2e-user-value`, `e2e-kept-delete`, `e2e-prepared-visible`, `e2e-review-request`; contracts regenerated. Visual baselines regenerated locally (fc-match = Inter): 13 changed, `approvals-api-input` added, once-input one removed.
- Runs: affected UI specs (approvals, dbmcp, kube, e2e-, connection-mcp, http-request) 320 run, all pass after fixing the `approvals-sections` box order; visual approvals and kube-review 15 pass; Go `./apps/kira-space/internal/... ./internal/...` pass after fixes (llmprompts count 179, `e2e-mask-echo` kept-id replace); `adeflow` and `internal/fswatch` failed once under load, pass alone; `bun run typecheck:web` and hooks pass; real claude `bun run test:claude:db -count=1` passes (auth probe ok, 0.07 USD).
- Deviations: `e2e-kept-delete` waiting-card case uses an API card; kept-row meta reuses existing keys; maxRows line reuses `kube.rows.status`; `ApiRequestView`, `DbRequestView`, `KubeRequestView`, `ReviewRequest` extracted. `go clean -cache` run once (disk full).
- Not verified: full UI suites beyond the listed specs (PR CI).

## P729

Streams: P728 (Stream A) and P729 (Stream B) run in parallel, no ordering dependency; shared files (i18n catalogs, docs) are listed in each plan's ownership table.

Scope.

- Kube list result: columns size to content (capped), resizable, full dialog width; same fix for the
  expand-row cell. Pod logs: terminal-style monospace view, no line gaps, optional line numbers,
  dimmed timestamps, error and warn coloring, wrap toggle (default chosen by the planner: unwrapped
  with horizontal scroll, or wrapped).
- Variables tab (P720): min column width per environment column; whole grid scrolls horizontally
  with the Name column pinned. Verify with a 20-environment fixture screenshot. Fix footer plural
  ("1 value missing").
- Removals per P721 decisions: A1c, A1d, A2, A3 exactly as in `plans/P721.md`. A2 detail section:
  keep what the details bench needs. Run the matching real `claude` bench row after removal. Update
  DEV_ENVIRONMENT runbook, ARCHITECTURE and PERF.md references, README lines.
- Refresh affected visual baselines. Flow and UI spec pairs where behaviour changes.

Deliverable. Plan `plans/P729.md`, Opus-authored, then Sonnet implementation.

Result. Done. Removals: `internal/pprofserve` and its `main.go` wiring (A1c); five grid tuning knobs in `kiraSlickGrid.ts`, `SlickGridHost.vue`, `columns.ts`, `debugHooks.ts` and `global.d.ts` (A1d); P643 spike code in `apps/kira-space/tests/claude` (spike, spike1b, metrics, report, stubs, frozen, state-accuracy, testdata) keeping only what the details bench needs (A2); `KIRA_SPIKE_DUMP` renamed `KIRA_BENCH_DUMP` (A3). DEV_ENVIRONMENT, PERF.md refs updated. Variables tab: `VariablesMatrix.vue` uses `table-fixed` with colgroup, env columns min 11rem, whole grid scrolls horizontally, Name column sticky with divider via `after:` classes; footer plurals through vue-i18n plural keys. Kube review: `ReviewTable.vue` sizes columns to content (canvas measure, `measureTextColumns` in `columns.ts`, cap 360px), resizable with `ResizeHandle` (double-click resets), fills dialog width via a trailing filler column; expand-row cell follows. Pod logs: `ReviewLog.vue` with `LogLines` (new opt-in `lineNumbers`, `levels`, `lineHeight` props, `logLevelOf`), monospace, no line gaps, dimmed timestamps, error and warn colors, Pinia `reviewLogStore` with toggles wrap (default on), timestamps (on), line numbers (off). Runs: golangci-lint v2.15.0 0 issues (root and nested module); `go vet`, `go build`, `go mod tidy -diff` clean in both modules; `bun run typecheck`, `bun run lint`, `bun run lint:dead` clean; free bench checks pass; real bench row F5,F6 12/12 runs pass (0.94 USD); targeted UI specs `api-variables-matrix`, `approvals-kube-review`, `dbmcp-preview` (13), approvals-prompt pass; full `visual` project 69 passed. Baselines regenerated: variables, kube-review, approvals-prompt shots plus new `variables-matrix-20-envs`. Fixed `dbmcp-preview.spec.ts` row assert (filler cell). Unverified: flow-test halves not added for the layout-only changes (no backend behavior changed); mobile specs and the rest of the `ui` project not run locally (PR CI).

## P730

Runs after P728 and P729 land (they regenerate overlapping baselines); plan against the merged tree.

Scope (user-approved from visual mock "M2").

- Planner first lists every use of the `--kira-on` family with file:line, before changing anything.
- Shared on-state look for switch, checkbox and radio (`packages/theme`: `--kira-on` and
  `--kira-on-fg` tokens or new tokens, `Switch.vue`, `Checkbox.vue`, `RadioGroupItem.vue`):
  - on: dark fill `#33333a`, light-grey border `#c4c4cc`, bright mark, knob and dot `#f4f4f5`;
  - off: field fill, dark edge `#3e3e45`, dim knob `#6b6b73`.
- Planner checks these values against existing tokens and contrast, and that a checked control with
  keyboard focus (`--kira-focus` `#a1a1aa`) stays distinguishable.
- Progress bar fill (`Progress.vue`, uses `bg-on` today) gets softer light grey `#b4b4bc`, not the
  dark fill (unreadable on its track).
- Not touched: segmented choices ("Ask me first / Read straight away" style); blue accent
  (`--kira-accent`: primary and dialog-primary buttons, links); grey focus and selection tokens.
- Regenerate every affected visual baseline once, locally, full `visual` project.

Deliverable. Plan `plans/P730.md`, Opus-authored, then Sonnet implementation.

Result. Done. Six dedicated tokens replace `--kira-on`/`--kira-on-fg` (deleted): `--kira-toggle-on` #33333a, `--kira-toggle-on-edge` #c4c4cc, `--kira-toggle-mark` #f4f4f5, `--kira-toggle-off-edge` #3e3e45, `--kira-toggle-knob` #6b6b73, `--kira-progress-fill` #b4b4bc (`tokens.css`, `tailwind-core.css`); `Switch.vue`, `Checkbox.vue`, `RadioGroupItem.vue`, `Progress.vue` use them, still keyed on `aria-checked`. Off-edge #3e3e45 is 1.4:1 on the field fill (user choice, below the old 3:1 bar); off controls read by the field fill plus the knob. Focus fix: the global 1px ring at offset -1px sits on the 1px edge, and #a1a1aa vs on edge #c4c4cc is 1.48:1, so a checked focused control was near-identical; the three roots add `focus-visible:outline-offset-2`, ring now 2px outside the edge. N18 guard (`scripts/check-conventions.ts`) lists the six tokens as neutral and keys on `bg-toggle-on`/`border-toggle-on-edge`; fixtures updated. `selection-colour.spec.ts` asserts the new colours, adds the off-knob assert and a focused-switch outline test (offset 2px, #a1a1aa); fixed stale env radio testid (now `matrix-activate-*`). `ARCHITECTURE.md` on-state sentence rewritten. Commits: tokens and guard together (`5fe79814f`; the pre-commit hook fails when tokens and guard disagree, so plan commits 1 and 2 could not land apart), N18 fixtures, UI spec, docs, baselines. Checks: `bun test scripts/check-conventions.spec.ts` 44 pass; `selection-colour.spec.ts` 8 pass; hooks green on every commit; full `visual` project 75 pass after update (one unrelated flake in one of three reruns). Baselines regenerated (27): workflow-graph x5, git-repo-settings-dialog, settings-advanced, settings-appearance, approvals-kept-login, approvals-review-resized-row, approvals-api-input, runs-restore-dialog, connection-privacy-review and -tab, docker-restore-dialog, approvals-kube-read-review-option, -review-list and -review-logs, kube-mcp-dialog-review, settings-mcp-review-results, variables-matrix and -narrow. Also stale from earlier phases, not toggles (titlebar "New window" label, toolbar): new-connection-dialog, console, data-grid, schema-dialog, workbench-shell. No baseline shows Progress. Unverified: Progress fill colour (no spec renders it).

## P731

Stream C: independent of P728, P729, P730; runs parallel in its own worktree. Shared files (i18n
catalogs, SPEC.md, docs) are listed in the plan's ownership table.

Scope (user request).

- Links found automatically in a task's notes and in its handovers are extracted and attached to the
  task.
- Link classes: a link the user provides manually (at task creation or added later) or the AI adds
  is in the main category (related to the task, as today). Every automatically found link cannot be
  classified, so it is added in the "related" category only, never promoted automatically.
- Planner decides, after reading the current link model (`useTaskLinks.ts`, Go link/ADE task code,
  `repos/adehandover.go`, ADE notes):
  - where extraction runs (backend preferred);
  - URL detection rules and recognised URL kinds;
  - de-duplication against existing links (a URL already present as main stays main);
  - behaviour on note/handover edit or delete (remove stale auto links vs keep);
  - how auto links are marked in the UI;
  - limits;
  - security: only extract, never fetch.
- Flow test plus UI spec pair per CLAUDE.md Tests.

Deliverable. Plan `plans/P731.md`, Opus-authored, then Sonnet implementation.

Result. Done. Found links derive on read in `notesSource` (`ade/foundlinks.go`, source `notes`), never stored, so an edit or delete of a note or handover takes its links along. Extractor `ade/linkextract.go` (unit-tested): skips code, keeps http(s) only, drops userinfo and loopback. `localLinkKind` is shared with `classifyLink`. A third merge pass keeps found links from replacing or demoting any main link. Promotion is an explicit add: user card action "Add to links" (`AddLink`) or the agent listing the URL in `links.yaml`; removing that row brings the link back. Only the user dismisses (`DismissFoundLink`, table `ade_task_found_link_dismissals`, migration 0106 stayed free). Limits 200 rows, 50 per row, 100 links, 5 sources. UI: Related line after Misc, dashed pill with note icon whose tooltip names the sources, card with "Found in" list and promote and dismiss buttons (dismiss edit mode only, none in view mode). README line tells agents how to promote. Tests on contract `ade-found-links`: flow `TestFoundLinks`, UI `ade-v2-found-links.spec.ts`, unit `linkextract_test.go` and `ade-v2-link-groups.spec.ts`. New visual baseline `ade-links-related-visual-linux.png` (existing ones unchanged). Runs: `go test ./apps/kira-space/internal/{ade,storage,taskfiles,bridge}/...` and `internal/agentmcp` pass; `go test -p 1 ./apps/kira-space/internal/flows/adeflow/` pass (407 s); UI `ade-v2-found-links`, `ade-v2-details-links`, `ade-v2-links-strip`, `ade-v2-artifacts` and visual `ade-links` pass; real `claude` ADE row (18 results) and `TestDetailsBenchFrozenServes` pass, none skipped (auth probe returned `ok`). Unverified: `-race` (the disk filled while building race objects, so the Go runs are without it), golangci-lint (not loadable here), PR CI.

## P732

One phase, one sequential implementer (touches the script engine, approval cards and the approvals dialog).

Scope (user decisions, final).

- Script API, hard cut with no alias, for API, SQL and kube scripts:
  - globals `plain` and `secret` replace `kira.storeValue`, `kira.storeSecret` and `kira.getValue`.
    `plain.token = x` stores a plain value, `secret.apiKey = x` a secret. Reads: `plain.token`; secrets are never readable.
  - No Proxy: the object is injected with the current plain values; after the run Kira diffs it (new, changed, deleted keys). `secret` starts empty; every key present afterwards is a write.
  - A name used as both kinds is refused after the run. Invalid names or non-text values are refused with a clear message. A script that declares its own local `plain` or `secret` is refused clearly.
  - Templates `{{plain.x}}` and `{{secret.x}}` replace `{{kept.x}}`. The "Kept" wording is retired in code and UI ("Saved for N min").
  - `api_request` inputs drop the `label` field; the prompt shows the exact variable name.
- Scripts are shown formatted (display only; the text that runs is unchanged; a small note says the view is formatted) everywhere a script is shown (prepare, transform, query script, prompt and review).
- Approval card, "script might keep" box is redundant: show what a script keeps highlighted in the script itself (or fold into "Will keep"), less verbose.
- Prepare script uses the same `plain` and `secret` that are interpolated in the request, so the user sees clearly what happens.
- Review dialog Request section: must look expandable, sit last but before Details, and show its content like the other sections when expanded.
- "Enter a new value" input: today it reads as pre-filled but editable. One box that lists every already-set variable instead of one box per variable.
- Approvals dialog groups several prompts: group by agent session; "Deny all" denies only that session's pending requests; the counter ("1/3") stays on the header line; "Deny all" is a normal button.
- Flow test plus UI spec pair per CLAUDE.md Tests; refresh visual baselines; run the real `claude` row for the DB/API MCP after changes.

Deliverable. Plan `plans/P732.md`, Opus-authored, then Sonnet implementation.

Result. Todo.

## P733

Runs after P732 (P732 renames the variable API and edits tool descriptions). Planning starts now in parallel and
plans against the P732 plan's end state; implementation starts once P732 is done, planned against that tree.

Scope (user request).

- Audit every Kira Studio MCP tool the agents see (DB, API, kube and any other studio server; planner finds them):
  names, descriptions, input schemas, results, error messages, server-level instructions.
- Goal: efficient for the agent. Short descriptions that still tell the agent from the start what to do and the
  order of calls; no repeated rules; no text the agent does not need until a specific error.
- Planner decides, with evidence from current MCP and agent best practices (tool-definition token cost,
  fewer and more capable tools, consolidated tools, response shaping and size limits, error messages that name
  the fix, pagination, avoiding re-sent data):
  - which tools merge or split, and which are dropped (hard cut, no alias);
  - description and schema rewrites, measured in tokens before and after;
  - result shaping: smaller default payloads, field selection, truncation notes the agent can act on;
  - what is dynamic (loaded only when needed) vs always sent.
- Verification: token count of the full tool list before and after (script in the plan, not committed unless it
  guards a budget), the real `claude` rows that exercise the tools (DEV_ENVIRONMENT) with call counts for
  representative tasks before and after, flow test plus UI spec pairs for behaviour changes.

Deliverable. Plan `plans/P733.md`, Opus-authored, then Sonnet implementation.

Result. Todo.
