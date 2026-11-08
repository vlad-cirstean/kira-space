# v0.3.5 code review findings

Base: `f69087cde` (docs: drop v0.3.4 review round 1 findings), last commit of prior review session.
Diff `f69087cde..HEAD` covers v0.3.4 post-review fixes squashed into `df8bae79f` (batched `LatestRunsOf`
in `ListTasks`, `ChildModel` unexport) plus P575-P579 and fix commits `43090d43f`, `d36e07dde`.
One-hop callers read: `useLog`/`readSignals.onLog`, `AdeLogsRepo.Page`, `scriptruns.Repo.ReadLog`,
`ApiVariablesDialog` open watch, `fillChildStep`/`currentStepID`/`plan`, `LatestRuns` callers.

Counts: 0 high, 1 medium, 1 low.

## F1 (medium) ADE Copy log copies a partial log as the whole log

- Where: `packages/workbench/src/components/CopyLogButton.vue:7-14`; ADE caller
  `apps/kira-space/frontend/src/ade/v2/sessions/AdeHeadlessPane.vue:12-16,49-53`.
- Defect: button copies `log.data.chunks` from `useLog` cache. `useLog` reads one page
  (`afterSeq: 0`), and `AdeLogsRepo.Page` caps a page at `adeLogPageLimit = 500` chunks. Pushes then
  append only newer chunks (`readSignals.ts:49-53`). Run with more than 500 stored lines when pane
  opens: cache holds seq 1-500 plus later pushes. Copy drops everything between, with no marker.
  Agent runs exceed 500 lines routinely (store keeps 2 MiB tail, 8 KiB per line). Also when
  `truncated` is true (head dropped, ADE and script runs), copied text gives no sign of missing head.
  Component comment promises "whole log". Script-run copy (`RunLog.vue`) is complete apart from the
  head marker, since `ReadLog` limit equals `maxLogLines` (5000).
- Fix: on click, page through the log instead of reading the cache. Give `CopyLogButton` a
  `load: () => Promise<{ text: string }[]>` prop. ADE caller loops `reader.readLog({ kind, id,
  afterSeq })` from 0, advancing to `nextSeq`, until a page returns under 500 chunks. Script-run
  caller keeps cached chunks. When the page reports `truncated`, prefix the copied text with one
  line saying earlier output was dropped. Extend `ade-v2-sessions.spec.ts` with a log over 500
  chunks and assert copied line count.

## F2 (low) Share image alt and committed image drift from headline

- Where: `site/src/data/content.ts:33` (`SITE.shareAlt`), `site/src/assets/share/kira-share.webp`,
  `site/scripts/share-image.ts`.
- Defect: `shareAlt` hard-codes the headline text that `HERO.line1`/`HERO.line2` already hold.
  Committed WebP bakes the same text and the icon, regenerated only by hand
  (`bun run site:share-image`). Headline or icon change ships stale `og:image` and alt with no
  failing check; `check-icons.ts` covers favicon and touch icon only.
- Fix: build `shareAlt` from `APP`/`HERO` fields
  (`` `${HERO.eyebrow} icon with the headline: ${HERO.line1} ${HERO.line2}` ``). Have
  `share-image.ts` write a sidecar hash of its inputs (HTML string plus icon bytes) next to the
  WebP, and have `site:build` (or `check-icons.ts`) recompute and fail on mismatch.

No findings in: `LatestRunsOf`/`fillChildStep` batching, P576 title bar, P578 file bar placement and
`ConfigFileProblem` (incl. `VariableSetView` Show in YAML path through `apiVariablesDialog` watch),
P579 dialog surface token and scrim, site icon/favicon/button refactor, install copy button,
knip entries, `dbmcpstep_test.go` fix.
