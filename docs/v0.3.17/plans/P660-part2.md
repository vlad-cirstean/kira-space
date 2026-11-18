# P660 Part 2 Git review panel state across module switches

Spec row: `../SPEC.md` P660 Part 2. Continues P660 Part 1 (chapter v0.3.16, history
`docs/history/07-v0.3.15.md` "v0.3.16 P660 Part 1"). Base: `chapter/v0.3.17` at `692fda904`
(P672 Parts 1-9 Done). Every fact below was checked at `692fda904` via `codegraph_explore` and file
reads. Line numbers are hints: grep the named symbol before editing.

Ask: after switching module away and back, the Git Review segment restores in-flight review, file
filter and scroll. Fix changes git-ui's mount contract. Own UI spec, same rules as Part 1.

## Root cause

- Module switch unmounts the Git panel. `WorkbenchShell.vue` renders `<component
  :is="activeModePanel" />`, so `GitPanel.vue` unmounts on every module switch (Part 1 premise).
  `reviewActivatedRepoIds` (`GitPanel.vue:90`) is component state and resets with it; on return the
  immediate `watch(tab)` re-adds the repo and `RepoReviewView.vue` mounts again.
- `RepoReviewView.vue` `onUnmounted` calls `handle.unmount()`, which unmounts git-ui's own Vue app
  (`packages/git-ui/src/main.ts` `mount`: `createApp(MountRoot)`, no `app.use(pinia)`).
- `ReviewView.vue` holds its view state in component refs and per-mount state objects, all rebuilt
  by `bootstrap()`:
  - `filterVisible = ref(false)` (`:257`), `filter = ref('')` (`:260`);
  - `usePanelHeaderSearch(rootEl, { stateKey: null, ... })` (`:263`), so `showSearch` is a local ref
    (`panelSearch.ts:80`, comment "git-ui has no Pinia, P660 Part 2");
  - selected file: `ReviewFilesState.selectedPath` (`state/reviewFiles.ts`), new per bootstrap;
  - expanded commit rows: `ReviewSessionState.expandedShas` (`state/review.ts:86`), new per bootstrap;
  - scroll: `ReviewCommitList.vue` `rowsEl` (`useVirtualRows` with no `keepScroll`), `FileTree.vue`
    `treeEl`, `ReviewCommentsPane.vue` `listEl`; none kept.
- The mount gets `viewState: new NullViewStateStore()` (`RepoReviewView.vue:44`). `ViewStateStore`
  is the graph's `PersistedViewState` v8 shape (`state/viewState.ts:54`), unfit for review state.
- Durable session resume exists but is narrow. `persistSession`/`resumeSession` (`ReviewView.vue:185-240`)
  store `ReviewSessionSnapshot` (`branch`, `baseOverride`, `pane`, `listMode`, `filter`, `diffMode`;
  `git-ipc/src/contract.ts:24`) through `review.session.save/load`, host-local in
  `repo/git/hostHandlers.ts:370` into the pinned graph tab (`repo/git/reviewSession.ts`). Resume is
  async (repo.list, session.load, resolveBase) and restores no filter bar visibility, selection,
  expansion or scroll. So the comparison comes back, the reviewer's place in it does not, and the
  restored `filter` text applies with its field hidden.
- git-ui cannot call the host's `useViewStateStore` directly: no Pinia is installed on its app. It
  works today only through Pinia's global `activePinia` fallback (how `provideShellServices`'s
  `useConfirmDialogStore()` resolves), which is a hidden coupling the SPEC asks to replace with a
  mount-contract seam. Part 1 therefore left both git-ui `usePanelHeaderSearch` callers on
  `stateKey: null` with U109 PERMANENT lines (`scripts/check-ui-primitives.sh:149-150`).

Step 0 of C3 (below) confirms this at base before the fix: the new spec runs red against the
unchanged review code and the implementer records which assertions fail.

## Design

### Where state lives

