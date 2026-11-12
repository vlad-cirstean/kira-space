# v0.3.11

Agents module performance: speed, latency and responsiveness, backend and frontend, including Kira Space MCP tools. Phases P622 to P624 run as parallel streams A, B, C (batch requested by user). Highest prior phase: P621. Two code review rounds after the full non-UI run.

Size survey (non-test): BE `ade` 24k lines/70 files, `bridge` 15k, `harness` 6.4k, `terminal` 1.4k, `leases` 0.5k; FE `ade/v2` 34k lines/290 files, approvals 0.6k.

| Phase | Title | Status |
|-------|-------|--------|
| P622 | Backend board, tasks, orchestrator, storage | Open |
| P623 | Kira Space MCP tools and agent runtime | Open |
| P624 | Frontend agents UI | Open |

Each phase: several Opus auditors run in parallel (read-only, `codegraph_explore` mandatory), findings go into the phase plan, then one Sonnet implementer per stream fixes them.

Cross-cutting requirement (user): cover latency, not only throughput. Slow work (git, DB, MCP tools, agent steps) may move async, but every such operation shows feedback while running and when finished (running status where the user looks, completion and failure notice). Each auditor lists: slow operations with no feedback, blocking calls that should go async, feedback surface to use. Reuse `adeOps` store, `AdeOpStatus` chip and vue-sonner toasts from P618; extend, never build a parallel mechanism. Measure before and after only where a claim needs it.

## P622 Backend board, tasks, orchestrator, storage (Stream A)

- Scope: `apps/kira-space/internal/ade`, repos, bridge ADE services. Auditors: A1 board, tasks, branches; A2 orchestrator, steps, review, workflows, storage.
- Targets: query counts, git facts per call (`Board()` cost), lock hold times, event fan-out, DB writes on read paths, calls that block the UI and need async plus feedback.

## P623 Kira Space MCP tools and agent runtime (Stream B)

- Scope: `agentmcp`, `llmprompts`, `harness`, hooks, `terminal`, `leases`, approvals backend. Auditors: B1 MCP tool handlers, B2 harness, hooks, terminal, session tracking.
- Targets: tool call latency, blocking waits, approval and prompt round trips, hook reply timing, tool calls that need async plus feedback.

## P624 Frontend agents UI (Stream C)

- Scope: `frontend/src/ade/v2`, `packages/workbench` approvals and agent code. Auditors: C1 plan, board, cards, panel; C2 sessions, terminal, agents, workflows, dialogs.
- Targets: first paint and interaction latency, reactivity cost, list rendering, query refetch storms, lazy loading, dialog and prompt responsiveness, missing running and finished feedback for slow actions.
