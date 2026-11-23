# v0.3.24

Chapter v0.3.24, branch `chapter/v0.3.24`, tag `v0.3.24`. First phase `P736` (highest phase: `P735`,
in `docs/history/07-v0.3.15.md`, chapter v0.3.23).

Code review at chapter end: 1 round.

| # | Phase | Status |
|---|-------|--------|
| P736 | Proof data: auto-expanded, fits the Kira viewport, small screenshots, selectable text | Done |
| P737 | Details panel: denser items, "see more" for long content | Done |
| P738 | Task list and detail: border colors, spacing, stronger selected marking | Done |
| P739 | Code review window: speed for big PRs | Done |
| P740 | Code review window: collapsible uncommitted changes, readable removed files | Done |
| P741 | Code review window: correct diff colors after a file is marked reviewed | Done |
| P742 | Studio MCP tools: agent can force an approval prompt for an allowed operation | Done |
| P743 | Task links: show at once, Jira details after; PR tooltip format | Done |
| P744 | Adopt rtk (OSS edition) for agent shell output, like CodeGraph | Done |
| P745 | Kube MCP: unambiguous transform script globals in the tool description | Done |
| P746 | Chapter close | Todo |

## P736

All proof data in Kira is auto-expanded. Purpose: see details at once and fit the screenshots Kira
takes.

- Screenshots render to fit the Kira viewport and keep that viewport small so the result is easy to
  see. Preferred: shrink at capture time. If that is complex or error prone, keep the window size
  small enough instead. Plan states the choice and why.
- Text in the proof view and in the proof tab is selectable and copyable.

## P736 Result

Window kept small (1024x640) instead of shrinking at capture time; reasons in `plans/P736.md`. Proof
documents and nested fields open expanded, key-value rows grow to content (`autoExpand`). Proof view
and proof tab text selectable; tab diagram draws at full height, steps wrap, open action is an icon
button. Tabular cells stay one line (SlickGrid fixed row height). UI specs added; proofflow Go test
unchanged and green.

## P737

Details panel shrinks items so far less scrolling finds a thing. Long content collapses; "see more"
(or similar) expands it.

## P737 Result

New theme `ClampedContent` (first 6 lines, fade, Show more / Show less, no link when content fits;
ARCHITECTURE bullet). Notes and handovers clamp to 6 lines, audit statements to 2; expansion lives in
`adeDetailsList` store, survives tab switch. Dead "Show all" toggle on short statements gone.
Padding: note body `flush` (no double inset), chain cards 6px/8px, tighter gaps, Details tab `gap-2
px-3 py-2`. Proof entries untouched. Specs: two new in `ade-v2-notes`, `ade-v2-db-audit` reworked;
110 ade-v2 UI specs and adeflow Go tests pass.

## P738

- Task list: drop the colored top bar; the border takes that color.
- Task detail: drop the top border; move it below the task title.
- Drop the gap between the title and the task tabs.
- Plan tab: remove the several extra gaps.
- Selected task gets clearer marking than the white highlight alone, using the new borders.

## P738 Result

Cards and span rows: 1px border in the task colour (`--task-color`), top bar gone. Selected: grey
fill plus 2px task-colour outline flush with the border (no reflow); ripple cards get a dashed
border. Task and branch panels: colour line moved under the title (`AdePanelFrame` `color` prop,
`ColorLine under`), `DetailHeader` flush with `pb-0.75`, empty default-slot wrapper hidden (title to
tabs about 18px down to 7px). Plan gaps: same-day cards 8px apart (was 16px), drop area padding
and Plan body top padding trimmed. Specs updated: plan, task-row, task-panel (new branch-panel
line test), selection-colour; targeted UI run 96 passed. Visual baselines and screenshots update in
PR CI.

## P739

Code review window is slow for big PRs. Plan profiles first (diff load, tree, render), then fixes the
measured cause.

## P739 Result

