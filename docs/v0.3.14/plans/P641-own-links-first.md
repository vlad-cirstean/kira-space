# P641 Own links first

Spec row: on an ADE task, the task's own Jira and PR links sort first inside their group, before
links collected from children or other sources. Update link strip and any sorted list sharing the
order. User: "The task's own Jira and PR links should come first in their group."

Base commit `ee9e0d597`. Frontend-only phase. No Go, wire, contract or i18n change.

## Current behavior

Ordering is decided in one place: `linkGroups()` in
`apps/kira-space/frontend/src/ade/v2/links/linkGroups.ts:98`. It feeds `useTaskLinks`
(`links/useTaskLinks.ts:53`), which feeds `AdeLinkStrip.vue`. The strip is the only links list:
Details tab (`details/AdeDetailsTab.vue`), archived Details (`panel/AdeArchivedPanel.vue`), stage
dialog (`stage/AdeStageDialog.vue`, incl. orchestrator pass) and Run dialog (`run/AdeRunDialog.vue`)
all render it. `AdeLinkTarget.vue` builds one preview pill, no list. Board cards and task list show
no link list (grep `jira` in `plan/`: only `AdeArchivedFinder.vue` title text). So no second sorted
list shares the order.

Input order: Go `TaskArtifacts` (`internal/ade/artifacts.go:79`) merges sources, dedups by URL
(detected beats stored) and sorts by kind, `createdAt`, title (`mergeArtifacts`, line 127). Sources:
`taskLinkSource` (host link, source `task`), `repoSource` (per own branch row: change request source
`host`, branch and deployment source `git`), one `trackerSource` per tracker (the task's own Jira
only, `trackerRef` reads `JiraURL`/`JiraKey`), `storedSource` (rows `agent` or `user`).
`TaskBoardDeps.ArtifactSources` is empty in production. Nothing gathers child-task links: an
orchestrator's "children" links only reach it as stored rows its agent reports.

`linkGroups` today:

1. `main` = Jira slot (`task.jira`, origin `jira`), host slot (`task.hostUrl`, origin `host`), then
   detected change requests with source `host`, in Go order. `seen` holds their URLs.
2. `related` = every other artifact in Go order, minus `seen` URLs and minus an issue whose title or
   ref equals `jira.key`. Detected branches go to a trailing `branches` list (only with `branches`).
3. Concatenate, filter per category (`issue`, `changeRequest`, `misc`).

Slots already lead. Gaps where an own link does not lead its group:

- G1 Own PR known only as a stored row. `repoSource` misses the branch's change request when the
  host call fails or passes the 5 s source timeout (gap), when the lookup does not match the head
  (fork, renamed remote branch) or before host detection caught up. The agent-reported row
  (`report_artifacts`, source `agent`) then sorts among other stored change requests by `createdAt`,
  usually behind older user or orchestrator rows. The board already knows the branch's change request
  independently: `useChangeRequests()` (`readQueries.ts:42`, `ChangeRequestsResult.branches`, keyed
  by branch id; same cache the branch panels use).
- G2 Own Jira row the slot filter misses. Tracker row for the task's issue survives the key filter
  when the tracker answers a different key (issue moved project: `PAY-10` becomes `NEW-5`, URL
  differs too). It then sorts among stored issues by `createdAt`/title.
- G3 Order rule lives in two loops with different dedup bookkeeping (`main` adds to `seen`,
  `related` does not). Ownership is implicit in "which loop", not a named predicate. Rewrite as one
  pass with an explicit `isOwn`, so G1/G2 fit without a third loop.

## Rule

Within each category group, items are `own` first, then `others`, then detected branches (only when
`branches` is on; unchanged).

An item is `own` when any holds:

