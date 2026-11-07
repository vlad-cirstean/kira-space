<script setup lang="ts">
import type { ReviewComment, ReviewSessionSnapshot, Transport, UiActionKind } from '@kira/git-ipc';
import { TransportError } from '@kira/git-ipc';
import EmptyState from '@theme/components/EmptyState.vue';
import { Button } from '@theme/components/ui/button';
/**
 * The sidebar view's root, mounted by `main.ts` when `view === "review"`. States: no branch, ask
 * for a base, unrelated histories, nothing to review, the list, plus `resolving` and `error`.
 * Persists nothing itself (`viewState` is accepted only to share `MountOptions` with `AppRoot`).
 *
 * The branch arrives three ways: `props.target` (cold bootstrap), the `review.target` event (any
 * time), and the no-branch state's own picker.
 */
import { FieldError } from '@theme/components/ui/field';
import { type MessageKey, t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { useLiveRegion } from '@workbench/behaviour/liveRegion';
import { usePanelHeaderSearch } from '@workbench/behaviour/panelSearch';
import { errorText } from '@workbench/bridge/codedError';
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, useTemplateRef, watch } from 'vue';
import { BridgeClient } from '../../bridge/client.ts';
import { adjacentInOrder, orderedFilePaths } from '../../lib/fileTreeModel.ts';
import { markOnToggle, reviewAttention } from '../../lib/reviewAttention.ts';
import { createReviewFilesActions } from '../../lib/reviewFilesActions.ts';
import { useReviewAnnouncements } from '../../lib/useReviewAnnouncements.ts';
import { useReviewStepping } from '../../lib/useReviewStepping.ts';
import type { MountOptions } from '../../main.ts';
import { retryBootstrap as sharedRetryBootstrap } from '../../state/bootstrap.ts';
import type { FileListMode } from '../../state/detail.ts';
import type { DetailActions } from '../../state/detailActions.ts';
import { RefsState } from '../../state/refs.ts';
import {
  type ReviewPane,
  ReviewSessionState,
  type ReviewTarget,
  type ReviewWorkingSection,
} from '../../state/review.ts';
import { ReviewCommentsState } from '../../state/reviewComments.ts';
import { ReviewFilesState } from '../../state/reviewFiles.ts';
import type { ViewStateStore } from '../../state/viewState.ts';
import GitViewHead, { type RepoHead } from '../GitViewHead.vue';
import ReviewCommentsPane from './ReviewCommentsPane.vue';
import ReviewCommitList from './ReviewCommitList.vue';
import ReviewFilesPane from './ReviewFilesPane.vue';
import ReviewNoBranch from './ReviewNoBranch.vue';
import ReviewToolbars from './ReviewToolbars.vue';

const props = defineProps<{
  transport: Transport;
  viewState: ViewStateStore;
  target?: ReviewTarget | null;
  lockTarget?: boolean;
  onReviewApi?: MountOptions['onReviewApi'];
  reviewShortcuts?: boolean;
  repoHead?: RepoHead;
  workingChanges?: ReviewWorkingSection | null;
}>();

const bridge = new BridgeClient(props.transport);
const connectionState = bridge.connectionState;
const refsState = new RefsState(bridge);
const review = shallowRef<ReviewSessionState | undefined>(undefined);
const reviewFiles = shallowRef<ReviewFilesState | undefined>(undefined);
const reviewComments = shallowRef<ReviewCommentsState | undefined>(undefined);
// P108 F12: this instance's own root, so a document-level handler below can tell a key/click that
// landed inside it from one that landed in a sibling mount (a graph tab, another review sidebar).
const rootEl = useTemplateRef<HTMLElement>('rootEl');
const repoId = ref<string | undefined>(undefined);
const noActiveRepo = ref(false);
// G12 D6: same treatment as App.vue's own bootError — "Loading…" forever is not an acceptable
// rendering of a failed bootstrap() (F7).
const bootError = ref<string | undefined>(undefined);

watch(repoId, (id) => refsState.setRepoId(id));

let unsubscribeTarget: (() => void) | undefined;
let unsubscribeUiAction: (() => void) | undefined;

// Bumped by every `applyTarget`. Callers that apply more state after their own awaits re-check
// the token they got back and skip writes a newer target already superseded.
let targetSequence = 0;

/** Routes a fire-and-forget rejection (`main.ts` sets no errorHandler) into the live region.
 *  `transport-closed` is ignored: the view is already gone. */
const { text: liveAnnouncement, announce } = useLiveRegion();