Four measured causes fixed. (1) `FileTree` is a virtual window (`useTreeVirtualRows`, rows in
`useFileTreeRows.ts`): 37 rows and 492 DOM elements at 3000 files (was 500 rows, 4,480); the 500-row
cap stays only for the expanded commit row (`capped`, so its catalog key and cap announce stay).
(2) `review.files` drops `-C` (`RangeNameStatusArgs`): warm 209-247 to 20-27 ms. (3) `review.markMany`
writes chunks of 256 paths or 16 MiB in one `PutMany` transaction, pooled flate writers: 1506 files
2.2-8.2 s to 0.78-0.90 s (target 600 ms missed; remaining time is per-file git reads and compression).
(4) `ReviewFilesState.changes` keeps the tree input stable across marks. UI at 3000 files, best runs:
steady open 1,406-1,771 to 524-816 ms, first open 3.2-3.8 s to 1.4 s, filter 1.1 s to 0.35-0.45 s
(runs under load1 12-20 read up to 2x higher). Tests: flow `TestReviewFilesCopyIsAdded`,
`TestReviewMarkManyChunksAndConcurrentMark` (`-race`, three runs; whole gitflow package `-race`
green), UI "file tree renders a window of 1100 files" (replaces Show more) and "a 3000-file review
list renders a window and steps to a far row"; `ade-v2-review-follow` scrolls to rows below the
window first. 158 UI specs of the tree-touching files green. Numbers: `docs/PERF.md` section 2.18.

## P740

- Uncommitted changes section cannot be collapsed. Make it collapsible.
- Removed files show an error, or read unreadably. Find which; fix so they read properly.

## P740 Result

Not a backend error: removed files rendered unreadably. Monaco's diff against an empty side paints
the whole text as one char-level change plus a phantom added line and trailing removed line. A
deleted file (head found, worktree missing; committed and uncommitted) now mounts one read-only
editor of the old text with `.kira-diff-line-removed` tint (`useDiffEditor.isDeletion`, `deleted`,
`codeEditor`), a `Deleted` badge, no layout toggle, Go to file or review layer; a file restored or
deleted on disk re-mounts the right widget. Uncommitted section in `ReviewFilesPane.vue` is a
shadcn-vue Collapsible (`workingOpen`, reopens when the cursor moves onto a working file; closed
section shrinks to its header). Added files hit the same Monaco special case mirrored; left out of
scope. Tests: flow `TestDeletedFileReads`, `TestReviewWorking` (`#deleted` contract); UI
`ade-v2-review-deleted.spec.ts` (3), `ade-v2-review-working.spec.ts` (2 new). All pass; `ade-v2-review`
and `repo-workspace` UI specs rerun green.

## P741

Diff against the base colors correctly. After a file is marked reviewed and new changes land, the
shown changes are right but everything is green, new changes in a second shade ("added over added").
Fix so the incremental diff colors like a normal diff.

## P741 Result

Root cause: `paint()` in `reviewDecorations.ts` tinted every reviewed line of a partly reviewed file with
a whole-line `--kira-ok` green background, the same hue the diff editor uses for added lines. Since-review
sides and hunks were already right (flow test pins them). Fix: the coverage is a gutter rail
(`marginClassName`, `box-shadow: inset 3px 0 0 var(--kira-ok)` in `review-decorations.css`), never a content
background; overview ruler and the fully-reviewed skip unchanged. Screenshot confirms the rail shows left of
the line numbers and the right pane is neutral except `LINE2` and `line5`. Tests: flow `TestSinceReviewSides`
(contract `git-review-since`); UI `ade-v2-review-since.spec.ts` (5: since-review side by side and inline,
fully reviewed, uncommitted reviewed vs unreviewed, added file). `ade-v2-review`, `-working`, `-deleted`,
`-rereview` UI specs and `reviewflow` (`-count=2`) green. ARCHITECTURE review-layer bullet updated.

## P742

Studio MCP tools let the agent request an approval prompt even when the operation is allowed. Use:
agent shows the user a sensitive action that needs confirmation.

## P742 Result

