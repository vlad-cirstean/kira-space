# Working agreement

## Communication style

Applies everywhere in this repo and every session working on it — chat replies, code comments,
commit messages, issue/PR/MR text, and every markdown doc in the repo (this file included). Sole
exception: `README.md` at repo root stays normal prose, since it's the outward-facing entry point
for people outside this convention.

Write terse. Substance only, no filler. Cut articles (a/an/the), hedge words (just/really/
basically/actually/simply), and pleasantries (sure/certainly/of course/happy to). Fragments are
fine. Prefer the short word over the long phrase (`fix` not `implement a solution for`). Standard,
widely-known acronyms are fine (DB, API, HTTP); never invent new shortenings (cfg, impl, req, res,
fn) — a tokenizer splits an invented abbreviation same as the full word, so it saves nothing and
just costs the reader a decode. Same logic kills arrow connectors (`→`): no shorter than the word
they replace. Never pad a sentence to *sound* terse either — compression only shrinks, it never
grows.

Never touch: negation words (not/never/no/only/except), numbers, units, code blocks, error strings,
API/CLI names, exact technical terms. Dropping or softening any of those flips meaning for a token
or two saved — never worth it.

Blend in plain-language clarity discipline: one idea per sentence, ~20 words max, active voice,
present tense where true, imperative for instructions ("Run X", not "X should be run"), one term
per concept (no synonym rotation), a pronoun only when its referent is unambiguous. Where caveman
terseness and this clarity discipline pull against each other, clarity wins.

No tool-call narration or preamble. No decorative tables or emoji. Don't dump raw logs or output
unless asked — quote just the one decisive line.

Drop all of the above and write normal prose for: security warnings, confirming an irreversible
action, a multi-step sequence where a dropped article or fragment could genuinely be misread, or
any other spot where the compression itself creates ambiguity. Resume terse style right after.

And the actual governing rule above all of these: if the terse phrasing isn't shorter than the
plain phrasing, use the plain phrasing. This is about saving real tokens, not performing a style.

This file is process only: how this team works. Facts about the app itself — driver choices,
protocol constraints, capability quirks, known open limitations — live in `docs/ARCHITECTURE.md`.
How to build, run and test things in whatever sandbox a session happens to be in — credential
constraints, container quirks, per-subsystem setup — lives in `docs/DEV_ENVIRONMENT.md`. Neither is
duplicated here; this file only points at them.

**Opus plans, a Sonnet subagent implements — this session only orchestrates.**

- The **main session runs on Sonnet and orchestrates only** — it doesn't implement, edit code, or
  fix findings directly. Its job: spawn the right subagents in order, carry context between them,
  and track progress. The actual writing always happens in a subagent.
- Each phase (the current chapter's `docs/v<chapter>/SPEC.md` phasing table; except the chapter
  close phase, see "Closing a chapter") needs an
  Opus-authored plan committed under that chapter's `plans/` before implementation starts — spawn
  an **Opus subagent** (`Agent` tool, `model: "opus"`) whose only job is writing that plan. No plan
  there means no implementing straight from the spec; get the plan written and committed first.
- **Once the plan lands, spawn a Sonnet subagent** (`model: "sonnet"`) to implement it. Default to
  **one sequential subagent for the whole phase** — a fresh subagent starts cold, so its prompt
  must carry the plan and whatever prior-phase context it needs; never assume it remembers
  anything. **Parallel subagents only when the plan's work is genuinely independent** (unrelated
  adapters, non-overlapping fixes) — never split one continuous, order-dependent piece of work
  across subagents to run it concurrently.
- **A genuinely independent split runs as named streams** (Stream A, Stream B, …), each its own git
  worktree off the same base commit — the pre-commit hook lints/typechecks the whole tree, so a
  shared checkout would fail implementers' commits on each other's half-done edits. The plan states
  the split explicitly: a per-stream file-ownership table naming each file's owner, plus
  confirmation of no ordering dependency between streams. Small overlap is allowed: a few shared
  files (registries, i18n catalogs, route tables, config lists) touched by more than one stream,
  each listed in the table with the edit each stream makes, in additive hunks that rebase cleanly.
  Large or semantic overlap, or no confirmation of no ordering dependency, means no split — default
  back to one sequential implementer (P110's iter2 plan rejected a split: real overlap in a shared
  script, plus a rename dependency chain between two would-be streams). Cap concurrent streams at 3.
  Landing: once every stream is implemented and independently verified, rebase each onto the
  chapter branch in any order; resolve a conflict in a listed shared file, stop on any other
  (the ownership boundary was wrong), remove the worktrees, push. A worktree survives a container restart — resume a stopped stream from
  its last commit, never restart from scratch.