function reportAsyncError(err: unknown, key: MessageKey): void {
  if (err instanceof TransportError && err.code === 'transport-closed') return;
  announce(t(key, { error: errorText(err) }));
}

async function applyTarget(
  nextRepoId: string,
  branch: string,
  open?: { readonly base?: string; readonly pane?: ReviewPane },
): Promise<number> {
  const token = ++targetSequence;
  repoId.value = nextRepoId;
  await review.value?.setTarget(nextRepoId, branch, open);
  return token;
}

/** Re-runs `setTarget` against the same branch: re-resolves the base and re-streams the range from
 *  scratch. A manually-chosen base (an `'override'` resolution) is re-applied after, mirroring
 *  `resumeSession`'s own `baseOverride` restore. `repoId` itself is unchanged, so `refsState`'s own
 *  `watch(repoId, ...)` above never re-fires on its own — re-seeded explicitly here, same as
 *  `App.vue`'s `applyRepoIdToStates`. A no-op with no review target open yet. */
async function recompare(): Promise<void> {
  const id = repoId.value;
  const branch = review.value?.branch.value;
  if (!id || !branch) return;
  const overrideBase =
    review.value?.resolution.value?.reason === 'override'
      ? review.value.resolution.value.base
      : undefined;
  const token = await applyTarget(id, branch);
  // P108 F6: a `review.target` push (or another recompare) landed during `applyTarget`'s own
  // await — that newer target already owns `review`/`repoId` now; applying this stale override
  // base or re-seeding `refsState` with this call's own (now old) `id` would stomp on it.
  if (token !== targetSequence) return;
  if (overrideBase) await review.value?.setBase(overrideBase);
  if (token !== targetSequence) return;
  refsState.setRepoId(id);
}

function retryCompare(): void {
  recompare().catch((err: unknown) => reportAsyncError(err, 'git.review.async.compare.error'));
}

async function bootstrap(): Promise<void> {
  // A retry re-enters here: dispose the earlier run's handlers and states first.
  unsubscribeTarget?.();
  unsubscribeUiAction?.();
  review.value?.dispose();
  reviewFiles.value?.dispose();
  reviewComments.value?.dispose();

  await bridge.init();
  review.value = new ReviewSessionState(bridge, { autoApply: props.lockTarget === true });
  reviewFiles.value = new ReviewFilesState(bridge);
  reviewComments.value = new ReviewCommentsState(bridge);

  if (!props.lockTarget) {
    unsubscribeTarget = bridge.on('review.target', (event) => {
      applyTarget(event.repoId, event.branch).catch((err: unknown) =>
        reportAsyncError(err, 'git.review.async.switchTarget.error'),
      );
    });
  }
  // G11 D17: the palette's own route into this already-mounted webview — toggles the file
  // currently open in the Files pane, or announces there is none to toggle.
  unsubscribeUiAction = bridge.on('ui.action', (event) => {
    onUiAction(event.action);
  });

  if (props.target) {
    const { repoId: targetRepo, branch, base, pane } = props.target;
    await applyTarget(targetRepo, branch, { base, pane });
    return;
  }
  const list = await bridge.request('repo.list', {});
  // A `review.target` push can land during this await (the `bridge.on('review.target',
  // ...)` listener just above is already live) and already call `applyTarget`, setting `repoId`
  // itself — must not then clobber it with the workspace's own default active repo while
  // `review` still holds the pushed repo/branch.
  if (repoId.value !== undefined) return;
  if (list.activeRepoId) {
    repoId.value = list.activeRepoId;
    await resumeSession(list.activeRepoId);
  } else {
    noActiveRepo.value = true;
  }
}

/**
 * The read half of durable persistence — its own round trip, made once `bootstrap()`
 * has a real `repoId` in hand (right here, alongside `props.target`/`review.target`'s own
 * priority, both of which still win unchanged: `props.target` already returned above, and a
 * `review.target` push that races ahead of this call already set `review.branch.value`, checked
 * again just before applying so it is never clobbered). No commit/diff data is ever restored —
 * only identifiers `setTarget`/`setBase` already re-ask fresh; a snapshot the extension host
 * judges too old (14 days) comes back as `{session: null}`, the same as never having saved one.
 */