1. Jira slot (origin `jira`).
2. Host slot (origin `host`).
3. Detected (`id === ''`) and kind `changeRequest` or `issue`. Detected change requests come only
   from the task's own branch rows (`repoSource`) or its host link (`taskLinkSource`); detected
   issues only from `trackerSource` (task's own Jira) or the host link. So every detected row of
   these kinds is the task's own by construction. Covers G2.
4. Its normalized URL (`norm`: trim, strip trailing `/`) equals the URL of a change request the board
   reports for one of `task.branchIds` (`useChangeRequests().data.branches[id].url`). Covers G1 for
   stored rows (agent or user).

Everything else (stored rows not matching rule 4, detected deployments) is `others`.

Order inside `own`: Jira slot, host slot, then remaining own items in Go input order. Order inside
`others`: Go input order. Go order = kind, `createdAt`, title; inside one category kind is constant,
so effectively `createdAt` then title. Stable: nothing re-sorts; the partition keeps input order.

Dedup unchanged in effect: skip an artifact whose normalized URL is in `seen` (slot URLs plus every
URL already taken); skip an issue whose title or ref equals `jira.key`; skip detected branches
without `branches`. Adding every taken URL to `seen` is a no-op on real input (Go already dedups by
the same key) and keeps one rule.

Not covered, by design:

- Detected branches stay last, not own: spec scope is Jira and PR links.
- Agent finish reply list (`ListArtifacts`, Go order, no grouping): not a grouped list, out of scope.
- Archived task: board change requests cover live branches only; rules 1-3 still hold (archive
  snapshot change requests are detected, source `host`).

## Files

Touch:

- `apps/kira-space/frontend/src/ade/v2/links/linkGroups.ts`: one-pass partition, `isOwn`, new
  option `ownUrls`, doc comment states the rule.
- `apps/kira-space/frontend/src/ade/v2/links/useTaskLinks.ts`: read `useChangeRequests()`, pass
  `ownUrls`.
- `apps/kira-space/tests/ui/ade-v2-links-strip.spec.ts`: new order spec.
- `apps/kira-space/tests/unit/ade-v2-link-groups.spec.ts`: new unit spec (see Tests).
- `docs/ARCHITECTURE.md`: "Task artifacts (P409)" Frontend bullet (line ~2976), one sentence on
  order; Tests bullet (line ~2977) names the new unit spec.

Read only: `readQueries.ts`, `wire.ts` (`ChangeRequestsResult`), `tests/ui/support/adeV2.ts`,
`tests/ui/support/links.ts`, `tests/fixtures/ade-v2/change-requests.json`, `board.json`.

Shared with P642 (task list day styling: `plan/` components, theme tokens, visual specs,
screenshots): none of the code or test files. Shared with P640 (API module tab close): none.
Possible overlap: `docs/ARCHITECTURE.md` only, if P640 or P642 also edit it. Different sections
(Task artifacts vs API module vs task list), additive hunks, rebase cleanly.

Not touched: Go code, contracts under `tests/contract/`, fixtures, i18n catalogs, `AdeLinkStrip.vue`
(renders `groups` as given), visual baselines (existing fixtures keep their order, see Oracles).

## Implementer steps

1. `linkGroups.ts`:
   - Extend options: `opts: { branches?: boolean; ownUrls?: readonly string[] } = {}`.
   - Keep slot construction (Jira, host) as is. `own = [...slots]`, `seen` = slot URLs.
   - `const ownSet = new Set((opts.ownUrls ?? []).map(norm).filter(Boolean))`.
   - One loop over `artifacts` in input order:
     - skip detected branch without `opts.branches`;
     - skip when `a.url && seen.has(norm(a.url))`;
     - skip issue matching `jira.key` (title or ref), as today;
     - `if (a.url) seen.add(norm(a.url))`;
     - push `fromArtifact(a)` to `branches` (detected branch), else `own` when `isOwn(a, ownSet)`,
       else `others`.
   - `isOwn(a, ownSet)`: `(a.id === '' && (a.kind === 'changeRequest' || a.kind === 'issue')) ||
     (a.url !== '' && ownSet.has(norm(a.url)))`. Module-level function, not exported.
   - `all = [...own, ...others, ...branches]`; category filter as today.
   - Replace the doc comment (lines 95-97) with the rule in two or three lines: own (slots, detected
     change requests and issues, board change requests of the task's branches) first, then others,
     then detected branches; input order inside each.
2. `useTaskLinks.ts`:
   - Import `useChangeRequests` from `../readQueries`. Call it once in `useTaskLinks`.
   - `ownUrls` computed: `toValue(task).branchIds.map((id) => crs.data.value?.branches[id]?.url ??
     '').filter(Boolean)`. Pass it in the `linkGroups(...)` options next to `branches`.
   - No new store: server state through the existing TanStack query (CLAUDE.md frontend rule). Query
     key is shared, so no extra IPC call where the plan view already loaded it.
3. Unit spec `tests/unit/ade-v2-link-groups.spec.ts` (`bun:test`, imports
   `../../frontend/src/ade/v2/links/linkGroups`). Build a minimal `Task` and `Artifact` with local
   helpers (cast through `as Task` for unused fields, as `ade-v2-timeline.spec.ts` builders do).
   Cases, each asserting `items.map((i) => i.key or label)` per group:
   - A: change requests, input Go order `[stored user CR createdAt 1, stored agent CR createdAt 5 URL
     = board CR]`, `ownUrls` = that URL (trailing `/` on one side) gives `[agent CR, user CR]`.
   - B: same input without `ownUrls` gives input order (no rule 4, nothing else changes).
   - C: issues, Jira slot `PAY-1`; input `[stored user issue PAY-2 createdAt 0, detected tracker row
     ref NEW-5 createdAt 10]` gives `[jira, NEW-5, PAY-2]`. Tracker row with ref `PAY-1` dropped.
   - D: host slot CR plus detected host CR (other URL) plus detected host CR with slot URL: `[host,
     detected]`, duplicate dropped. Detected branch only with `branches: true`, always last in misc.
   Justification for a unit test: four interacting rules (URL dedup across slots and artifacts, Jira
   key filter, three own signals, branches-last switch) on one pass; the UI spec covers one visible
   path only.
4. UI spec in `tests/ui/ade-v2-links-strip.spec.ts`, test `own Jira and PR links lead their lines`:
   - Artifacts: `listed()` plus a stored agent change request `{ id: 'a_own', kind: 'changeRequest',
     title: 'Own PR', ref: '#77', url: 'https://github.com/acme/api/pull/77', source: 'agent',
     createdAt: 5 }` appended after `api!9` (Go order: same kind, later `createdAt`).
   - Change requests: `adeFixture('change-requests')` with `branches.d_alerts_api` set to a change
     request whose `url` is `https://github.com/acme/api/pull/77`; pass as
     `{ channel: IPC.adeTaskChangeRequests, response }` in `openPlan` extras (extras replace the
     default of the same channel, `adeV2Control` doc). If the plan model rejects a change request on
     a draft id, instead mutate the board through `adeBoard()` so `T_alerts` holds a named branch id
     present in the fixture; check before choosing.
   - `openDetails(page)`. Assert pill order by reading `line(page, cat).locator(pill testid)`
     `allTextContents()`:
     - `changeRequest`: first pill contains `Own PR`, second `api!9`.
     - `issue`: `PAY-130`, `PAY-10`, `PAY-12` (slot, detected tracker row, stored).
   - Same scenario, second assertion: Run dialog strip (`ade-run-links`) shows `Own PR` first in its
     change request line (one shared order; reuse the Run dialog setup of the existing test).
5. `docs/ARCHITECTURE.md`: in the Frontend bullet of "Task artifacts (P409)", after "data from
   `useTaskLinks` (`useTaskArtifacts` plus `linkGroups`)", add: "Inside each line the task's own
   links lead (Jira and host slots, detected change requests and issues, stored rows whose URL is a
   board change request of the task's branches), then others, in Go order (P641)." Tests bullet: add
   `tests/unit/ade-v2-link-groups.spec.ts`.
6. Commits (Conventional Commits): `feat: own Jira and PR links lead their link group` (steps 1-2),
   `test: own links first in link strip` (steps 3-4), `docs: link order in architecture` (step 5).

## Tests and IPC split

Ordering is decided frontend-only, over data the backend already returns. No backend behavior
changes, so no new flow test: the signals the rule reads are existing, already-tested contracts —
detected change request rows with id `''` and source `host` (`TestTaskArtifacts`, contract
`ade-artifacts`), tracker issue row (`TestDetailsLinks`, contract `ade-details-links`, the `PAY-10`
row), board `ChangeRequests.branches` by branch id (fixture `change-requests.json`, its backend
contract unchanged). The UI spec is the end-to-end half that changes; the unit spec guards the
rule matrix.

## Oracles

- Before the change, the new UI spec fails: change request line reads `api!9`, `Own PR` (Go order by
  `createdAt`). After: `Own PR`, `api!9`.
- Unit case A fails before (no `ownUrls` option, stored rows in input order). Case C fails before:
  old code keeps input order, `[jira, PAY-2, NEW-5]`.
- Existing specs unchanged and green: eight-link fixture order stays `PAY-130, PAY-10, PAY-12 | api!9
  | feat/x, Design spec, Runbook, wiki.example/x` (`PAY-10` is detected, now own by rule 3, same
  slot; `T_alerts` draft branches have no board change requests, so rule 4 matches nothing). Keyboard
  test (`ArrowRight` from `PAY-130` to `PAY-10`) holds. Visual `tests/visual/ade-links.spec.ts` and
  screenshot `branchesHandover.shot.ts` keep their order for the same reason; implementer confirms by
  grepping their artifact fixtures for a stored change request whose URL is a board change request of
  the shown task, refreshes nothing unless one exists (then PR CI refreshes).

## Verification

- `bun run typecheck`, `bun run lint`.
- `bun test apps/kira-space/tests/unit/ade-v2-link-groups.spec.ts`.
- Targeted UI: `bun run build:test` then `playwright test --config=apps/kira-space/playwright.config.ts
  --project=ui apps/kira-space/tests/ui/ade-v2-links-strip.spec.ts
  apps/kira-space/tests/ui/ade-v2-details-links.spec.ts apps/kira-space/tests/ui/ade-v2-artifacts.spec.ts
  apps/kira-space/tests/ui/ade-v2-task-panel.spec.ts`, once near phase end. Disk is tight (about 5 GB
  free); skip other suites.
- Orchestrator check: grep `linkGroups.ts` for `isOwn` and `ownUrls`; grep `useTaskLinks.ts` for a
  real `useChangeRequests()` call; run the new specs.
