<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useTimeoutFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { registerCommand } from '@workbench/shortcuts/commands';
import { computed, onMounted, onUnmounted, ref, shallowRef, watch } from 'vue';
import { useHostLineComment } from '../../ade/v2/review/useHostLineComment';
import { useReviewWorkingDiff } from '../../ade/v2/review/useReviewWorking';
import { useAdeReviewWindowStore } from '../../ade/v2/state/adeReviewWindow';
import { gitRepoIdFor } from '../../repo/git/hostHandlers';
import { gitTransportFor } from '../../repo/git/transport';
// The diff editor mount, modelled on RepoFileView.vue line for line — same lazy Monaco
// bootstrap, same read-only posture, same navigation registration. The one real difference is the
// editor itself: a diff editor over two models (HEAD, worktree) instead of one.
//
// Extends this same tab kind for a *commit* diff (openRepoCommitDiffTab)
// rather than forking a second one — repoDiffTabStateSchema's revision pair is null on both
// sides for the original HEAD-vs-worktree comparison (unchanged below) and non-null for a commit
// diff.
//
// The content-resolution + editor-construction body now lives in useDiffEditor.ts,
// shared with RepoMultiDiffView.vue's own per-file sections — this view keeps only what stays
// single-active-view scoped: repoId resolution, the view.find/repo.goToFileFromDiff command
// registrations, and review-decorations wiring.
import { useReviewNavigationStore } from '../../state/reviewNavigation';
import type { RepoDiffTabRecord } from '../../state/tabDomain';
import { noRepositoryMessage, repoIdOfTab } from '../../state/workspace';
import { type DiffLayout, useDiffLayout } from './diffLayout';
import { attachDiffNavigation, type DiffNavigation } from './diffNavigation';
import { loadMonaco } from './monaco';
import { attachReviewDecorations, type ReviewDecorationsHandle } from './reviewDecorations';
import { committedReviewSource, workingReviewSource } from './reviewSource';
import { useDiffEditor } from './useDiffEditor';

const props = defineProps<{ tab: RepoDiffTabRecord }>();

const layout = useDiffLayout();
const layoutItems = [
  {
    value: 'sideBySide',
    label: t('space.repo.view.diffLayoutSideBySide.label'),
    icon: 'split-horizontal',
    testid: 'repo-diff-layout-side-by-side',
  },
  {
    value: 'inline',
    label: t('space.repo.view.diffLayoutInline.label'),
    icon: 'diff-single',
    testid: 'repo-diff-layout-inline',
  },
];

const errorMessage = ref('');
const container = ref<HTMLElement | null>(null);
let unregisterFind: (() => void) | null = null;
let unregisterGoToFile: (() => void) | null = null;
let reviewDecorations: ReviewDecorationsHandle | null = null;
// True once the review layer (glyphs, context-menu actions, first load) is attached and loaded.
const reviewReady = ref(false);

const navigation = useReviewNavigationStore();
const diffNavigation = shallowRef<DiffNavigation | null>(null);
// Bumped when the editor's change cursor moves; the navigator itself is reactive.
const navTick = ref(0);
const isReview = props.tab.state.review !== null;
const workingBranchId = props.tab.state.working?.branchId;
// Only a review window can mark and comment on uncommitted lines; elsewhere the tab stays a plain diff.
const windowTarget = useAdeReviewWindowStore().target;
const workingTarget =
  workingBranchId !== undefined && windowTarget?.branchId === workingBranchId ? windowTarget : null;
const repoIdForNav = repoIdOfTab(props.tab);
// A committed tab of the window's own branch can post a line comment to the branch's change request.
const tabReview = props.tab.state.review;
const hostComment =
  windowTarget !== null &&
  tabReview !== null &&
  tabReview.branch === windowTarget.branch &&
  repoIdForNav === windowTarget.codeRepoId
    ? useHostLineComment(() => windowTarget, tabReview.branchTip)
    : undefined;

function hasAdjacentFile(dir: 'next' | 'previous'): boolean {
  if (!isReview || !repoIdForNav) return false;
  return navigation.navigatorFor(repoIdForNav)?.adjacentFile(props.tab.path, dir) != null;
}
// Off until the diff is computed: an empty change list would send the move straight to the next file.
const canNext = computed(() => {
  navTick.value;
  const nav = diffNavigation.value;
  if (nav && !nav.ready.value) return false;
  return (nav?.canNext.value ?? false) || hasAdjacentFile('next');
});
const canPrevious = computed(() => {
  navTick.value;
  const nav = diffNavigation.value;
  if (nav && !nav.ready.value) return false;
  return (nav?.canPrevious.value ?? false) || hasAdjacentFile('previous');
});