Host Pinia `useViewStateStore` (Part 1, `packages/workbench/src/state/viewState.ts`), reached from
git-ui through an injected interface. No new store (one store, one concern: view state of unmounted
surfaces is exactly that store's concern). No KeepAlive (Part 1 reason holds). The durable
`ReviewSessionSnapshot` stays as is: it is the cold-start (app restart) path and keeps branch, base,
pane, list mode and diff mode; this Part adds the session-only rest.

### Workbench seam (`packages/workbench`)

`state/viewState.ts` adds, beside the store:

```ts
/** What a kept-state composable needs; the store satisfies it. Provided to a git-ui mount (P660). */
export interface KeptViewState {
  read<T>(key: string, fallback: T): T;
  write(key: string, value: unknown): void;
  scroll(key: string): KeptScroll | undefined;
  setScroll(key: string, pos: KeptScroll): void;
  drop(prefix: string): void;
}
export const KEPT_VIEW_STATE_KEY: InjectionKey<KeptViewState> = Symbol('keptViewState');
```

`behaviour/viewState.ts`: one private resolver used by `useKeptState`, `useKeptScroll`,
`useKeptEditorScroll`:

```ts
// A git-ui mount is its own Vue app without Pinia; its host provides the store (P660 Part 2).
function keptViewState(): KeptViewState {
  return (hasInjectionContext() ? inject(KEPT_VIEW_STATE_KEY, null) : null) ?? useViewStateStore();
}
```

`useKeptScroll`'s `el` param widens to `Readonly<Ref<HTMLElement | null | undefined>>` (it only
reads; lets a `useTemplateRef` pass). Space and Studio callers provide nothing, so they keep the
store exactly as today (26 `useKeptState` and 30 `useKeptScroll` callers, no behaviour change).

### git-ui mount contract (`packages/git-ui/src/main.ts`)

`MountOptions` gains:

```ts
/** Review view only: keeps filter, selection, expanded rows and scroll across an unmount
 *  (P660 Part 2). Every entry key starts with `key`; the host drops that prefix. */
readonly keep?: { readonly state: KeptViewState; readonly key: string };
```

`mount` destructures `keep` out of `rest` (never a root prop) and, for `view === 'review'` with
`keep`, calls `app.provide(KEPT_VIEW_STATE_KEY, keep.state)` and `app.provide(REVIEW_KEEP_KEY,
keep.key)`. `viewState` stays required and graph-shaped; review hosts keep passing
`NullViewStateStore`. ADE review window (`AdeReviewFiles.vue`, `lockTarget`) passes no `keep` and
behaves as today (its own window, no module switch).

Cross-app reactivity: git-ui is bundled into Space (`loadGitUi` dynamic chunk, one `vue` copy), so a
store `reactive` read inside a git-ui computed tracks normally.

### git-ui keep helpers: new `packages/git-ui/src/lib/reviewKeep.ts`

- `REVIEW_KEEP_KEY: InjectionKey<string>`.
- `keptScrollIf(key: MaybeRefOrGetter<string | null>, el, opts?)`: calls workbench `useKeptScroll`
  only when `inject(KEPT_VIEW_STATE_KEY, null)` is non-null; otherwise a no-op. Generic components
  (`FileTree`, `ReviewCommitList`, `ReviewCommentsPane`) use it, so the graph view and the ADE window
  never resolve a store.
