# v0.3.24

Chapter v0.3.24, branch `chapter/v0.3.24`, tag `v0.3.24`. First phase `P736` (highest phase: `P735`,
in `docs/history/07-v0.3.15.md`, chapter v0.3.23).

Code review at chapter end: 1 round.

| # | Phase | Status |
|---|-------|--------|
| P736 | Proof data: auto-expanded, fits the Kira viewport, small screenshots, selectable text | Done |
| P737 | Details panel: denser items, "see more" for long content | Todo |
| P738 | Task list and detail: border colors, spacing, stronger selected marking | Todo |
| P739 | Code review window: speed for big PRs | Todo |
| P740 | Code review window: collapsible uncommitted changes, readable removed files | Todo |
| P741 | Code review window: correct diff colors after a file is marked reviewed | Todo |
| P742 | Studio MCP tools: agent can force an approval prompt for an allowed operation | Todo |
| P743 | Chapter close | Todo |

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

## P738

- Task list: drop the colored top bar; the border takes that color.
- Task detail: drop the top border; move it below the task title.
- Drop the gap between the title and the task tabs.
- Plan tab: remove the several extra gaps.
- Selected task gets clearer marking than the white highlight alone, using the new borders.

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

## P743

Chapter close per `CLAUDE.md` "Closing a chapter", by one Sonnet implementer, no plan.