// Past the last (first) change of this file, open the next (previous) file in the review list.
function crossFile(dir: 'next' | 'previous'): void {
  if (!repoIdForNav) return;
  const navigator = navigation.navigatorFor(repoIdForNav);
  const target = navigator?.adjacentFile(props.tab.path, dir);
  if (!navigator || target == null) return;
  navigation.requestArrival({
    repoId: repoIdForNav,
    path: target,
    at: dir === 'next' ? 'first' : 'last',
  });
  navigator.selectFile(target);
}
function onNextChange(): void {
  if (diffNavigation.value?.next()) navTick.value++;
  else crossFile('next');
}
function onPreviousChange(): void {
  if (diffNavigation.value?.previous()) navTick.value++;
  else crossFile('previous');
}

type ViewState = 'loading' | 'found' | 'binary' | 'tooLarge' | 'bothMissing' | 'error';
const state = ref<ViewState>('loading');
function onGoToFile(): void {
  diffEditor?.goToFile();
}

let diffEditor: ReturnType<typeof useDiffEditor> | null = null;

// A mark the server refused: the file changed on disk, and the view reloaded to show it.
const markRefused = ref(false);
const hideRefused = useTimeoutFn(() => (markRefused.value = false), TIMING.postedStatus, {
  immediate: false,
});
function onMarkRefused(): void {
  markRefused.value = true;
  hideRefused.start();
}

async function mount(): Promise<void> {
  const repoId = repoIdOfTab(props.tab);
  if (!repoId) {
    state.value = 'error';
    errorMessage.value = noRepositoryMessage();
    return;
  }

  const { left, right, review } = props.tab.state;
  const handle = useDiffEditor(container, {
    editorKey: props.tab.id,
    repoId,
    path: props.tab.path,
    left,
    right,
    review: review !== null,
    reviewBranch: review?.branch,
    workingBranchId,
  });
  diffEditor = handle;
  const arrival = repoId ? navigation.takeArrival(repoId, props.tab.path) : null;
  await handle.mount();
  if (diffEditor !== handle) return; // Unmounted while mount() awaited.
  state.value = handle.state.value;
  errorMessage.value = handle.errorMessage.value;
  navTick.value++;
  if (state.value !== 'found') return;

  const editor = handle.editor.value;
  if (!editor) return;
  const mod = await loadMonaco();
  if (diffEditor !== handle) return;
  const gitRepoId = left === null ? undefined : gitRepoIdFor(repoId);

  // The modified (worktree) pane, never the HEAD pane — the same reasoning
  // useDiffEditor's own goToFile follows for which side is navigable. view.find is a global
  // command, so it must reach through to one editor pane rather than the diff editor as a whole
  // (IStandaloneDiffEditor itself has no getAction).
  const modifiedEditor = editor.getModifiedEditor();
  unregisterFind = registerCommand('view.find', () => {
    modifiedEditor.focus();
    void modifiedEditor.getAction('actions.find')?.run();
  });

  // P74 §7.4 item 1 / P92 item 7: the diff editor's own "go to file" — both the palette command
  // and the visible header button (template) run useDiffEditor's own goToFile().
  unregisterGoToFile = registerCommand('repo.goToFileFromDiff', () => diffEditor?.goToFile());

  // C11 §7.4/S11: the comment-thread/mark-reviewed layer, only for a review diff tab. `left` is
  // never null here — openRepoReviewDiffTab (S8) always supplies both revisions alongside `review`.
  if (review !== null && gitRepoId !== undefined && left !== null) {
    reviewDecorations = attachReviewDecorations(
      mod,
      editor,
      committedReviewSource({
        transport: gitTransportFor(repoId),
        gitRepoId,
        path: props.tab.path,
        leftRev: left,
        review,
        hostComment,
      }),
    );
  } else if (workingTarget !== null) {
    reviewDecorations = attachReviewDecorations(
      mod,
      editor,
      workingReviewSource({
        target: workingTarget,
        path: props.tab.path,
        shownHash: () => diffEditor?.contentHash.value,
        onMarkRefused,
      }),
    );
  }

  const attached = reviewDecorations;
  void attached?.ready.then(() => {
    if (reviewDecorations === attached) reviewReady.value = true;
  });

  // After the decorations so its listener runs once comment lines were revealed.
  const changeNav = attachDiffNavigation(editor);
  diffNavigation.value = changeNav;
  editor.onDidUpdateDiff(() => navTick.value++);
  navTick.value++;
  if (arrival) {
    changeNav.land(arrival);
    await reviewDecorations?.ready;
    if (diffNavigation.value === changeNav) changeNav.settle();
  }
}

