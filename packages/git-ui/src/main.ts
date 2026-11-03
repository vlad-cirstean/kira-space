import type { Transport, UiActionKind } from '@kira/git-ipc';
import { i18n } from '@theme/i18n';
import { provideShellServices } from '@workbench/bootstrapShell';
import { createApp, shallowReactive, shallowRef, type App as VueApp } from 'vue';
import AppRoot from './App.vue';
import type { RepoHead } from './components/GitViewHead.vue';
import ReviewView from './components/review/ReviewView.vue';
import { GRAPH_VISIBLE_KEY } from './graphVisibility.ts';
import MountRoot from './MountRoot.vue';
import type { ReviewApi, ReviewTarget, ReviewWorkingSection } from './state/review.ts';
import type { DateFormat, ViewStateStore } from './state/viewState.ts';

export interface MountHandle {
  unmount(): void;
  /** `false` while backgrounded (host hidden but kept mounted) lets
   *  `CommitGrid.vue`'s own generation-triggered rebuild defer to a single catch-up on the next
   *  `true`, instead of paying that cost once per missed bump against a grid nobody can see. Only
   *  meaningful for `view: "graph"` — a no-op call for a `"review"` mount, which has no
   *  `CommitGrid.vue` at all. Optional so every existing caller (this
   *  package's own tests) that never calls it keeps today's always-visible behavior exactly. See
   *  `graphVisibility.ts`. */
  setVisible?(visible: boolean): void;
  /** Pushes new `dateFormat`/`repoHead` into the mounted view; an omitted key is left as is. */
  update?(next: { dateFormat?: DateFormat; repoHead?: RepoHead }): void;
  /** Review view only: the uncommitted-changes section under the Files list; `null` hides it. */
  setWorkingChanges?(section: ReviewWorkingSection | null): void;
}

export interface MountOptions {
  readonly transport: Transport;
  readonly viewState: ViewStateStore;
  /** `docs/plans/P7.md` W9: which root to mount — `AppRoot` (the panel, P0-P6) or `ReviewView`
   *  (the sidebar, §6.8). Defaults to `"graph"` so every pre-P7 call site (and every test that
   *  constructs `MountOptions` without this field) keeps mounting exactly what it always did. */
  readonly view?: 'graph' | 'review';
  /** Only meaningful when `view === "review"` — the cold-bootstrap arm of D40's "how the view
   *  learns which branch to review" (see `ReviewView.vue`'s own doc comment). `undefined`/`null`
   *  is the "no branch yet" state; ignored entirely for `view: "graph"`. */
  readonly target?: ReviewTarget | null;
  /** Only meaningful when `view === "review"` with a `target`: the view stays on it. No back,
   *  swap or base pick, no `review.target` push, no session resume or save. */
  readonly lockTarget?: boolean;
  /** Only meaningful when `view === "review"`: receives a handle that opens a file's diff, as a click in the Files pane does. */
  readonly onReviewApi?: (api: ReviewApi) => void;
  /** Only meaningful when `view === "review"`: binds Alt+F8 / Shift+Alt+F8 to the next / previous
   *  file to review. Off for sidebars that share a window with other mounts. */
  readonly reviewShortcuts?: boolean;
  /** Only meaningful when `view === "graph"` — the exact mirror of `target` above, for a
   *  palette command that fired while the graph webview was cold (`panelView.ts`'s own
   *  `#pendingUiAction`/bootstrap-island arm). `undefined`/`null` means no action is pending.
   *  Mirrors `ui.action`'s own shape. */
  readonly pendingUiAction?: {
    action: UiActionKind;
    target?: { repoId: string; sha: string };
  } | null;
  /** Only meaningful when `view === "graph"` — Kira Space's own app-wide
   *  `appearance.dateFormat` (`packages/shared/domain/settings.ts`), read once at mount time and
   *  preferred over `PersistedViewState.dateFormat` when present (`App.vue`'s own `bootstrap()`).
   *  `undefined`/absent keeps today's behaviour: `PersistedViewState`'s own stored value, or its
   *  `'relative'` default — the shape every call site (and every test that constructs
   *  `MountOptions` without this field) already gets for free. The value at mount; a later change
   *  to the app-wide setting reaches a mounted graph through `MountHandle.update`. */
  readonly dateFormat?: DateFormat;
  /** P173: only meaningful when `view === "graph"` — opens the host's Operations log. Absent where
   *  the host has none; the failure banner then names it in text instead. */
  readonly onShowOperations?: () => void;
  /** Repo identity for the view head above the toolbar (graph and review views). */
  readonly repoHead?: RepoHead;
}

/**
 * Mounts the app shell into `container`, wired to `transport` and `viewState`, Kira Space's mounts
 * call this rather than each owning their own bootstrap. `viewState` keeps
 * scroll/selection/loaded-row state across an unmount. `view` is one build, one entry — the host's
 * own injected initial state says which root this call mounts, never a second bundle.
 */
export function mount(container: Element, opts: MountOptions): MountHandle {
  // §5.1 perf budgets are measured from navigation start (the implicit start of a
  // timeOrigin-relative measure); this marks the point the app's own bundle has parsed
  // and begun mounting. App.vue/ReviewView.vue each mark first-paint once mounted (W18 needs it
  // from both roots).
  performance.mark('kira:page-parsed');
  performance.measure('kira:page-parsed', undefined, 'kira:page-parsed');

  const {
    view = 'graph',
    target,
    lockTarget,
    onReviewApi,
    reviewShortcuts,
    pendingUiAction,
    dateFormat,
    onShowOperations,
    repoHead,
    ...rest
  } = opts;
  // P131 Part 2 §3.5: MountRoot wraps whichever root this mounts in the one TooltipProvider every
  // Tooltip/TooltipTrigger/TooltipContent trio in this package needs — git-ui is its own Vue app,
  // so neither host's own <App.vue> TooltipProvider reaches it.
  // Shallow: `transport`/`viewState` must not be proxied; `update` swaps top-level props.
  const rootProps = shallowReactive<Record<string, unknown>>(
    view === 'review'
      ? { ...rest, target, lockTarget, onReviewApi, reviewShortcuts, repoHead }
      : { ...rest, pendingUiAction, dateFormat, showOperations: onShowOperations, repoHead },
  );
  const app: VueApp = createApp(MountRoot, {
    root: view === 'review' ? ReviewView : AppRoot,
    rootProps,
  });
  // P79 review fix: scoped to this one app instance, not module-level — several repo workspaces'
  // graphs can be mounted (and independently backgrounded) at once. A no-op provide for a
  // `"review"` mount (no CommitGrid.vue there to read it) is harmless.
  app.use(i18n);
  // Dirty dialogs ask through the host's confirm store (git-ui has no Pinia of its own).
  provideShellServices(app);
  const graphVisible = shallowRef(true);
  app.provide(GRAPH_VISIBLE_KEY, graphVisible);
  container.classList.add('h-full', 'w-full', 'overflow-hidden');
  app.mount(container);
  return {
    unmount(): void {
      app.unmount();
      container.classList.remove('h-full', 'w-full', 'overflow-hidden');
    },
    setVisible(visible: boolean): void {
      graphVisible.value = visible;
    },
    setWorkingChanges(section): void {
      if (view === 'review') rootProps.workingChanges = section;
    },
    update(next): void {
      if (next.repoHead !== undefined) rootProps.repoHead = next.repoHead;
      if (next.dateFormat !== undefined && view !== 'review')
        rootProps.dateFormat = next.dateFormat;
    },
  };
}
