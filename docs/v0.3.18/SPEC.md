# v0.3.18

Full-app UI uniformity audit, then a JavaScript engine for the API MCP: scripted transforms, result review with raw and transformed views, chained requests through kept variables, user-supplied values the agent never sees. Continues from chapter v0.3.17.
Phases from `P674` (highest shipped: `P673`, chapter v0.3.17). Next new phase: `P682`. Branch
`chapter/v0.3.18`.

| # | Phase | Status |
|---|-------|--------|
| P674 Part 1 | Full-app UI uniformity audit: audit tool `scripts/uiaudit/` | Planned |
| P674 Part 2 | Full-app UI uniformity audit: iteration 1 | Planned |
| P674 Part 3 | Full-app UI uniformity audit: iteration 2 | Planned |
| P674 Part 4 | Full-app UI uniformity audit: final whole-app scan | Planned |
| P675 | JS engine for the API MCP | Planned |
| P676 | API MCP: scripted request preparation and result transform | Planned |
| P677 | API MCP: result review with raw and transformed views | Planned |
| P678 Part 1 | API MCP: kept variables and chained requests, store, syntax, approval card, settings | Planned |
| P678 Part 2 | API MCP: kept variables script binding (`kept.get`/`kept.set`), held writes, review-card "Will keep"; runs after P676 and P677 | Planned |
| P679 | API MCP: user-supplied values the agent never sees | Planned |
| P680 | DB MCP: JS scripting and kept variables for SQL | Planned |
| P681 | Dependency cleanup | Planned |

Chapter end (unasked, per working agreement, after the last implementation phase P681): non-UI full run, 2 code review rounds (user set 2 for this chapter), visual baseline re-record in this sandbox (fonts match CI, check `fc-match sans-serif` prints `Inter` first), close, PR.

Standing rules for every phase below: new feature tests split at the IPC boundary (flow test plus UI spec, same scenario); UI follows `docs/ARCHITECTURE.md` uniformity rules and the guard script; no secret ever reaches an agent, a log or the op log; open-source licences only (check package and feature level).

## P674 Parts 1 to 4: Full-app UI uniformity audit

Same method as v0.3.17 P672, but whole app with no subject list: the earlier pass was organised by subject (forms, buttons, menus, headings, status, tabs, states). This pass finds reimplementations and variation the subject list could not name. Scope is every module (Studio, Space, git-ui, ADE, docker-ui, mobile, capture, settings, approvals, site where it shares theme).

Look for: components reimplemented near a shared primitive instead of using it; two components doing one job; module-local wrappers that duplicate a theme or workbench piece; per-module copies of a layout, row, list, header, empty or error pattern; hand-built widgets that shadcn-vue, VueUse, Pinia or TanStack Query already provide; same data shown in different ways in different places (dates, sizes, durations, counts, identifiers, paths, shortcuts); one concept with several names or icons; duplicated state logic that belongs in a shared composable or store; inconsistent copy for the same action. Each finding names the shared home it moves to. The plan decides split into Parts, iteration count and guards; fresh scan per iteration, final all-subject scan last, carry-forward items listed.

Plan: `plans/P674.md`.

## P675: JS engine for the API MCP

Add an embedded JavaScript engine in Go, used by the API MCP (P676 to P679) and later the DB MCP (P680); no UI-facing scripting. Design the interface for both from the start. Engine: `dop251/goja` (pure Go, MIT; user decision); the plan verifies licence of every linked module and maintenance, records the alternatives compared, and decides how the memory ceiling and CPU limit hold given goja has no memory limit API. Requirements: hard CPU time limit and interrupt, memory ceiling, output size cap, no filesystem, network, process, timers or `require`; deterministic globals only (JSON, Math, Date, string and array builtins); big-integer handling documented (JSON numbers above 2^53); errors returned as structured messages with line and column. One engine package with a narrow interface so later features reuse it. Not in scope: jq or gojq filters.

User accepted MPL-2.0 for goja's `ftoa` package (Rhino DToA port; open source, file-level copyleft): use unmodified, keep headers, NOTICES points to the public source at the pinned commit; any patch to those files must be published. Engine stays goja in a per-run child worker process.

Plan: `plans/P675.md`.

## P676: API MCP: scripted request preparation and result transform

Agents use the engine to prepare an API call for the user and to post-process its result before it returns to the AI: parse, pick fields, or anonymise. The user must see the script: it appears in the approval card next to the request (read-only, syntax highlighted), and in the op log. The AI receives only the script output, never the raw response (unless the script returns it). Script input is the response (status, headers, body text, parsed JSON when valid). A failing or over-limit script returns a clear error to the agent, and the user sees the same error. Approval modes, redaction of credentials and the existing size limits keep applying; the plan decides tool shape (new argument on `api_request` versus new tool), schema docs in the prompt registry, and settings.

