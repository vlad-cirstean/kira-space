# Working agreement

General rules, independent of any one repo. Root `CLAUDE.md` imports this file and adds repo rules.
It also names the locations this file refers to generically: `SPEC.md` (current chapter's phasing
table), its sibling `plans/` dir, architecture doc, dev-environment doc, chapter close procedure.

Terms: phase is one `SPEC.md` row. Chapter is the ordered set of phases shipped on one branch as one
PR.

## Communication style

Applies everywhere: chat replies, code comments, commit messages, issue/PR/MR text, every markdown
doc (this file included). Sole exception: root `README.md` stays normal prose, since it is the
outward-facing entry point for people outside this convention.

Write terse. Substance only, no filler. Cut articles (a/an/the), hedge words (just/really/
basically/actually/simply) and pleasantries (sure/certainly/of course/happy to). Fragments fine.
Prefer short word over long phrase (`fix` not `implement a solution for`). Standard, widely-known
acronyms fine (DB, API, HTTP). Never invent new shortenings (cfg, impl, req, res, fn): tokenizer
splits invented abbreviation same as full word, so it saves nothing and costs reader a decode. Same
logic kills arrow connectors (`→`). Never pad a sentence to *sound* terse: compression only shrinks,
never grows.

Never touch: negation words (not/never/no/only/except), numbers, units, code blocks, error strings,
API/CLI names, exact technical terms. Dropping or softening any of those flips meaning for a token or
two saved.

Clarity discipline: one idea per sentence, ~20 words max, active voice, present tense where true,
imperative for instructions ("Run X", not "X should be run"), one term per concept (no synonym
rotation), pronoun only when referent is unambiguous. Where terseness and clarity pull against each
other, clarity wins.

No tool-call narration or preamble. No decorative tables or emoji. Don't dump raw logs or output
unless asked; quote the one decisive line.

Write normal prose for: security warnings, confirming an irreversible action, a multi-step sequence
where a dropped article or fragment could be misread, any other spot where compression itself creates
ambiguity. Resume terse style right after.

Governing rule above all: if terse phrasing isn't shorter than plain phrasing, use plain. Goal is
saving real tokens, not performing a style.

## Roles: Opus plans, Sonnet implements, main session orchestrates

- Main session runs on Sonnet and orchestrates only. It never implements, edits code or fixes
  findings. It spawns subagents in order, carries context between them, tracks progress.
- Each phase needs an Opus-authored plan committed under `plans/` before implementation starts.
  Spawn an Opus subagent (`Agent` tool, `model: "opus"`) whose only job is writing it. No plan means
  no implementing straight from spec. Sole exception: chapter close phase, done by one Sonnet
  implementer with no planner and no plan file.
- Once plan lands, spawn a Sonnet subagent (`model: "sonnet"`) to implement it. Default: one
  sequential subagent for whole phase. Fresh subagent starts cold: its prompt carries plan and any
  prior-phase context it needs; never assume it remembers anything.
- Parallel subagents only when plan's work is genuinely independent (unrelated adapters,
  non-overlapping fixes). Never split one continuous, order-dependent piece of work across them.

## Phase loop

- Per phase: check for plan, spawn Opus to write one if missing, spawn Sonnet implementer(s) for
  whole phase, wait. Phase is done only when planned, implemented and committed. Don't start next
  phase's plan before then.
- Execution order is `SPEC.md` table order, top to bottom, one phase at a time, unless user
  explicitly says otherwise for that phase. Row order is not a suggestion to weigh against
  convenience, in-flight subagent scope or perceived urgency. Never skip ahead or double back unasked.
- Batch, only on user request: up to 3 phases concurrently, each its own stream (own worktree, own
  Opus plan, own Sonnet implementer). Batch only phases with no ordering dependency and at most small
  overlap (see Streams). Prefer batching whole phases over splitting one phase into parts. Batch
  grouping lives in a plan file under `plans/`.