async function resumeSession(id: string): Promise<void> {
  if (review.value?.branch.value) return;
  let loaded: { session: ReviewSessionSnapshot | null };
  try {
    loaded = await bridge.request('review.session.load', { repoId: id });
  } catch {
    return; // best-effort: a failed resume falls back to the ordinary "no branch" picker
  }
  const session = loaded.session;
  if (!session || review.value?.branch.value) return;
  const token = await applyTarget(id, session.branch);
  // P108 F6: `session.branch` was checked above the *previous* await (`review.session.load`), not
  // this one — a `review.target` push or a user's own pick during `applyTarget` itself must still
  // win over this resume's own base/pane/filter restore.
  if (token !== targetSequence) return;
  if (session.baseOverride !== null) await review.value?.setBase(session.baseOverride);
  if (token !== targetSequence) return;
  review.value?.setPane(session.pane);
  listMode.value = session.listMode;
  filter.value = session.filter;
  reviewFiles.value?.setDiffMode(session.diffMode);
}

/** The write half — one `watch()` over the same fields the snapshot lists, coalesced
 *  with a trivial `queueMicrotask` (this is not a hot path). `persistSession` reads the *current*
 *  values itself rather than the watcher's own old/new arguments, so several fields changing in
 *  the same tick (e.g. `setTarget` resetting `pane` while also changing `branch`) still produce
 *  exactly one save with everything already settled. */
let sessionSaveScheduled = false;
function scheduleSessionSave(): void {
  if (sessionSaveScheduled) return;
  sessionSaveScheduled = true;
  queueMicrotask(() => {
    sessionSaveScheduled = false;
    persistSession().catch((err: unknown) =>
      reportAsyncError(err, 'git.review.async.saveSession.error'),
    );
  });
}

async function persistSession(): Promise<void> {
  if (props.lockTarget) return;
  const id = repoId.value;
  const r = review.value;
  if (!id || !r?.branch.value) return;
  const session: ReviewSessionSnapshot = {
    branch: r.branch.value,
    baseOverride:
      r.resolution.value?.reason === 'override' ? (r.resolution.value.base ?? null) : null,
    pane: r.pane.value,
    listMode: listMode.value,
    filter: filter.value,
    diffMode: reviewFiles.value?.diffMode.value ?? 'sinceReview',
  };
  await bridge.request('review.session.save', { repoId: id, session });
}

/** G19 D11a (item 11): "back to branch selection" — clears the in-memory target and, since this
 *  is an explicit "go back", also clears the durable resume point (D11b), so the next cold boot
 *  never silently jumps back into a comparison the user deliberately left. */
async function goBackToSelection(): Promise<void> {
  const id = repoId.value;
  review.value?.clearTarget();
  if (id) await bridge.request('review.session.save', { repoId: id, session: null });
}

// The file filter is revealed by the toolbar's search button; `filterVisible` gates it.
const filterVisible = ref(false);

// Type to search: a printable key in the commit list reveals the file filter and starts it.
const { showSearch } = usePanelHeaderSearch(rootEl, {
  searchable: () => review.value?.phase.value === 'listing',
  getSearch: () => filter.value,
  setSearch: (value) => {
    filter.value = value;
    if (value !== '') filterVisible.value = true;
  },
  searchTestid: 'review-file-filter',
});
watch(filterVisible, (visible) => {
  if (!visible) showSearch.value = false;
});

function onUiAction(action: UiActionKind): void {
  switch (action) {
    case 'toggleFileReviewed': {
      const rf = reviewFiles.value;
      const path = rf?.selectedPath.value;
      if (!rf || path === null || path === undefined) {
        announce(t('git.review.noOpenFile.announce'));
        return;
      }
      const entry = rf.files.value.find((e) => e.change.path === path);
      void rf.mark(path, markOnToggle(reviewAttention(entry?.review)));
      return;
    }
    // G13 D19: the palette's own route to the Comments pane's copy-for-AI action.
    case 'copyReviewComments':
      void reviewComments.value?.copyForAi();
      return;
    // G13 D19: pushed by the extension after an editor-side comment add/delete, so the pane
    // updates without the user switching away from it.
    case 'refreshReviewComments':
      void reviewComments.value?.reload();
      return;
    // G15 D9: pushed by the extension after an editor-side range mark, so the Files pane's
    // reviewed state doesn't sit stale until an unrelated repo.changed comes along — reuses the
    // existing 'refresh' UiActionKind rather than adding a new contract member (§11.2).
    case 'refresh':
      reviewFiles.value?.reload();
      return;
    default:
      return;
  }
}

// The Files pane's target follows the session's resolved (branch, base).
watch(
  () => {
    const r = review.value;
    if (r?.phase.value !== 'listing') return undefined;
    const base = r.resolution.value?.base;
    const branch = r.branch.value;
    const id = repoId.value;
    return id && branch && base ? `${id}\0${branch}\0${base}` : undefined;
  },
  (key) => {
    const [id, branch, base] = key?.split('\0') ?? [];
    reviewFiles.value?.setTarget(
      id !== undefined && branch !== undefined && base !== undefined
        ? { repoId: id, branch, base }
        : undefined,
    );
  },
);