// An uncommitted file follows the disk: a refreshed read rewrites both models in place.
const workingDiff = useReviewWorkingDiff(
  () => workingBranchId ?? '',
  () => props.tab.path,
);
watch(
  () => workingDiff.data.value,
  (diff) => {
    const models = diffEditor?.editor.value?.getModel();
    if (!diff || !models || diff.head.kind === 'binary' || diff.worktree.kind === 'binary') return;
    if (models.original.getValue() !== diff.head.text) models.original.setValue(diff.head.text);
    if (models.modified.getValue() !== diff.worktree.text) {
      models.modified.setValue(diff.worktree.text);
    }
    // The hash travels with the text it was read with, never ahead of what the editor shows.
    if (diffEditor) diffEditor.contentHash.value = diff.contentHash;
    reviewDecorations?.reload();
  },
);

onMounted(() => void mount());

// A mere tab switch away — the widget disposes, the models survive in editors.ts until an actual
// close calls TAB_KINDS['repo-diff'].dropResources.
onUnmounted(() => {
  unregisterFind?.();
  unregisterFind = null;
  unregisterGoToFile?.();
  unregisterGoToFile = null;
  reviewDecorations?.dispose();
  reviewDecorations = null;
  diffNavigation.value?.dispose();
  diffNavigation.value = null;
  diffEditor?.dispose();
  diffEditor = null;
});
</script>

<template>
  <div class="h-full flex flex-col">
    <!-- P92 item 7: the same action the repo.goToFileFromDiff palette command runs — P74 built the
         behaviour and gave it no other affordance. -->
    <ViewToolbar v-if="state === 'found' || (isReview && state !== 'loading')">
      <Button
        variant="toolbar"
        size="kira"
        data-testid="repo-diff-prev-change"
        :disabled="!canPrevious"
        :aria-label="t('space.repo.view.previousChange.label')"
        :data-kira-tip="t('space.repo.view.previousChange.label')"
        @click="onPreviousChange"
      >
        <CodiconIcon name="arrow-up" :size="13" />
      </Button>
      <Button
        variant="toolbar"
        size="kira"
        data-testid="repo-diff-next-change"
        :disabled="!canNext"
        :aria-label="t('space.repo.view.nextChange.label')"
        :data-kira-tip="t('space.repo.view.nextChange.label')"
        @click="onNextChange"
      >
        <CodiconIcon name="arrow-down" :size="13" />
      </Button>
      <Button
        v-if="state === 'found' && !workingBranchId"
        variant="toolbar"
        size="kira"
        data-testid="repo-diff-go-to-file"
        @click="onGoToFile"
      >
        <CodiconIcon name="go-to-file" :size="13" />
        {{ t('common.goToFile.label') }}
      </Button>
      <Badge v-if="workingBranchId" variant="warn" data-testid="repo-diff-uncommitted">
        {{ t('space.ade.changes.uncommitted.label') }}
      </Badge>
      <SecondaryTabs
        v-if="state === 'found'"
        class="ml-auto"
        variant="segmented"
        :aria-label="t('space.repo.view.diffLayout.aria')"
        :model-value="layout"
        :items="layoutItems"
        @update:model-value="(value) => (layout = value as DiffLayout)"
      >
        <template #item="{ item }">
          <CodiconIcon :name="item.icon ?? ''" :size="13" />
          <span class="sr-only">{{ item.label }}</span>
        </template>
      </SecondaryTabs>
    </ViewToolbar>
    <Alert v-if="markRefused" variant="warn" data-testid="repo-diff-mark-refused">
      <AlertDescription>{{ t('space.ade.review.working.markRefused.body') }}</AlertDescription>
    </Alert>
    <div
      v-if="state === 'loading' || state === 'found'"
      ref="container"
      class="monaco-host flex-auto min-h-0 w-full"
      data-testid="repo-diff-editor"
      :data-review-ready="reviewReady"
    />
    <EmptyState
      v-else-if="state === 'binary'"
      fill
      icon="file-binary"
      :title="t('space.repo.view.binaryCompare.empty')"
    />
    <EmptyState
      v-else-if="state === 'tooLarge'"
      fill
      icon="warning"
      :title="t('space.repo.view.tooLargeCompare.empty')"
    />
    <EmptyState
      v-else-if="state === 'bothMissing'"
      fill
      icon="warning"
      :title="t('space.repo.view.missing.empty')"
    />
    <EmptyState
      v-else
      fill
      icon="warning"
      :title="errorMessage || t('space.repo.view.openDiffFailed.error')"
    />
  </div>
</template>