Optional `confirm` string (trimmed, at most 300 runes, checked first) on `run_query` (and explain),
`browse_table`, `kube_read`, `run_script` start and `api_request`. It raises the approval card for an
allowed op (`Reason: confirm`) and never relaxes a refusal; a card the policy raises anyway keeps its
reason and shows the note. `ApprovalRequest.AgentNote` becomes `approvals.Item.agentNote`; `holdResult`
clears note and confirm reason for the response stage. `ApprovalItemCard` renders two generic risk
lines (`approval-agent-confirm`, `approval-agent-requested`), desktop and phone. Tests: flow
`confirm_test.go` (contract `approvals-confirm`) plus UI spec `approvals-confirm.spec.ts` (20 UI specs
of the touched files passed); `dbmcpflow`, `llmpromptsflow`, `dbmcp`, `llmprompts` green; real claude
`TestDbMcpConfirmThroughClaude` passes (3 of 3 runs after the prompt names the approval prompt, not
chat). Tool-list budget: 3990 before, 3992 after (five `confirm` args cost about 143 tokens, paid by
trimming duplicated default text); per the user's instruction `toolListTokenBudget` rose 4000 to 4020
(comment updated: aim is efficient texts, raise only by what a new arg needs). Prompt count 150 to 155.

## P743

- Links show immediately; Jira details fill in after. Today every link waits for Jira to load, which is
  slow. Links without Jira data render at once, Jira-backed fields (title, status) arrive when loaded.
- PR tooltip in the task view is formatted badly. Fix its layout.

## P743 Result

`trackerSource.Artifacts` returns the stored issue row only: no acli call, no tracker gap (wire type
unchanged). Jira title, status and reason come from `TaskIssue` (`useTaskIssue`). `AdeLinkStrip` no
longer wraps pills in `QueryState`: slot pills (Jira, host) show on the first frame, artifact pills
join when `Artifacts` answers, skeleton and empty state sit below. `useTaskLinks` marks the Jira item
`pending` (pulsing muted dot, same 8px slot, no width jump) and adds the tracker gap from the issue
query. PR tooltip body (button and tooltip variants) is a stacked, left-aligned column
(`pr-glyph-tip`); state line reads `Open into main`, reusing `space.hostView.header.branches.label`
(a new key failed catalog rule C4 as a near duplicate), so no `en.json` edit. Task-view PR glyph now
passes `baseRef`. Tests: flow `TestTaskLinksBeforeJira` (contract `ade-issue-tracker`, `#held`),
`TestTaskIssueTracker`, `TestTaskIssueFastLoading`, `TestTaskArtifacts` reworked; contracts (incl. `ade-details-links`, `ade-details-open`)
regenerated; UI specs issue-tracker, links-strip, artifacts, pr-glyph, main-branches: 48 passed.

## P744