Plan: `plans/P676.md`.

## P677: API MCP: result review with raw and transformed views

The DB MCP has a review step for results (the approval body for responses). Check whether the API MCP has an equivalent; if not, add it. The user can see, before anything returns to the AI: the raw response (status, headers, body) and the result after the JS transform, as two views of one review, and the script that produced it. The user approves or denies what the agent receives. Follows the shared approvals surface (`Approvals surface`, P520) and its item bodies. Settings: whether results are reviewed, mirroring the DB connection option.

Plan: `plans/P677.md`.

## P678 Part 1 and Part 2: API MCP: kept variables and chained requests

Chained requests: an agent gets ids from one request and passes them, parsed into a list, into a later POST. A script can store a result in a named variable held by Kira. The stored value is never returned to the AI; the agent can only refer to it by name in later requests and scripts. It must be obvious to the user that a request uses a variable: the approval card lists every variable the request or script reads (name, value, which request created it, age, expiry) and highlights where it is used in the request and script. A variables view (settings pane or panel) lists live variables with value, source and remaining time, and can clear them. Retention is short: default 1 hour, in memory only, wiped on app exit, expiry refreshed never silently. The user sees values in full (secrets from P679 masked until revealed); the agent sees names only. The store is MCP-agnostic (P680 reuses it for SQL): one package, entries tagged with their source MCP and request. The plan decides storage package, size caps, naming rules, and how variable use is stated in agent-facing tool descriptions.

Plan: `plans/P678.md`.

## P679: API MCP: user-supplied values the agent never sees

The agent prepares a request that needs a value only the user has (a token). The agent declares an input (name, label); the user provides it in the approval UI; the agent never sees the value; the value is reusable by name afterwards through the same variable store and retention as P678, marked as user-supplied and secret (masked by default, reveal on demand). Request and script reference it like any variable. Never logged, never in op log or history, never in prompts; redaction applies to responses that echo it. The plan covers entry UI, reuse across requests within retention, clearing, and what the agent is told (that an input exists, never its content).

Plan: `plans/P679.md`.

## P680: DB MCP: JS scripting and kept variables for SQL

Give the SQL (database) MCP the same abilities as the API MCP: the agent prepares a query for the user and can post-process its result with a JS script before the result returns to the AI (parse, pick, anonymise); the script is shown to the user next to the statement in the approval card and the op log; results can be kept in named variables held by Kira (P678 store, same retention and visibility rules) and used by later queries or scripts, for example ids from one query passed as a list into the next. Kept values never return to the AI. Uses the P675 engine and the P678 store without forking either. Interacts with existing connection MCP settings (read, write and DDL modes, review results, masking): the script never widens what a connection allows, runs after masking and redaction, and a query's approval class is decided from the statement, not the script. Review view shows raw rows and transformed result as in P677. The plan decides tool shape, how variables bind into SQL safely (parameters, never string splicing), and per-adapter limits.

Plan: `plans/P680.md`.

## P681: Dependency cleanup

Audit every direct dependency and remove what is dead or prototype-only. Scope: Go modules in `go.mod` and `apps/kira-space/go.mod`; npm packages in every `package.json`, devDependencies included. Remove: the dev-only `apps/kira-space/frontend/proto/grid` Cheetah Grid prototype (P165) with `cheetah-grid`, `vue-cheetah-grid`, `cheetah-grid-playwright`, `vite.proto.config.ts`, its knip entries and scripts; `seti-icons` (v0.0.4, last release 2020; check real use in `packages/git-ui` and fileIcon); anything unused per knip/depcheck-style checks and real import greps; duplicates doing the same job; deps used only by removed code. For each low-popularity or stale survivor (audit examples: wasilibs/go-pgquery, gomlx/go-huggingface, rqlite/sql, AfterShip/clickhouse-sql-parser, fsnotify/fsevents, aymanbagabas/go-udiff; npm slickgrid is used and stays) record in the plan: use, last release, replaceability, decision keep or replace. Replace only where a clearly better maintained open-source option meets the same requirements, licence-checked at package and feature level. Verify removal by real build (typecheck, `lint:dead`, `go build ./...`, affected tests); add no dependency without licence check. Delete orphaned scripts, configs, docs and guard-script references.

Plan: `plans/P681.md`.
