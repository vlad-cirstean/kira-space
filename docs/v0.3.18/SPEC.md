# v0.3.18

Full-app UI uniformity audit, then a JavaScript engine for the API MCP: scripted transforms, result review with raw and transformed views, chained requests through kept variables, user-supplied values the agent never sees. Continues from chapter v0.3.17.
Phases from `P674` (highest shipped: `P673`, chapter v0.3.17). Next new phase: `P680`. Branch
`chapter/v0.3.18`.

| # | Phase | Status |
|---|-------|--------|
| P674 | Full-app UI uniformity audit | Planned |
| P675 | JS engine for the API MCP | Planned |
| P676 | API MCP: scripted request preparation and result transform | Planned |
| P677 | API MCP: result review with raw and transformed views | Planned |
| P678 | API MCP: kept variables and chained requests | Planned |
| P679 | API MCP: user-supplied values the agent never sees | Planned |

Chapter end (unasked, per working agreement): non-UI full run, 2 code review rounds (user set 2 for this chapter), visual baseline re-record in this sandbox (fonts match CI, check `fc-match sans-serif` prints `Inter` first), close, PR.

Standing rules for every phase below: new feature tests split at the IPC boundary (flow test plus UI spec, same scenario); UI follows `docs/ARCHITECTURE.md` uniformity rules and the guard script; no secret ever reaches an agent, a log or the op log; open-source licences only (check package and feature level).

## P674: Full-app UI uniformity audit

Same method as v0.3.17 P672, but whole app with no subject list: the earlier pass was organised by subject (forms, buttons, menus, headings, status, tabs, states). This pass finds reimplementations and variation the subject list could not name. Scope is every module (Studio, Space, git-ui, ADE, docker-ui, mobile, capture, settings, approvals, site where it shares theme).

Look for: components reimplemented near a shared primitive instead of using it; two components doing one job; module-local wrappers that duplicate a theme or workbench piece; per-module copies of a layout, row, list, header, empty or error pattern; hand-built widgets that shadcn-vue, VueUse, Pinia or TanStack Query already provide; same data shown in different ways in different places (dates, sizes, durations, counts, identifiers, paths, shortcuts); one concept with several names or icons; duplicated state logic that belongs in a shared composable or store; inconsistent copy for the same action. Each finding names the shared home it moves to. The plan decides split into Parts, iteration count and guards; fresh scan per iteration, final all-subject scan last, carry-forward items listed.

Plan: `plans/P674.md`.

## P675: JS engine for the API MCP

Add an embedded JavaScript engine in Go, used only by the API MCP for now (no UI-facing scripting, no other MCP). Candidate: `dop251/goja` (pure Go); the plan must verify licence, maintenance, and compare alternatives. Requirements: hard CPU time limit and interrupt, memory ceiling, output size cap, no filesystem, network, process, timers or `require`; deterministic globals only (JSON, Math, Date, string and array builtins); big-integer handling documented (JSON numbers above 2^53); errors returned as structured messages with line and column. One engine package with a narrow interface so later features reuse it. Not in scope: jq or gojq filters.

Plan: `plans/P675.md`.

## P676: API MCP: scripted request preparation and result transform

Agents use the engine to prepare an API call for the user and to post-process its result before it returns to the AI: parse, pick fields, or anonymise. The user must see the script: it appears in the approval card next to the request (read-only, syntax highlighted), and in the op log. The AI receives only the script output, never the raw response (unless the script returns it). Script input is the response (status, headers, body text, parsed JSON when valid). A failing or over-limit script returns a clear error to the agent, and the user sees the same error. Approval modes, redaction of credentials and the existing size limits keep applying; the plan decides tool shape (new argument on `api_request` versus new tool), schema docs in the prompt registry, and settings.

Plan: `plans/P676.md`.

## P677: API MCP: result review with raw and transformed views

The DB MCP has a review step for results (the approval body for responses). Check whether the API MCP has an equivalent; if not, add it. The user can see, before anything returns to the AI: the raw response (status, headers, body) and the result after the JS transform, as two views of one review, and the script that produced it. The user approves or denies what the agent receives. Follows the shared approvals surface (`Approvals surface`, P520) and its item bodies. Settings: whether results are reviewed, mirroring the DB connection option.

Plan: `plans/P677.md`.

## P678: API MCP: kept variables and chained requests

Chained requests: an agent gets ids from one request and passes them, parsed into a list, into a later POST. A script can store a result in a named variable held by Kira. The stored value is never returned to the AI; the agent can only refer to it by name in later requests and scripts. It must be obvious to the user that a request uses a variable: the approval card lists every variable the request or script reads (name, value, which request created it, age, expiry) and highlights where it is used in the request and script. A variables view (settings pane or panel) lists live variables with value, source and remaining time, and can clear them. Retention is short: default 1 hour, in memory only, wiped on app exit, expiry refreshed never silently. The user sees values in full (secrets from P679 masked until revealed); the agent sees names only. The plan decides storage package, size caps, naming rules, and how variable use is stated in agent-facing tool descriptions.

Plan: `plans/P678.md`.

## P679: API MCP: user-supplied values the agent never sees

The agent prepares a request that needs a value only the user has (a token). The agent declares an input (name, label); the user provides it in the approval UI; the agent never sees the value; the value is reusable by name afterwards through the same variable store and retention as P678, marked as user-supplied and secret (masked by default, reveal on demand). Request and script reference it like any variable. Never logged, never in op log or history, never in prompts; redaction applies to responses that echo it. The plan covers entry UI, reuse across requests within retention, clearing, and what the agent is told (that an input exists, never its content).

Plan: `plans/P679.md`.