- `useReviewKeep({ review, reviewFiles })` (args are `ReviewView`'s two `shallowRef`s). Returns
  `{ filter, filterVisible, stateKey, scrollKeys, forgetBranch }`:
  - `prefix = inject(REVIEW_KEEP_KEY, null)`; `kept = inject(KEPT_VIEW_STATE_KEY, null)`.
  - `filter`: `useKeptState(`${prefix}:filter`, '')` when `prefix`, else `ref('')`.
  - `filterVisible`: same with `${prefix}:filterVisible`, fallback `false`.
  - `stateKey`: `prefix` (null without keep) for `usePanelHeaderSearch`, giving
    `${prefix}:showSearch`.
  - `branchKey = computed(() => prefix && branch ? `${prefix}:b:${branch}` : null)`.
  - `scrollKeys = computed(() => branchKey ? { commits, files, comments } : null)` with keys
    `${branchKey}:commits|files|comments`.
  - Expanded rows, write: `watch(review.expandedShas)` writes `[...shas]` to `${branchKey}:expanded`
    only when `phase === 'listing'` and this key's restore already ran (guards the clear in
    `setTarget`/`#clearExpansions`, which lands in the same tick as `phase = 'resolving'`).
  - Expanded rows, restore: `watch([branchKey, phase, loadedRows])`; once per `branchKey`, when
    `phase === 'listing'` and `loadedRows > 0`: for each kept sha among loaded rows
    (`store.shaAt(i)`, `i < loadedRows`) call `review.expand(sha)`; mark the key restored.
  - Selected file, write: `watch(reviewFiles.selectedPath)` writes non-null paths to
    `${branchKey}:selected`, only after this key's selection restore ran.
  - Selected file, restore: once per `branchKey`, when `reviewFiles.files.length > 0`: if
    `selectedPath` is null and the kept path is in `files`, call `reviewFiles.followFile(path)`
    (moves selection, loads diff metadata, never opens an editor; the diff tab is already open in
    the main area). Mark restored.
  - `forgetBranch()`: `kept?.drop(`${branchKey}:`)`; used by "back to branch selection".
  - All watchers created only when `prefix && kept`; without keep the composable returns plain refs
    and `null` keys (no store touched).

Restore runs after the durable resume: remount, `bootstrap`, `resumeSession`, `applyTarget`,
listing, then kept restore. Filter, its bar and `showSearch` restore synchronously at setup, so
`usePanelHeaderSearch`'s "never a hidden active filter" (`panelSearch.ts:83`) also holds.

### Key scheme and cap