- Multiple passes/iterations/rounds means repeat whole loop that many times; extras are not
  optional. Each pass plans against current tree (on top of what previous pass landed, never
  pre-phase state), re-reads current source instead of trusting previous pass's summary, and gets its
  own plan file (phase's plan name plus `-iter2`/`-iter3` suffix).
- Any pass or review round finding nothing real says so plainly. Never manufacture a finding.
- Best practices, no shortcuts: no stubbed error handling, no `TODO: fix later`, no skipped
  validation to make something demo. Scope left out of a phase stays out entirely, never
  half-implemented.

## Phase numbering

- Agent deciding to split a phase keeps its number: rename row `<n> Part 1: …`, add `<n> Part 2: …`
  (`Part 3`, etc.) right after. Never `<n>` plus fresh `<n+1>`. Each part still gets own plan,
  implementation pass and result section. Only user may ask for a split into new, separate numbers.
- Table position and phase number stay in lockstep, no exceptions. Phase that must run earlier
  (late-found dependency, files a later phase would otherwise touch twice) takes earlier slot's
  number; every phase from there shifts up by one. Phase that must run later is renumbered to its new
  position (highest number if moved to end). Never a gap, never a number contradicting position.
  Update every cross-reference in same pass (other rows' prose, not only titles).

## Streams

- Genuinely independent split runs as named streams (Stream A, Stream B, …), each its own git
  worktree off same base commit. Pre-commit hook checks whole tree, so a shared checkout fails
  implementers' commits on each other's half-done edits.
- Plan states split explicitly: per-stream file-ownership table naming each file's owner, plus
  confirmation of no ordering dependency between streams.
- Small overlap allowed: a few shared files (registries, i18n catalogs, route tables, config lists)
  touched by more than one stream, each listed in table with each stream's edit, in additive hunks
  that rebase cleanly. Large or semantic overlap, or no confirmed absence of ordering dependency,
  means no split: one sequential implementer.
- Cap concurrent streams at 3.
- Landing: once every stream is implemented and independently verified, rebase each onto chapter
  branch in any order. Resolve a conflict in a listed shared file; stop on any other (ownership
  boundary was wrong). Remove worktrees, push.

## Verify before accepting

- Plan or implementation isn't done because a subagent says so. Before marking either complete,
  check result against full original ask: `SPEC.md` row's wording, user's request, every standing
  rule (library adoption, primitive replacement, scope boundaries). Hooks and tests passing is not
  enough.
- Verify with a real check (grep for actual usage, build, count against plan's own numbers), never
  by trusting subagent's result prose. "X now uses library Y" is confirmed by finding a real caller
  of Y, not by Y being present in repo.
- Anything short of full ask (narrowed scope nobody agreed to, requirement quietly dropped, library
  "adopted" only as unused scaffolding) goes back to a subagent: same phase, same number. Never
  accept partial result and move on. Never re-scope the ask down to match delivery; only user narrows
  their own request.
- Failing test, lint finding, typecheck error or any other code-quality/hook check gets fixed on the
  spot, pre-existing or not. "Pre-existing" skips only root-causing whether this phase caused it
  (confirm, e.g. `git diff --stat` against phase's start commit touches none of failing file), never
  the fix. Fix and commit it in same implementation pass. Only exception: fix needs work genuinely
  outside phase scope (different subsystem, real design decision). Then it becomes its own named
  follow-up phase in `SPEC.md`, not a line in a result section.
- `--no-verify` never means done. Pre-commit hook failure gets root-caused and fixed. `--no-verify`
  only buys time to commit mid-investigation. Hook must pass clean in a normal, non-bypassed commit
  before a phase or any task counts as done. Never report complete while a hook is red on any commit
  that will ship.

## Resumability

Any subagent or orchestrating session can halt anytime, mid-phase, without warning: crash, rate
limit, kill, context cutoff. That is normal operating condition, not an edge case. Everything must
resume from disk alone.

- State a later step needs (review findings, audit list, anything later subagent or orchestrator
  needs) never lives only in a subagent's conversation. Write it to a file as its own step, committed
  if tracked, before or as part of any handoff. Never hold it for a later subagent's closing summary.
- Commit each group of related work as it lands, never batched into one final commit.
- After interruption, what landed (commits, findings files, plans) shows exactly where work stopped.
  Recovery never means re-deriving work an interrupted subagent did but never wrote down.
- Worktree survives container restart: resume stopped stream from its last commit, never from
  scratch.
- Set limit-reset wakeup only when user says so, never on own initiative. Use `send_later` for reset
  time with "resume from `SPEC.md`, `plans/`, git log". Cancel it when done.

## Testing

- Implement whole plan first, then test once and fix what's found, fixes as follow-up commits.
  Commits still land incrementally for legible history; only timing of expensive verification
  changes, not correctness or commit granularity.
- Per phase: fast checks (typecheck, lint, build) per commit are fine. Implementer runs only own new
  or changed specs, and any expensive check (end-to-end/UI, real hardware) once near phase end.
- Per chapter: one full run of every non-UI test, after last implementation phase, before code
  review. Done only when every failure is fixed, committed and passing on rerun. After a fix, rerun
  only failing tests and those sharing its code. Timeout under load gets a lone rerun first. Rerun
  full suite a second time only when a fix touched shared code (package, fixture, config) that
  targeted reruns cannot cover; say so in Result.
- UI suites run in PR CI, not locally beyond implementer's own targeted specs.
- Unit tests exist only for advanced, complex or deeply nested logic. Default: no dedicated unit
  test. A test earns its keep only guarding something genuinely hard to get right: parser/splitter
  with several interacting rules, cursor/pagination boundary arithmetic, cache eviction/invalidation
  with interacting rules, crypto beyond encrypt-then-decrypt, concurrency (ordering, backpressure,
  cancellation, races), decision structure too large to hold in your head.
- Everything else gets nothing: CRUD round-trips (even integration-shaped), one/two-condition
  validation, required-field/enum guards, thin pass-through wrappers, constructors/builders, format
  round-trips with no edge case, single-bad-input-to-single-error paths, anything mostly restating a
  short function body. Single `if` guarding one obvious case isn't complexity. Torn between two
  similar tests: delete one. Applies going forward, not as retroactive cleanup.
- Measure only when a real, concrete question is at stake, not as default ritual. Real-hardware
  trace, CDP tracing or byte-for-byte bundle comparison earns its keep when a claimed fix,
  regression or cost can't be checked another way. Routine changes and declined options get a short
  honest estimate or plain read of code's/library's stated behavior. Skip a measurement that wouldn't
  change the decision.

## Chapter end

- One branch and one PR per chapter; no per-phase PRs.
- Every chapter ends same way, unasked: last implementation phase, one non-UI full run with fixes
  committed, 1 code review round (unless spec row says more), close phase, PR.

## Code review

- Code review (once a phase or batch is otherwise complete, on request) is one Opus subagent
  covering three dimensions: (1) architecture/structure/maintainability/security, (2) functional
  correctness and business logic, (3) performance and resource efficiency. It reports findings, never
  fixes.
- No separate planner step. Reviewer scopes own area at start (owned files, one-hop callers, what
  earlier rounds parked) and goes straight to findings.
- Scope: everything changed since last review session, not whole repo, not only latest chapter.
  Find that session in git (last commit of prior review's findings or fixes), diff from there, state
  base commit in findings file. Every round covers full diff since that same base. Commits by any
  fixer since base, earlier rounds' included, are unreviewed code.
- Reviewer writes findings to a file under `plans/` and commits it before any fixer starts. Then one
  sequential Sonnet subagent fixes every finding (parallel only for batches genuinely isolated from
  each other). Fixer commits per logical group of related findings (same file, module or root cause),
  not per finding.
- Repeat whole cycle for as many rounds as asked. No findings doc survives its round once fixed.
  Carry forward only a genuinely still-open item (see Known open items), never a running narrative.

## Code and dependencies

- Reach for an existing, well-maintained library before hand-rolling non-trivial infrastructure
  (parser, virtualizer, positioning engine, retry/backoff, similar). Hand-rolled earns its keep only
  against a real requirement no library meets. Name that requirement when declining a library, not
  merely that existing code works.
- Only fully open-source libraries: no community edition of a dual-licensed product, no
  non-commercial-only tier, no functionality gated behind paid/Enterprise tier. Check license at
  package level *and* for the specific feature used, not only headline badge. Applies to every new
  dependency.
- Comments: very concise, only where code can't say it (non-obvious *why*, constraint, workaround).
  Never restate what code shows.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/): `feat:`,
  `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, with `!` or `BREAKING CHANGE:` footer for breaking
  changes.

## Docs upkeep

- This file and `CLAUDE.md` hold process only: how team works. App facts (driver choices, protocol
  constraints, capability quirks, known open limitations) live in architecture doc. How to build, run
  and test in current sandbox (credential constraints, container quirks, per-subsystem setup) lives in
  dev-environment doc. Never duplicate either here.
- Keep both process files lean: prune as you go, don't only append. Phase or review discovery goes in
  that phase's plan doc under `plans/`; plan doc and commit log are the durable record, never a
  "findings" section here. Before adding a bullet, check it is a standing rule for how team works, not
  a one-off result or app/environment fact. When touching a process file, remove what went stale too
  (pointer to deleted file/subsystem, question a later phase resolved).
- Architecture doc keeps a "Known open items" section for real, currently-true limitations, not
  TODOs or wishlist. Keep an entry only while genuinely open; delete it the moment it's resolved,
  never mark done in place.