- **Implement the whole plan first, then test once and fix what's found** — don't gate every
  intermediate commit on the full test suite. Fast checks (typecheck, lint, build) are cheap and
  fine per-commit; an expensive suite (end-to-end/UI, a real-hardware check) runs once near phase
  end, with fixes landing as follow-up commits. Commits still land incrementally as work completes,
  for legible history — only *when* the expensive verification happens changes, not whether the
  result must be correct or commits stay granular.
- **One full run of an expensive suite per chapter, not per phase.** Implementers run fast checks
  (typecheck, lint, build) and only their own new or changed specs per phase. After the last
  implementation phase, before the code review, run the full end-to-end/UI/Go suites once. The run
  isn't done until every failure is fixed, committed and passing on rerun. After a fix, rerun only the failing specs and the specs that share its code. A timeout under load gets a
  lone rerun of that spec first. Rerun the full suite a second time only when a fix touched shared
  code (a package, a fixture, a config) that targeted reruns cannot cover; say so in the Result.
- **Every chapter ends the same way, unasked:** last implementation phase, then the one full suite
  run with its fixes committed, then 1 code review round (unless the spec row says more), then the
  close phase, then the PR. The user need not restate it.
- **The loop per phase:** check for a plan, spawn an Opus subagent to write one if missing, spawn
  a Sonnet subagent (or several, only if genuinely parallelizable) to implement the whole phase, and
  wait for it before moving on. Phases run in table order, one at a time, unless the user
  asks for parallel phases. **A phase isn't done until it's both planned and implemented.** Don't
  start the next phase's plan until the current phase's Sonnet implementer has finished and its
  work is committed. Exception, on the user's request: run a batch of up to 3 phases concurrently,
  each its own stream (own worktree, own Opus plan, own Sonnet implementer). Batch only phases with
  no ordering dependency and at most small overlap (see the streams rule above). Prefer batching
  whole phases over splitting one phase into parts. The batch's grouping lives in a plan file
  under the chapter's `plans/`.
- **The orchestrating session verifies before accepting — a plan or an implementation isn't done
  because a subagent says so.** Before marking a phase's plan, or its implementation, complete,
  check the result against the full original ask: the `SPEC.md` row's own wording, the user's own
  request, and every standing rule this file states (library adoption, primitive replacement,
  scope boundaries) — not just that hooks/tests pass. Verify with a real check (a grep for actual
  usage, a build, a count against the plan's own numbers), not by re-reading the subagent's result
  section and trusting its prose — a claim like "X now uses library Y" gets confirmed by finding a
  real caller of Y, not by X being present in the repo. Anything short of the full ask — a
  narrowed scope nobody agreed to, a requirement quietly dropped, a library "adopted" only as
  unused scaffolding — goes back to a subagent to fix, same phase, same number. Never accept a
  partial result and move on, and never silently re-scope the ask down to match what was
  delivered; only the user narrows their own request.
- **Everything must be resumable from disk alone — a subagent or the orchestrating session can
  halt at any point, mid-phase, with no warning.** A crash, a rate limit, a kill, a context cutoff:
  none of these are edge cases to plan around later, they are the normal operating condition every
  step must already survive. So state the next step depends on never lives only in a subagent's own
  conversation — a review's findings, an audit's list, anything a later subagent or the
  orchestrating session will need — it gets written to a file as its own step, on its own commit if
  the repo's tracking it, before or as part of any handoff, never held back for a single later
  subagent to fold into a closing summary. Concretely: a review subagent writes its findings to a
  file under the current chapter's `plans/` before a fixer ever starts (never just a conversational
  handback the fixer is trusted to remember); a fixer commits each group of related fixes as it lands,
  not batched for one commit at the end. If a run is interrupted, what actually landed
  (commits, findings files, plans) must be enough to see exactly where it stopped and pick back up
  — never a state where recovering means re-deriving work an interrupted subagent already did but
  never wrote down.
- **Set a limit-reset wakeup only when the user says so.** Never on own initiative. Use `send_later`
  for the reset time with "resume from `SPEC.md`, `plans/`, git log". Cancel it when done.