Prefix `git:review:<codeRepoId>` (Part 1 shape `<mode>:<surface>[:<id>]`; `codeRepoId`, the
workspace key, not git's `repoId`). Per repo: `:filter`, `:filterVisible`, `:showSearch`. Per branch:
`:b:<branch>:selected|expanded|commits|files|comments`. Git ref names cannot hold `:`, so a branch
never splits a key; the `b:` segment keeps branch keys apart from the per-repo ones for `drop`.

400-key LRU cap: 3 keys per repo plus 5 per reviewed branch. A session with 10 repos and 5 branches
each holds 280 keys from review alone in the worst case, but the cap is shared with every other
surface, so eviction of a stale branch is expected and harmless: an evicted entry restores cold
(durable resume still brings branch, pane, filter text back). Scroll writes `touch` per event (Part 1
behaviour), which keeps the active branch's keys hot.

### Cleanup

- Workspace close: `state/workspace.ts` `closeRepoWorkspace` adds
  `useViewStateStore().drop(`${reviewKeepKey(repoId)}:`)` beside `dropRepoSearch`.
- Back to branch selection (explicit leave, D11b): `ReviewView.goBackToSelection` calls
  `forgetBranch()` before `clearTarget()`, so re-picking the branch starts clean, matching the
  durable session it already clears.
- Branch change in place (picker, `review.target` push, swap): no drop; the old branch's keys stay
  until LRU or workspace close, and returning to that branch restores its place.
- Repo change: another workspace has another prefix; `GitPanel` keys `RepoReviewView` by `repoId`.

### Not kept (reason)

- Roving focus row in commits and folder collapse in Files tree: not named by SPEC; focus is not
  restored on module return anyway. `FileTree.vue` sits at 584/600 lines (N10).
- Working-changes tree scroll (`review-working-tree`): Git sidebar passes no `workingChanges`.
- Rows beyond the first loaded page: restore expands and scrolls only within loaded rows; a scroll
  past them clamps. Load-more count is not kept.

## Files and changes

1. `packages/workbench/src/state/viewState.ts`: `KeptViewState`, `KEPT_VIEW_STATE_KEY` (above).
2. `packages/workbench/src/behaviour/viewState.ts`: `keptViewState()` resolver in the three
   composables; widen `useKeptScroll` `el` type.
3. `packages/workbench/src/behaviour/panelSearch.ts`: `stateKey` doc comment (`:73`) becomes "Null
   keeps it a local ref (a git-ui mount without kept view state)". Logic unchanged.
4. `packages/git-ui/src/lib/reviewKeep.ts` (new): `REVIEW_KEEP_KEY`, `keptScrollIf`,
   `useReviewKeep`.
5. `packages/git-ui/src/main.ts`: `keep` option, doc comment, provides (above).
6. `packages/git-ui/src/components/review/ReviewView.vue` (599 lines, N10 cap 600):
   - header doc (`:9`): replace "Persists nothing itself (...)" with "Keeps view state through the
     host's `keep` (lib/reviewKeep.ts); `viewState` is accepted only to share `MountOptions`".
   - replace `filterVisible`/`filter` refs (`:256-260`) with `useReviewKeep({ review, reviewFiles })`;
     `stateKey: null` becomes `stateKey`.
   - `goBackToSelection`: `forgetBranch()` first.
   - template: `:keep-scroll="scrollKeys?.commits ?? null"` on `ReviewCommitList`, `?.files` on
     `ReviewFilesPane`, `?.comments` on `ReviewCommentsPane`.
   - budget: net growth must stay at or under 600 after `bun run format`; shorten the
     `resumeSession` doc block (`:176-184`) to its first sentence plus the 14-day note to make room.
7. `packages/git-ui/src/components/review/ReviewCommitList.vue`: prop `keepScroll?: string | null`;
   `keptScrollIf(() => props.keepScroll ?? null, rowsEl, { ready: () => totalSize.value > 0 })`;
   add `data-testid="review-commit-rows"` on the `rowsEl` div if no spec selector exists for it.
8. `packages/git-ui/src/components/review/ReviewFilesPane.vue`: prop `keepScroll?: string | null`,
   passed to the committed `FileTree` as `:keep-scroll`.
9. `packages/git-ui/src/components/FileTree.vue` (584 lines): prop `keepScroll?: string | null`
   (doc: "Review Files pane only: view-state key of the tree scroll"), and
   `keptScrollIf(() => props.keepScroll ?? null, treeEl, { ready: () => visibleRows.value.length > 0 })`.
   `usePanelHeaderSearch` there stays `stateKey: null`: `filterInput` is a local echo of the
   `filter` prop, owned by the caller (`DetailState` in the graph, `ReviewView` in review).
10. `packages/git-ui/src/components/review/ReviewCommentsPane.vue`: prop `keepScroll?: string | null`;
    `keptScrollIf(() => props.keepScroll ?? null, listEl, { ready: () => groups.value.length > 0 })`.
11. `apps/kira-space/frontend/src/repo/git/reviewSession.ts`: `export const reviewKeepKey =
    (codeRepoId: string): string => `git:review:${codeRepoId}`;` with a one-line comment.
12. `apps/kira-space/frontend/src/repo/RepoReviewView.vue`: `keep: { state: useViewStateStore(), key:
    reviewKeepKey(props.repoId) }` in `mount`; rewrite the header comment (`:2-7`): graph-shaped
    `viewState` stays `NullViewStateStore`; module-switch view state goes through `keep`.
13. `apps/kira-space/frontend/src/state/workspace.ts`: drop on `closeRepoWorkspace` (above).
14. `apps/kira-space/frontend/src/repo/GitPanel.vue:83-89` comment: switching module now remounts too;
    durable resume plus kept view state restore it. Code unchanged.
15. `scripts/check-ui-primitives.sh`: remove the U109 PERMANENT line for `ReviewView.vue` (its
    `filter`/`filterVisible` refs are gone; a kept line would fail as stale). Reword the `FileTree.vue`
    line's reason to "filterInput echoes the filter prop; its owner keeps it". `RepoFileTree.vue`
    line unchanged. PERMANENT count 50 to 49.
16. `apps/kira-space/tests/ui/support/gitUiPortFixtures.ts`: export `reviewFile`; add
    `manyReviewFiles(n)` (`src/file-NN.ts`, kind `none`) for a scrollable Files tree.
17. `apps/kira-space/tests/ui/review-module-state.spec.ts` (new), see Test split.
18. `docs/ARCHITECTURE.md`: see C4.
19. `docs/v0.3.17/SPEC.md`: row Done, Result (C5).

Frontend rules: no new UI surface, no markup or class change beyond one test id. Pinia: reuses the
one view-state store. VueUse: `useKeptScroll` already wraps `useEventListener`. TanStack Query: no
server state involved (review data stays in git-ui's bridge states). shadcn-vue: no primitive
involved. All components stay `<script setup lang="ts">`.

## Stream decision

One sequential implementer. The workbench seam, the git-ui contract, the host wiring and the spec
form one order-dependent chain; the guard edit needs the ReviewView change. No split.

## Commits

Order: C1 seam, C2 git-ui plus host plus guard line, C3 spec (any `fix(test)` it forces lands
before it), C4 ARCHITECTURE, C5 SPEC. Run `bash scripts/check-ui-primitives.sh`, `bun run lint`, `bun run typecheck` before every
commit. Commit messages end with the session trailer lines.

### C1 `refactor(workbench): kept view state through an injectable seam`

Files 1-3. No behaviour change for Space or Studio.

### C2 `feat(git-ui): keep review state across module switches (P660 Part 2)`

Files 4-15. The ReviewView PERMANENT line goes in this commit (the hook checks the whole tree, and
the line turns stale here). Recount `ReviewView.vue` and `FileTree.vue` after `bun run format`.

### C3 `test(ui): review state survives a module round trip`

Step 0 first: write the spec before the C2 edits (or with them stashed), run it against base, note
which assertions fail (expected: filter bar, selection, expansion, scroll; branch, pane and
filter text may already pass through the durable resume). Then run it green on C2. Files 16-17.

### C4 `docs: architecture for P660 Part 2`

- "Code review, ported natively (C11)" `:2078` "ReviewView.vue ... persists no per-mount viewState
  (it takes NullViewStateStore)" becomes: takes `NullViewStateStore` for the graph-shaped
  `viewState`; filter, filter bar, selection, expanded rows and pane scrolls survive an unmount
  through `MountOptions.keep` (host `useViewStateStore` via `KEPT_VIEW_STATE_KEY`), keys
  `git:review:<codeRepoId>` plus `:b:<branch>:<surface>`, dropped on workspace close and on back to
  branch selection.
- `:2079` GitPanel bullet: switching module remounts the review too.
- P660 bullet (`:1163-1168`): add "git-ui review mount reads the store through
  `KEPT_VIEW_STATE_KEY` (its app has no Pinia)".
- `:1437` "keeps 50 named PERMANENT lines" becomes 49.

### C5 `docs: P660 Part 2 done`

SPEC row Done; Result under the P660 Part 2 section: what is kept, key scheme, cleanup, Step 0
base findings, spec counts, guard change, files count, runs.

## Test split

Frontend-only feature: the kept state lives in the renderer's session-only Pinia store, no IPC,
Go or persisted-schema change (`review.session.*` untouched, answered host-locally anyway). The
CLAUDE.md split is "at the IPC boundary"; nothing crosses it, so no flow-test half, as in Part 1. No
unit test: the restore guards are a few one-condition checks; the UI spec covers them end to end.

`apps/kira-space/tests/ui/review-module-state.spec.ts`, all through `openReviewListing(relaunch,
{ chunks: oneChunk(manyRows(60)), results: { 'review.files': { ...many files } } })`, viewport
1200x520, `roundTrip(page, 'automations', 'git')` from `support/moduleRoundTrip.ts`:

1. "review place, filter and scroll survive a module round trip": expand a commit row near the top
   (`review-row-header` click, `aria-expanded="true"`), scroll `review-commit-rows` (poll
   `scrollTop > 0`), switch to Files (`review-pane-files`), click a file row (`file-tree-row`,
   `aria-selected="true"`), scroll `[data-testid="review-files-tree"] [role="tree"]`, open the filter
   (`review-filter-toggle`) and type a filter every file matches (`src/`). Round trip. Assert:
   `review-branch-name` shows `REVIEW_BRANCH`; Files pane active; `review-file-filter` visible with
   `src/`; same file row `aria-selected="true"`; files tree `scrollTop` equals the saved value
   (`expect.poll`); switch to Commits: list `scrollTop` equals saved; scroll to top, the row is
   `aria-expanded="true"`.
2. "back to branch selection forgets the branch's place": expand a row, select a file, click
   `review-back-button`, pick `REVIEW_BRANCH` again: row collapsed, no file `aria-selected="true"`,
   commit list `scrollTop` 0. Round trip: branch picker shows (durable session cleared, D11b).
3. "closing the workspace drops kept review state": set filter and selection, close the repo from
   the repo row context menu (Close; grep `RepoList.vue` menu ids for the item testid), reopen the
   repo, open Review, pick the branch: filter bar hidden, `review-filter-toggle` not pressed, no file
   selected.

Mutation check (not committed): with `keep` removed from `RepoReviewView.vue`, test 1 fails; report
in Result, as Part 1 did.

Also run: `module-state.spec.ts`, `panel-find.spec.ts`, `repo-review-interaction.spec.ts`,
`consistency-git.spec.ts`, `copy-coverage-git.spec.ts`, `list-keyboard-git.spec.ts`,
`ade-v2-review*.spec.ts` (ADE window mounts ReviewView without `keep`), `repo-workspace.spec.ts`.
Command: `bun run build:test`, then from repo root
`bunx playwright test --config=apps/kira-space/playwright.config.ts --project=ui <specs>`.

Known sandbox issue: `openReviewListing` failed in this sandbox in P665, P667 and P672 Part 2
("review branch list empty"), also at their base commits. The new spec depends on it, so it is in
scope now. Run `repo-review-interaction.spec.ts -g "clicking a file"` first. If it fails, root-cause
(refs.list fixture, review host mount, branch picker load) and fix in its own
`fix(test): ...` commit before C3. Only if the cause is proven sandbox-only (decisive log line in
Result) does verification fall to PR CI; never skip or quarantine the spec.

Sandbox lessons:

- Playwright click on a menu item inside Monaco needs `locator.hover()` first.
- reka radio and toggle arrow presses need `keyboard.press(key, { delay: 40 })`
  (`tests/ui/support/radioKeys.ts`); pane switch uses clicks, so not expected here.
- N17 (`scripts/check-conventions.ts`) forbids the `dialog` variant on a delete/discard label
  without a `conventions-allow N17` marker; this Part adds no button.
- N10 caps files at 600 lines: `ReviewView.vue` 599, `FileTree.vue` 584; recount after
  `bun run format`.
- `scripts/check-ui-primitives.sh` has no TEMPORARY mechanism any more; a new guard lands with zero
  offenders. This Part adds no guard; it removes one PERMANENT line.
- A timeout under load gets a lone rerun first.
- Scroll restore polls: `useKeptScroll` restores after `ready` (rows sized), so assert with
  `expect.poll`, never one read.

## Risks

- Restore after async resume: the panel shows the branch picker briefly before resume lands, as
  today. Kept restore waits for `listing`; a spec reading state too early flakes. Poll.
- `useReviewKeep` write guards: a missed guard writes the `setTarget` clear (empty set, null path)
  over the kept value before restore. Spec test 1 catches it; guard by `phase` and the per-key
  restored flag exactly as designed.
- Scroll restore on a hidden host: `RepoReviewView` mounts only when the Review tab shows
  (`reviewActivatedRepoIds` gate), so the element is laid out. If a later change mounts it hidden,
  `useKeptScroll` sets `scrollTop` on a `display: none` box and the restore is lost.
- Shared store from another Vue app: relies on one `vue` copy in the Space bundle (true today:
  `loadGitUi` imports the workspace package). A future separate git-ui bundle would break
  reactivity of `filter`.
- `FileTree` gains a review-only prop; the graph `DetailPane` passes none and `keptScrollIf` finds no
  provider there, so no behaviour change. Verify with `repo-graph*` specs if `FileTree` diff grows.
- Expanding restored rows fires one `commit.detail` per kept sha; bounded by what the user expanded.

## Out of scope

- ADE review window (`AdeReviewFiles.vue`): no module switch inside its window.
- Graph tab view state (`TabViewStateStore`, keepMounted): unchanged.
- Persisting kept state across restart: Part 1 store is session-only by design; durable resume
  stays the restart path.
- Roving focus row, folder collapse, working-tree scroll, load-more page count (see "Not kept").
- Any change to `ReviewSessionSnapshot`, `review.session.*` or Go.