// The Comments pane's target is repoId + branch only: its session key has no base.
watch(
  () => {
    const r = review.value;
    if (r?.phase.value !== 'listing') return undefined;
    const branch = r.branch.value;
    const id = repoId.value;
    return id && branch ? { repoId: id, branch } : undefined;
  },
  (target) => {
    reviewComments.value?.setTarget(target);
  },
);

// D10's own "on becoming the active pane" — a plain reload against whatever target is already
// current; setTarget's own initial load (above) already covers the first activation.
watch(
  () => review.value?.pane.value,
  (pane) => {
    if (pane === 'comments') void reviewComments.value?.reload();
  },
);

const { cursor, canStep, step, reveal } = useReviewStepping({
  reviewFiles,
  committedOrder: () => orderedPaths.value,
  working: () => props.workingChanges,
  filter: () => filter.value,
  listMode: () => listMode.value,
  shortcuts: () => props.reviewShortcuts === true && review.value?.phase.value === 'listing',
  announce,
});

function onSelectComment(path: string, origin: ReviewComment['origin']): void {
  if (origin === 'working' && props.workingChanges) props.workingChanges.onOpen(path, false);
  else reviewFiles.value?.selectFile(path);
}

// A plain computed so the template narrows the row-action bundle once.
const rowActions = computed<DetailActions | undefined>(() => review.value?.rowActions.value);

const filesActions = createReviewFilesActions(bridge, () => repoId.value, announce);

onMounted(() => {
  props.onReviewApi?.({
    selectFile: (path) => reviewFiles.value?.selectFile(path),
    revealFile: reveal,
    adjacentFile: (path, dir) => adjacentInOrder(orderedPaths.value, path, dir),
    committedPaths: () => (reviewFiles.value?.files.value ?? []).map((e) => e.change.path),
    refresh: () => {
      reviewFiles.value?.reload();
      review.value?.checkNow();
    },
  });
  // The review-view perf metric measures from this first-paint mark.
  requestAnimationFrame(() => {
    performance.mark('kira:first-paint');
    performance.measure('kira:first-paint', undefined, 'kira:first-paint');
  });
  void bootstrap().catch((err: unknown) => {
    bootError.value = errorText(err);
  });
});

function retryBootstrap(): void {
  sharedRetryBootstrap(bootError, bootstrap);
}

onBeforeUnmount(() => {
  unsubscribeTarget?.();
  unsubscribeUiAction?.();
  review.value?.dispose();
  reviewFiles.value?.dispose();
  reviewComments.value?.dispose();
  refsState.dispose();
  bridge.dispose();
});

// One panel-level filter/list-mode toolbar, replacing what used to be a separate
// FileTree toolbar per expanded row (and a third copy in ReviewFilesPane). Owns the state;
// ReviewCommitRow/ReviewFilesPane's own FileTree instances render no toolbar of their own
// (show-toolbar="false") and receive these as plain props.
const listMode = ref<FileListMode>('tree');
const filter = ref('');
// Reactive so a host computed reading the navigator refreshes when the list order changes.
const orderedPaths = computed(() =>
  orderedFilePaths(
    (reviewFiles.value?.files.value ?? []).map((entry) => entry.change),
    filter.value,
    listMode.value,
  ),
);

// Saves the session when any snapshot field changes; scheduleSessionSave coalesces same-tick changes.
watch(
  () => [
    review.value?.branch.value,
    review.value?.resolution.value,
    review.value?.pane.value,
    listMode.value,
    filter.value,
    reviewFiles.value?.diffMode.value,
  ],
  scheduleSessionSave,
);

function pickBranch(name: string): void {
  const id = repoId.value;
  if (!id) return;
  void applyTarget(id, name);
}

const filesChangedCount = computed(() => reviewFiles.value?.files.value.length ?? 0);
const commentsCount = computed(() => reviewComments.value?.comments.value.length ?? 0);

useReviewAnnouncements(review, announce);
</script>

