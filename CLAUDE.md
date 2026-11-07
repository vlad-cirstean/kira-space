# Working agreement

@docs/WORKING_AGREEMENT.md

General rules live in `docs/WORKING_AGREEMENT.md`, imported above. Rules below are kira-space only.

## Locations

- Architecture doc: `docs/ARCHITECTURE.md`. Dev-environment doc: `docs/DEV_ENVIRONMENT.md`.
- `SPEC.md` and `plans/`: open chapter's `docs/v<ch>/SPEC.md` and `docs/v<ch>/plans/`. Between
  chapters no `docs/v*/` exists. Open next chapter with `docs/v<new>/SPEC.md` (intro: first `P` is
  highest plus one, branch `chapter/v<new>`) and `plans/`.
- Data a test or script reads at runtime never lives under `docs/v*/`; it goes in its reader's
  `testdata/`. Move any found first, own phase.
- `.github/workflows/` can't be pushed from this session (OAuth scope limit, not policy). Before
  touching a workflow file, read `docs/DEV_ENVIRONMENT.md`'s section on it and the
  `docs/pending-changes/`/`docs/pending-workflows/` workaround.

## Chapters, branches, phase numbers

- Branch `chapter/v<version>`, tag `v<version>`. User names version at chapter start; otherwise take
  latest tag and add 1 to its patch. Work and push only on chapter branch, never a `claude/*` session
  branch: `.githooks/pre-push` refuses any branch but `main` and `chapter/v<semver>`. Merged PR tags
  and releases itself (`docs/PACKAGING.md` §7).
- Phases use `P<n>`: one running sequence across whole repo, not per chapter (v1.6 topped out at
  `P70`; v1.8 starts at `P71`, v1.9 at `P96`, v2.0 at `P126`, v2.1 at `P185`, v2.2 at `P210`, v2.5 at
  `P271`, v2.6 at `P294`). Before opening a chapter, read highest `P` in `docs/HISTORY.md` and in open
  chapter's `SPEC.md`, if one exists, and continue from there. v1.5's `C` and v1.7's `M` lettering
  predate this rule and stay as shipped, never renumbered.
- Chapter's non-UI full run: Go, typecheck, lint, site and package tests, flow tests. UI suites
  (Playwright `ui`, `visual`, `ui-timing`, screenshots) stay out of it and run in PR CI.

## Closing a chapter

Chapter's last phase closes it by hand. Its Sonnet implementer writes the close itself, never with
`scripts/docshistory` or any tool.

1. In the commit before the fold, mark the row Done and write its Result in `SPEC.md`, so its entry
   folds closed.
2. Write the `<!-- chapter <ch> -->` block in `docs/HISTORY.md`, copying the previous chapter block's
   format: header `Source: docs/<ch>/ at <commit>. N phases.` with the true row count, then one
   `### <ch> <id> (Done)` heading per `SPEC.md` row in table order, distilled from each row's Result.
   Fix links to the chapter by hand. Commit.
3. `git rm -r docs/<ch>/` (and `docs/history-runs/` if it exists). Commit.
4. Push, then open the PR from `chapter/v<version>` to `main` (command in `docs/DEV_ENVIRONMENT.md`).
   It auto-merges once CI passes. Opening the PR is part of closing, unless changes are still pending
   (uncommitted, unpushed or unfinished work). Then report them and hold the PR.

Hand-writing the close entry is the standard close; any other edit to `docs/HISTORY.md` stays
forbidden.

PR CI:

- After opening PR, subscribe (`subscribe_pr_activity`) and watch it. UI and visual failures in CI
  are expected.
- Per CI run: wait for whole run to finish, collect every failing check, reproduce and fix all of
  them on chapter branch, rerun them, push once. New run fails: repeat until CI is green and PR
  merges. Never push after an isolated fix. Never skip, disable or quarantine a test to get green.
- Chapter started from another chapter's branch rebases onto `main` before its PR opens. If previous
  chapter's PR has not merged, even after trying to fix its CI, do not open dependent PR. Continue
  with next chapters, finish every other step (suite run, reviews, close, push), report PR as held.

## Frontend

- Lean on shadcn-vue, Tailwind CSS, VueUse, Pinia, TanStack Query; never hand-roll an equivalent.
  Every planning pass adapts its design to them, whether or not spec row says so:
  - new UI surface styles with Tailwind utility classes, not a scoped `<style>` block;
  - component primitive (button, dialog, dropdown, menu, popover) comes from shadcn-vue;
  - browser/DOM composable (debounce, resize and intersection observers, event-listener wiring,
    local-storage sync, clipboard) comes from VueUse;
  - shared client state lives in a Pinia store;
  - server state (anything fetched over the bridge with loading, error and cache handling) goes
    through TanStack Query.

  Decline one only against a real requirement it cannot meet, and name that requirement.