Adopt [rtk](https://github.com/rtk-ai/rtk), a hook that filters shell command output before the agent
reads it, the way CodeGraph is adopted.

- Edition: `rtk-oss` (Apache-2.0 only). The default `rtk` edition bundles a BUSL-1.1 engine, which
  fails the open-source-only rule in `docs/WORKING_AGREEMENT.md`. Plan re-checks the license at
  package level and for each feature used.
- Plan decides, from the rtk docs and a `--dry-run`: project-level vs global hook config, what is
  committed (like `.mcp.json`), install via a script like `scripts/codegraph-setup.sh`, telemetry
  stays off, no pipelines or proxy.
- Plan checks interaction with repo rules: hooks and pre-push output, `git diff` in reviews (bypass
  with `rtk bypass`), subagent verification greps, the CLI-credentials rule.
- Doc the setup in `docs/DEV_ENVIRONMENT.md`; process rule in `CLAUDE.md` only if it is a standing
  rule.

## P744 Result

rtk-oss `1.2.1` (Apache-2.0) adopted. Pins re-verified against the downloaded release `checksums.txt`
(all four match the plan). New `scripts/rtk-setup.sh` (pinned archive SHA-256, archive must hold exactly
`rtk` and `LICENSE`, refuses a full-edition or foreign `rtk`, telemetry disabled and checked, no `rtk
init`), called from `prepare-dev-environment.sh`. Project-level hook: `.claude/settings.json`
`PreToolUse` (Bash) plus `env` (`RTK_TELEMETRY_DISABLED`, `RTK_SUPPRESS_HOOK_WARNING`), wrapper
`.claude/hooks/rtk-pretooluse.sh` fails open. `CLAUDE.md` `## rtk` (bypass rule, never-list),
`docs/DEV_ENVIRONMENT.md` "rtk" section. Diffs stay raw by the `rtk bypass` rule, no per-user config.

Checks (isolated `HOME`/`RTK_INSTALL_DIR` in the scratchpad):

- Setup twice: first install, second prints `already at`, same mtime and SHA-256 of the binary, exit 0
  also with `HTTPS_PROXY=http://127.0.0.1:9` (no download). `git status --porcelain` clean of side
  effects.
- Fake `rtk` printing `rtk 9.9.9` (accepts `pipeline`) and one printing `something else`: both exit 1
  with the not-oss message, fake untouched.
- Hook payload pipe: `git status` yields `"command":"rtk git status"`; `gh auth token` and a `PATH`
  without rtk print nothing, exit 0.
- `RTK_TELEMETRY_DISABLED=1 rtk telemetry status`: `enabled: no`, env override blocked.

Bytes, raw vs rtk (this worktree):

| Command | Raw | rtk |
| --- | --- | --- |
| `git status` | 65 | 47 |
| `git log -20` | 7,229 | 4,731 |
| `git diff` since phase base (lossy) | 25,847 | 13,286 |
| `go test -count=1 -v` (4 packages, 22 tests) | 2,402 | 33 |
| `bun run lint` (clean) | 1,125 | 1,125 |
| `go test` failing scratch module | 85 | 162 |

Failing `go test` keeps `Sum() = 2, want 3`, exit 1 (tiny output: rtk adds a header, so larger).

Not confirmed: live-session hook pickup. The implementer cannot see its own session reload hooks; it
stays unconfirmed until a spawned session (P745) runs `git status` and gets the rtk form.

## P745

Found while hardening a real `claude` test (review round 1 fixer): the kube MCP tool description
reportedly says transform scripts get globals `result` and "You get {output, input}", while `result`
is the output itself, so the model guessed `output`, `input`, `result.output` and failed 2 of 10 runs
of `TestKubeMcpKeptChainThroughClaude` until the test prompt spelled it out.

- Plan verifies the claim first (tool description text, token budget test, the script engine's real
  globals). If the text is correct, close the phase as no change with the evidence in its Result.
- If wrong or ambiguous: fix the description for every tool that shares the wording (DB, API, kube),
  keep it terse, keep the tool-list budget test passing, then rerun the kube real-`claude` test 10
  times with the original (pre-hardening) prompt to prove the description alone now suffices.

## P745 Result

Verdict: ambiguous, not wrong. Globals were correct; "You get {output, input}" read as more globals,
and kube "what op returns" was circular and gave no list shape.

- Texts changed (`internal/llmprompts/registry.go`): `run_query` `script`, `api_request` `script.transform`,
  `kube_read` `script.transform`. All say "You get output (its return value)"; kube also spells the
  list shape and drops "can set values for a later read" (`dbmcp.instructions.scripts` covers it).
- Tool-list tokens: 3992 before, 4016 after. Budget stays 4020 (kube text tightened, no raise).
  `budget.tsv` rune counts: 256, 248, 167.
- `TestKubeMcpKeptChainThroughClaude` with pre-hardening prompt (scratch, not committed): 9 of 10 before,
  10 of 10 after.
- Fixed text, once each: `TestDbMcpScriptThroughClaude`, `TestDbMcpKeptChainThroughClaude`,
  `TestKubeMcpThroughClaude`, `TestKubeMcpKeptChainThroughClaude` all pass.

## P746

Chapter close per `CLAUDE.md` "Closing a chapter", by one Sonnet implementer, no plan.