- **A failing test, lint finding, typecheck error, or any other code-quality/hook check gets fixed
  on the spot, pre-existing or not.** "Pre-existing" justifies skipping root-cause investigation of
  whether *this phase* caused it, never skipping the fix itself. Confirm it predates the phase (e.g.
  `git diff --stat` against the phase's start commit touches none of the failing file), then fix it
  in that same implementation pass and commit it — don't just note it as pre-existing/unrelated and
  move on. Only exception: fixing it needs work genuinely outside the phase's own scope (a different
  subsystem, a real design decision) — then it becomes its own named follow-up phase in `SPEC.md`,
  not a line in a result section.
- **`--no-verify` never means done.** A pre-commit hook failure (test, lint, typecheck, or any other
  check the repo runs) gets root-caused and fixed, full stop — `--no-verify` is not a way to finish
  and move on with the hook still failing. It only ever buys time to commit mid-investigation; the
  hook must pass, clean, in a normal (non-bypassed) commit before the phase — or any task — counts
  as done. Never report something complete while a hook, on any commit that will ship, is still red.
- **Multiple passes/iterations/rounds means repeat the whole loop that many times**, not run it once
  and treat extras as optional. Each pass plans against the *current* tree (on top of everything the
  previous pass landed, never the pre-phase state) and gets its own file under the current
  chapter's `plans/` (a phase's plan plus `-iter2`/`-iter3` suffixes), so what each round found
  stays legible. A planning pass re-reads the current source rather than trusting the previous
  pass's summary prose, and states plainly when a pass finds nothing real rather than manufacturing
  a finding.
- **"Code review"** (once a phase or batch is otherwise complete, on request) means **one Opus
  subagent** covering all three dimensions: (1) architecture/structure/maintainability/security,
  (2) functional correctness and business logic, (3) performance and resource efficiency. It only
  reports findings, never fixes. It writes them to a file under the current chapter's `plans/` and
  commits it before any fixer starts. Then one sequential Sonnet subagent fixes every finding (parallel
  only for a batch genuinely isolated from each other). Repeat the whole cycle for as many rounds as
  asked, each round over the full diff since the base — a round finding nothing real should say so, not manufacture a finding. No findings document
  survives a round once fixed. The fixer commits per logical group of related findings (same file,
  module or root cause), not one commit per finding, so the commit log stays short and still durable. Carry forward only a genuinely still-open item (see "Known open items"),
  never a running narrative of what each round found. **A review has no separate planner step.**
  The reviewer scopes its own area at the start of its run (owned files, one-hop callers, what
  earlier rounds parked) and goes straight to findings. It treats commits other
  fixers made in its files since the base as unreviewed code. Implementation phases keep their
  Opus plan.