<template>
  <div
    ref="rootEl"
    class="flex flex-col h-full w-full relative bg-bg text-fg font-ui text-graph-md overflow-hidden"
    :data-connection-state="connectionState"
  >
    <!-- P131 Part 3 §5.1: MountRoot.vue's own TooltipProvider (Part 2 §3.5) already wraps this
         root, so no tooltip surface of this component's own is needed. -->
    <span class="sr-only" data-testid="connection-state">{{ connectionState }}</span>
    <div class="sr-only" role="status" aria-live="polite" data-testid="live-announcements">
      {{ liveAnnouncement }}
    </div>
    <GitViewHead v-if="repoHead" :repo="repoHead" />

    <template v-if="bootError">
      <div class="flex flex-col gap-2 p-3" data-testid="boot-error">
        <p class="text-muted-foreground">{{
          t('git.app.bootError.error', { error: bootError })
        }}</p>
        <Button
          variant="dialog"
          size="kira"
          class="self-start"
          data-testid="boot-retry"
          @click="retryBootstrap"
        >
          {{ t('common.action.retry.label') }}
        </Button>
      </div>
    </template>

    <template v-else-if="!review">
      <EmptyState fill loading :title="t('git.review.loading.loading')" />
    </template>

    <template v-else-if="noActiveRepo">
      <div class="flex flex-col gap-1 p-3 min-h-0">
        <h2 class="text-kira-lg font-medium text-fg">{{ t('git.review.start.title') }}</h2>
        <p>{{ t('git.review.noRepo.emptyHint') }}</p>
      </div>
    </template>

    <template v-else-if="!review.branch.value">
      <ReviewNoBranch :refs-state="refsState" @pick="pickBranch" />
    </template>

    <template v-else>
      <ReviewToolbars
        v-model:list-mode="listMode"
        v-model:filter="filter"
        v-model:filter-visible="filterVisible"
        :review="review"
        :refs-state="refsState"
        :files-count="filesChangedCount"
        :comments-count="commentsCount"
        :locked="lockTarget"
        @back="goBackToSelection"
      />

      <div class="flex-1 min-h-0 flex flex-col">
        <p v-if="review.phase.value === 'resolving'" class="p-3 text-muted-foreground">
          {{ t('git.review.resolving.loading') }}
        </p>

        <div v-else-if="review.phase.value === 'error'" class="flex flex-col items-start gap-1 p-3">
          <FieldError>{{
            t('git.review.async.compare.error', { error: review.resolveError.value ?? '' })
          }}</FieldError>
          <Button variant="dialog" size="kira" data-testid="review-retry" @click="retryCompare">
            {{ t('common.action.retry.label') }}
          </Button>
        </div>

        <div
          v-else-if="review.phase.value === 'ask'"
          class="p-3 text-muted-foreground"
          data-testid="review-ask"
        >
          <p>
            <CatalogText k="git.review.ask.body" :params="{ branch: review.branch.value }">
              <template #branch><strong>{{ review.branch.value }}</strong></template>
            </CatalogText>
          </p>
        </div>

        <p
          v-else-if="review.phase.value === 'unrelated'"
          class="p-3 text-muted-foreground"
          data-testid="review-unrelated"
        >
          {{
            t('git.review.unrelated.body', {
              branch: review.branch.value,
              base: review.resolution.value?.base ?? '',
            })
          }}
        </p>

        <p
          v-else-if="review.phase.value === 'empty'"
          class="p-3 text-muted-foreground"
          data-testid="review-empty"
        >
          {{
            t('git.review.empty.body', {
              branch: review.branch.value,
              base: review.resolution.value?.base ?? '',
            })
          }}
        </p>

        <template
          v-else-if="
            review.phase.value === 'listing' && review.pane.value === 'commits' && rowActions
          "
        >
          <ReviewCommitList
            :review="review"
            :actions="rowActions"
            :list-mode="listMode"
            :filter="filter"
            :boundary="rootEl"
          />
        </template>

        <ReviewFilesPane
          v-else-if="
            review.phase.value === 'listing' &&
            review.pane.value === 'files' &&
            reviewFiles &&
            filesActions
          "
          class="flex-1 min-h-0"
          :review-files="reviewFiles"
          :store="review.store"
          :actions="filesActions"
          :list-mode="listMode"
          :filter="filter"
          :working="workingChanges ?? null"
          :cursor="cursor"
          :can-step="canStep"
          :review-shortcuts="reviewShortcuts === true"
          @step="step"
          @opened-working="(path) => (cursor = { section: 'working', path })"
        />

        <ReviewCommentsPane
          v-else-if="
            review.phase.value === 'listing' && review.pane.value === 'comments' && reviewComments
          "
          class="flex-1 min-h-0"
          :review-comments="reviewComments"
          @select="onSelectComment"
        />
      </div>
    </template>
  </div>
</template>
