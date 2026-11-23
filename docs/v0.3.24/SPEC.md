# v0.3.24

Chapter v0.3.24, branch `chapter/v0.3.24`, tag `v0.3.24`. First phase `P736` (highest phase: `P735`,
in `docs/history/07-v0.3.15.md`, chapter v0.3.23).

Code review at chapter end: 1 round.

| # | Phase | Status |
|---|-------|--------|
| P736 | Proof data: auto-expanded, fits the Kira viewport, small screenshots, selectable text | Done |
| P737 | Details panel: denser items, "see more" for long content | Done |
| P738 | Task list and detail: border colors, spacing, stronger selected marking | Done |
| P739 | Code review window: speed for big PRs | Todo |
| P740 | Code review window: collapsible uncommitted changes, readable removed files | Todo |
| P741 | Code review window: correct diff colors after a file is marked reviewed | Todo |
| P742 | Studio MCP tools: agent can force an approval prompt for an allowed operation | Done |
| P743 | Task links: show at once, Jira details after; PR tooltip format | Todo |
| P744 | Adopt rtk (OSS edition) for agent shell output, like CodeGraph | Todo |
| P745 | Chapter close | Todo |

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

## P740

- Uncommitted changes section cannot be collapsed. Make it collapsible.
- Removed files show an error, or read unreadably. Find which; fix so they read properly.

## P741

Diff against the base colors correctly. After a file is marked reviewed and new changes land, the
shown changes are right but everything is green, new changes in a second shade ("added over added").
Fix so the incremental diff colors like a normal diff.

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

## P745

Chapter close per `CLAUDE.md` "Closing a chapter", by one Sonnet implementer, no plan.