- **A review's scope is everything changed since the last review session**, not the whole repo and not
  just the latest chapter. Find that session in git (the last commit of a prior review's findings or
  fixes, e.g. P108's close-out `771512bc`) and diff from there. State the base commit in the review's
  findings file. Consecutive rounds each cover the full diff since that same base; a round-1 fixer's commits are unreviewed code in round 2.
- **One branch and one PR per chapter; no per-phase PRs.** Branch `chapter/v<version>`, tag
  `v<version>`. The user names the version at chapter start; otherwise take the latest tag and add 1
  to its patch. Work and push only on the chapter branch, never a `claude/*` session branch:
  `.githooks/pre-push` refuses any branch but `main` and `chapter/v<semver>`. The merged PR tags and
  releases itself (`docs/PACKAGING.md` §7).
- **Every chapter uses `P` phase numbers, one running sequence across the whole repo, not
  per-chapter.** v1.1/v1.2/v1.4/v1.6/v1.8/v1.9 continue one counter (v1.6 topped out at `P70`; v1.8
  starts at `P71`, v1.9 at `P96`, v2.0 at `P126`, v2.1 at `P185`, v2.2 at `P210`, v2.5 at `P271`, v2.6 at
  `P294`). Before opening a new chapter, read the highest `P` in `docs/HISTORY.md` and in an open
  chapter's `SPEC.md`, if one exists, and continue from there. v1.5's `C` lettering and v1.7's `M` lettering predate this rule and
  stay as shipped, never renumbered; every chapter from here on uses `P`.
- **Closing a chapter folds it, by hand.** The chapter's last phase closes it; the Sonnet implementer
  writes the close itself, never with `scripts/docshistory` or any tool. Close phase is exempt from
  the Opus-plan rule: one Sonnet implementer does it directly, no planner, no plan file. In the commit before the
  fold, mark the row Done and write its Result in `SPEC.md`, so its entry folds closed. Then write the
  `<!-- chapter <ch> -->` block in `docs/HISTORY.md`. Copy the previous chapter block's format: header
  `Source: docs/<ch>/ at <commit>. N phases.` with the true row count, then one `### <ch> <id> (Done)`
  heading per `SPEC.md` row in table order, distilled from each row's Result. Fix links to the chapter
  by hand. Commit. Then `git rm -r docs/<ch>/` (and `docs/history-runs/` if it exists) and commit.
  Push, then open the PR from `chapter/v<version>` to `main` (command in `docs/DEV_ENVIRONMENT.md`). It auto-merges once CI passes. Opening the PR is part of closing, unless changes are still pending (uncommitted, unpushed or unfinished work) — then report them and hold the PR. After opening it, subscribe to it (`subscribe_pr_activity`) and watch it: when a check fails, reproduce the failure, fix it on the chapter branch, rerun that check, push. Repeat until CI is green and the PR merges. Never skip, disable or quarantine a test to get green.
  A chapter started from another chapter's branch rebases onto `main` before its PR opens. If the previous chapter's PR has not merged, even after trying to fix its CI, do not open the dependent PR. Continue with the next chapters, finish every other step (suite run, reviews, close, push), and report the PR as held.
  Between chapters no `docs/v*/` exists. Open the next with `docs/v<new>/SPEC.md`
  (intro: first `P` is highest plus one, branch `chapter/v<new>`) and `plans/`.
  Data a test or script reads at runtime never lives under `docs/v*/`; it goes in its reader's `testdata/`.
  Move any found first, own phase. Hand-writing the close entry is the standard close; any other
  edit to `docs/HISTORY.md` stays forbidden.
- **Splitting a phase keeps its number — it doesn't consume a new one, when an agent is the one
  deciding to split it.** A subagent (an Opus planning pass finding the scope is really two
  pieces, or anyone else in the loop) that decides a `SPEC.md` row needs splitting renames it
  `P<n> Part 1: …` and adds `P<n> Part 2: …` (`Part 3`, etc.) right after it — never `P<n>` plus a
  freshly incremented `P<n+1>`. Each part still gets its own plan under `plans/`, its own
  implementation pass, and its own result section — the split only changes the numbering, not the
  loop each part goes through. This is the agent default, not an absolute: the user can still ask
  for a split into new, separate `P` numbers directly (P98/P99 in v1.9 is exactly that, done at the
  user's own request before this rule existed) — that call is the user's to make, not a subagent's
  or the orchestrating session's own to infer.
- **Table position and `P` number stay in lockstep, no exceptions.** A phase that must run earlier
  than already-numbered ones (a dependency discovered later, a phase whose files a later one would
  otherwise touch twice) takes the earlier slot's number and every phase from there on shifts up by
  one. A phase that must run later than its original slot (a dependency on a later phase's own work
  landing first, discovered after numbering) is renumbered to its new position instead — the highest
  number, if it moves to the end. Never a number that contradicts table position, in either
  direction; there is no reordering that skips the renumber (v1.9's P101 was briefly treated as an
  exception, then reversed and renumbered to P105 once this contradiction was noticed — that
  precedent stands for "renumber," not "leave a gap"). Update every cross-reference to a renumbered
  phase in the same pass (other rows' prose, not just titles) so nothing in `SPEC.md` still points at
  the old number.
- **Implementation proceeds in the table's top-to-bottom order, full stop, unless the user
  explicitly says otherwise for that phase.** The row order is the execution order — not a
  suggestion to weigh against convenience, in-flight subagent scope, or judgment about what seems
  more urgent. Don't reorder execution to skip ahead or double back without the user asking.
- **Best practices throughout, no shortcuts** — no stubbed error handling, no `TODO: fix later`, no
  skipped validation to make something demo. Scope left out of a phase stays out entirely, never
  half-implemented.
- **Reach for an existing, well-maintained library before hand-rolling non-trivial infrastructure**
  — a parser, a virtualizer, a positioning engine, retry/backoff, and similar. This repo already
  relies on Monaco, zod, sql-formatter and SlickGrid rather than reimplementing them. A
  hand-rolled version earns its keep only against a real requirement no library meets (e.g.
  spelling-preserving timestamp re-encoding, or the parse tree P60b's SQL tokenizer replaced after
  the library carrying it was removed app-wide) — name that requirement when declining a library,
  not just that existing code already works.
- **Lean on the frontend libraries P98 wired in — shadcn-vue, Tailwind CSS, VueUse, Pinia, TanStack
  Query — rather than hand-rolling an equivalent.** Every phase's planning pass adapts its design to
  them, whether or not the spec row asking for the work says so: a new UI surface styles with
  Tailwind utility classes, not a scoped `<style>` block; a component primitive (button, dialog,
  dropdown, menu, popover) comes from shadcn-vue; a browser/DOM composable (debounce, resize and
  intersection observers, event-listener wiring, local-storage sync, clipboard) comes from VueUse;
  shared client state lives in a Pinia store; server state — anything fetched over the bridge with
  loading, error and cache handling around it — goes through TanStack Query. Decline one only
  against a real requirement it cannot meet, and name that requirement, same standard as the
  library rule above.
- **Every Vue component is `<script setup lang="ts">`.** Composition API only — no Options API, no
  `defineComponent`, no second plain `<script>` block.
- **One Pinia store, one concern.** A store owns a single subsystem's state; never a grab-bag
  app-wide store. Split one that grows a second concern rather than widening it.
- **Only fully open-source libraries** — no community edition of a dual-licensed product, no
  non-commercial-only tier, no functionality gated behind a paid/Enterprise tier. Check the license
  at the package level *and* for the specific feature used, not just the headline badge (AG Grid
  Community's own license is fine; the context menu/range selection/clipboard features this app
  needed are Enterprise-only, so declined). Applies to every new dependency.
- **Never touch a credential a driven CLI owns** (gh, az, acli, …): no token commands, no key or
  token reads, no auth headers. Run the CLI, read its output. Reading non-secret config (host,
  docker context `meta.json`) is fine. Key or token material may be read only by the CLI's own
  library, never Kira code. Rule and facts in `docs/ARCHITECTURE.md` "CLI credentials: never
  touched". Guard G5 enforces it under `githost/`, `issuetracker/`, `internal/docker` and `internal/harness/claude`.
- **Measure when there's a real, concrete question at stake, not as a default ritual.** A
  real-hardware trace, CDP tracing, or a byte-for-byte bundle comparison earns its keep when a
  claimed fix, regression, or cost genuinely can't be checked another way. Don't extend that rigor
  to routine changes or every declined option — a short honest estimate or a plain read of the
  code's/library's stated behavior is enough there. Skip a measurement that wouldn't change the
  decision.
- **Comments: very concise, only where truly necessary.** Add one only when the code can't say it
  — a non-obvious *why*, a constraint, a workaround. Never restate what the code shows.
- **Unit tests exist only for advanced, complex or deeply nested logic — this app has very little.**
  Default to *no dedicated unit test*. A test earns its keep only guarding something genuinely hard
  to get right: a parser/splitter with several interacting rules, cursor/pagination boundary
  arithmetic, cache eviction/invalidation with interacting rules, crypto beyond
  encrypt-then-decrypt, concurrency (ordering, backpressure, cancellation, races), or a decision
  structure too large to hold in your head. Everything else gets nothing: CRUD round-trips (even
  integration-shaped), one/two-condition validation, required-field/enum guards, thin pass-through
  wrappers, constructors/builders, format round-trips with no edge case,
  single-bad-input-to-single-error paths, anything that mostly restates a short function body. A
  single `if` guarding one obvious case isn't complexity. When torn between two similar tests,
  delete. Applies going forward, not as a retroactive cleanup.
- **The adapter conformance suites are exempt from that bar, not an application of it.**
  `apps/kira-studio/internal/adapters/*/*_test.go` are the sole successor to the deleted
  `packages/db-fixtures/*.spec.ts` files — nothing else exercises a Go adapter capability by
  capability (`tests/e2e-real/` only spot-checks a scenario or two per kind). Keep per-capability
  coverage there even where it reads like a CRUD round-trip; prune only genuine duplication.
- **New feature tests behave like a full e2e, split at the IPC boundary:** same scenario runs once on the backend (flow test, assert the result) and once on the frontend (UI spec, assert the same result). Together they prove the feature end to end; each half stays easy to validate. Applies to every new feature from now on. Keep `e2e-real` for what cannot be split.
- **Real-container adapter tests split into two suites, by design (P25).** A *general* suite runs
  frequently in the normal dev/CI loop — basic per-adapter connectivity sanity, not the full
  permutation matrix. A *complete* suite runs only on-demand and in CI — the full auth/config
  permutation matrix per adapter (root vs. least-privilege user, with/without password, with/without
  the database-equivalent field) plus error-handling verification, deliberately comprehensive since
  it's opt-in, not part of every local run. Extend the complete suite's own harness for new
  functional coverage (load/write/delete/filter/DDL, per adapter) rather than building a parallel
  mechanism — it's designed for that (P25's own `Scenario`/`Requires` seam, populated by P26).
- **Real `claude` tests are opt-in and cost tokens.** After changing Claude integration code, run the
  matching row of `docs/DEV_ENVIRONMENT.md` "Real `claude` tests" (`bun run test:claude*`). They live in
  nested Go modules under `apps/*/tests/claude/`. Never wire them into hooks or CI. Run on this machine;
  verify auth with the probe in DEV_ENVIRONMENT, never assume no login.
- **Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)** —
  `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, with a `!` or `BREAKING CHANGE:` footer
  for breaking changes.
- **Keep this file lean — prune as you go, don't just append.** An app fact belongs in
  `docs/ARCHITECTURE.md`; an environment/tooling fact belongs in `docs/DEV_ENVIRONMENT.md`; a phase
  or review round's discovery belongs in that phase's plan doc under the current chapter's `plans/`
  (never a permanent "findings" section here — the plan doc and commit log are the durable record,
  not this file). Before adding a bullet, ask whether it's a standing rule for how this team works,
  not a one-off result or a fact about the app/environment. When you touch this file, remove what's
  gone stale too — a pointer to a deleted file/subsystem, a question a later phase already resolved.
- **`docs/ARCHITECTURE.md` keeps its own "Known open items" section** for a real, currently-true
  limitation (not a TODO or a wishlist) — keep an entry only while genuinely open, delete it the
  moment it's resolved, not marked done in place.
- **`.github/workflows/` can't be pushed from this session** (an OAuth scope limit, not a policy
  choice) — see `docs/DEV_ENVIRONMENT.md`'s own section for the constraint and the
  `docs/pending-changes/`/`docs/pending-workflows/` workaround before touching a workflow file.

## CodeGraph

Code navigation in this repo goes through [CodeGraph](https://github.com/colbymchenry/codegraph):
symbol index, call graphs, blast radius, registered as an MCP server via the
committed `.mcp.json` — use the MCP tools, not the `codegraph` CLI.

**Mandatory for discovery, not for applying a known fix — and verified, not assumed.** Real usage
splits cleanly along that line: an audit agent (P107 iter2's own Fable audit) called
`codegraph_explore` 37 times, all before it ever called `Read` — finding duplication candidates and
tracing call graphs needs it. An implementer given an exact file:line and a named fix from a
findings doc called it zero times — `Edit` requires its own prior `Read` on that exact file
regardless of what CodeGraph already showed, so a resolved fix has no discovery left to do.

So: **mandatory** whenever a subagent is finding or understanding something it doesn't already know
— an Opus plan being written, a duplication/bug audit, tracing what calls what or a change's blast
radius before a structural decision. Read the injected context before opening whole files; call
`codegraph_explore` before Read/Grep for any symbol/call-graph/blast-radius question in that work.
**Not required** for a subagent executing an already-named fix (an exact file:line and a concrete
change from a committed plan or findings doc) — there's nothing left to discover, and forcing a call
there is busywork, not signal.

CLAUDE.md being on disk in a subagent's worktree doesn't make it follow this — every subagent
prompt doing discovery work (Opus planner, audit agent) must restate the requirement explicitly.
Before accepting a plan or an audit's findings (this file's own verification rule), the
orchestrating session confirms real `codegraph_explore` tool calls happened in
that subagent's own run — grep its tool-call log, don't take "I used CodeGraph" on prose alone.
Loading the tool via `ToolSearch` without ever calling it doesn't count. A discovery subagent that
skipped it despite loading it goes back to redo the lookup, same as any other short-of-the-ask
result.

**The tools aren't in the default tool list.** Call `ToolSearch` for `"codegraph"` first — it
loads `codegraph_explore` by name, then it's callable like any other tool. Don't fall back to the
CLI just because it's not visible yet; search for it.

- **Automatic** — a `UserPromptSubmit` hook (`codegraph prompt-hook`) fires on every message and
  injects matching symbols as `<codegraph_context>`, no tool call needed. Read it before
  searching files.
- **Explicit** — `codegraph_explore` answers most code questions in one call: the relevant
  symbols' source plus the call paths between them, including dynamic-dispatch hops grep can't
  follow.

`scripts/codegraph-setup.sh` installs the `codegraph` binary and builds/syncs the index once per
worktree, before a Claude session is spawned into it (not a session hook — `docs/DEV_ENVIRONMENT.md`
has the setup) — that's build tooling, not how code gets navigated; navigation is the MCP tool above.