- Every Vue component is `<script setup lang="ts">`. Composition API only: no Options API, no
  `defineComponent`, no second plain `<script>` block.
- One Pinia store, one concern. Store owns a single subsystem's state, never a grab-bag app-wide
  store. Split one that grows a second concern instead of widening it.

## Tests

- New feature tests behave like a full e2e, split at IPC boundary: same scenario runs once on
  backend (flow test, assert result) and once on frontend (UI spec, assert same result). Together
  they prove the feature end to end; each half stays easy to validate. Applies to every new feature.
  Keep `e2e-real` for what cannot be split.
- Adapter conformance suites (`apps/kira-space/internal/adapters/*/*_test.go`) are exempt from
  unit-test bar. They are sole successor to deleted `packages/db-fixtures/*.spec.ts`; nothing else
  exercises a Go adapter capability by capability (`tests/e2e-real/` only spot-checks a scenario or
  two per kind). Keep per-capability coverage even where it reads like a CRUD round-trip; prune only
  genuine duplication.
- Real-container adapter tests split into two suites by design. *General* suite runs in normal
  dev/CI loop: basic per-adapter connectivity sanity, not full permutation matrix. *Complete* suite
  runs only on demand and in CI: full auth/config permutation matrix per adapter (root vs.
  least-privilege user, with/without password, with/without database-equivalent field) plus
  error-handling verification. Extend complete suite's own harness (`Scenario`/`Requires` seam) for
  new functional coverage (load/write/delete/filter/DDL, per adapter), never a parallel mechanism.
- Real `claude` tests are opt-in and cost tokens. After changing Claude integration code, run
  matching row of `docs/DEV_ENVIRONMENT.md` "Real `claude` tests" (`bun run test:claude*`). They live
  in nested Go modules under `apps/*/tests/claude/`. Never wire them into hooks or CI. Run on this
  machine; verify auth with the probe in DEV_ENVIRONMENT, never assume no login.

## CLI credentials

Never touch a credential a driven CLI owns (gh, az, acli, …): no token commands, no key or token
reads, no auth headers. Run the CLI, read its output. Reading non-secret config (host, docker context
`meta.json`) is fine. Key or token material may be read only by the CLI's own library, never Kira
code. Rule and facts: `docs/ARCHITECTURE.md` "CLI credentials: never touched". Guard G5 enforces it
under `githost/`, `issuetracker/`, `internal/docker` and `internal/harness/claude`.

## CodeGraph

Code navigation goes through [CodeGraph](https://github.com/colbymchenry/codegraph) (symbol index,
call graphs, blast radius), registered as MCP server via committed `.mcp.json`. Use the MCP tools,
not the `codegraph` CLI.

- Mandatory for discovery: whenever a subagent finds or understands something it doesn't already
  know (Opus plan, duplication/bug audit, tracing callers or blast radius before a structural
  decision). Read injected context before opening whole files; call `codegraph_explore` before
  Read/Grep for any symbol/call-graph/blast-radius question in that work.
- Not required for a subagent executing an already-named fix (exact file:line and concrete change
  from a committed plan or findings doc): nothing left to discover, and `Edit` needs its own prior
  `Read` anyway.
- CLAUDE.md on disk does not make a subagent follow this. Every discovery subagent's prompt (Opus
  planner, audit agent) restates the requirement explicitly.
- Before accepting a plan or audit's findings, orchestrating session greps that subagent's tool-call
  log for real `codegraph_explore` calls; prose claims don't count. Loading via `ToolSearch` without
  calling doesn't count. Discovery subagent that skipped it goes back to redo the lookup.
- Tools aren't in default tool list. Call `ToolSearch` for `"codegraph"` first; it loads
  `codegraph_explore` by name. Never fall back to CLI because it's not visible yet.
- Automatic: `UserPromptSubmit` hook (`codegraph prompt-hook`) injects matching symbols as
  `<codegraph_context>` on every message. Read it before searching files.
- Explicit: `codegraph_explore` answers most code questions in one call: relevant symbols' source
  plus call paths between them, including dynamic-dispatch hops grep can't follow.
- `scripts/codegraph-setup.sh` installs the `codegraph` binary and builds/syncs index once per
  worktree, before a Claude session is spawned into it (not a session hook; setup in
  `docs/DEV_ENVIRONMENT.md`). That is build tooling, not navigation.
