<script setup lang="ts">
import type { CommitStore } from '@kira/git-core';
import type { ReviewDiffMode, ReviewFileStatus } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
/**
 * The review sidebar's Files pane, and the only new component this phase adds to
 * `packages/git-ui` (which SPEC otherwise freezes). `FileTree` over `ReviewFilesState` instead of
 * `DetailState`: the rebase/squash/amend case SPEC wrote this phase for makes every commit in the
 * Commits tab unfamiliar, so a file-level view of the range is the surface that survives it.
 *
 * Selecting a file no longer opens an in-webview `DiffView` — it opens the host's native
 * diff editor (`ReviewFilesState.selectFile`'s own `editor.openRangeDiff` call). This component
 * keeps the file list on screen throughout and renders only the Since-review/Full-range toggle
 * and the delta status line as feedback, never a diff body.
 *
 * `FileTree`'s merge-parent picker never renders here (`parents` is always `[]` — a branch review
 * has no single "commit" with parents to pick between); `store` is still required by that
 * component's own props, so `ReviewView.vue` passes its own `PackedStreamState.store` down, unused
 * by anything this pane shows. `list-mode`/`filter` are props now — `ReviewView`
 * owns one toolbar for both the Commits and Files panes, so this component's own toolbar is
 * disabled (`show-toolbar="false"`) rather than duplicated.
 */
import InlineState from '@theme/components/InlineState.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { FieldError } from '@theme/components/ui/field';
import { type MessageKey, t } from '@theme/i18n';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { formatShortcut } from '@workbench/shortcuts/keys';
import { computed } from 'vue';
import { ACTION_ICONS } from '../../icons/index.ts';
import { markOnToggle, reviewAttention } from '../../lib/reviewAttention.ts';
import type { FileListMode } from '../../state/detail.ts';
import type { DetailActions } from '../../state/detailActions.ts';
import type { ReviewWorkingSection } from '../../state/review.ts';
import type { ReviewFilesState } from '../../state/reviewFiles.ts';
import FileTree from '../FileTree.vue';

// P131 Part 3 §5.5: a local option shape for the ToggleGroup below — matches RefsPopover.vue's
// own PickerTabOption precedent, not a shared package export.
interface DiffModeOption {
  readonly id: ReviewDiffMode;
  readonly icon: string;
  readonly label: MessageKey;
}

const props = defineProps<{
  reviewFiles: ReviewFilesState;
  store: CommitStore;
  actions: DetailActions;
  // G12 D13: ReviewView.vue's one panel-level toolbar owns these; this pane's own FileTree
  // renders no toolbar of its own (show-toolbar="false") and so never emits an update to forward.
  listMode: FileListMode;
  filter: string;
  /** Uncommitted changes under the committed list; `null` hides the section. */
  working: ReviewWorkingSection | null;
  /** The file last opened, committed or uncommitted. */
  cursor: { section: 'committed' | 'working'; path: string } | null;
  /** Some listed file still needs a review. */
  canStep: boolean;
  /** Show the Alt+F8 chords on the step buttons. */
  reviewShortcuts: boolean;
}>();

const emit = defineEmits<{
  (e: 'step', dir: 'next' | 'previous'): void;
  (e: 'openedWorking', path: string): void;
}>();

const diffModeOptions: readonly DiffModeOption[] = [
  { id: 'sinceReview', icon: ACTION_ICONS.diffSingle, label: 'git.review.files.sinceReview.label' },
  { id: 'range', icon: ACTION_ICONS.diffMultiple, label: 'git.review.files.fullRange.label' },
];
const diffModeItems = computed(() =>
  diffModeOptions.map((o) => ({
    value: o.id,
    label: t(o.label),
    icon: o.icon,
    tooltip: t(o.label),
  })),
);
const files = computed(() => props.reviewFiles.files.value.map((entry) => entry.change));

const reviewStatesMap = computed<ReadonlyMap<string, ReviewFileStatus>>(() => {
  const map = new Map<string, ReviewFileStatus>();
  for (const entry of props.reviewFiles.files.value) map.set(entry.change.path, entry.review);
  return map;
});

const reviewedCount = computed(
  () =>
    props.reviewFiles.files.value.filter((e) => reviewAttention(e.review) === 'reviewed').length,
);
const changedCount = computed(
  () => props.reviewFiles.files.value.filter((e) => reviewAttention(e.review) === 'changed').length,
);

const workingFiles = computed(() => props.working?.files ?? []);
const workingReviewedCount = computed(
  () =>
    workingFiles.value.filter(
      (f) => reviewAttention(props.working?.reviewStates.get(f.path)) === 'reviewed',
    ).length,
);
const workingChangedCount = computed(
  () =>
    workingFiles.value.filter(
      (f) => reviewAttention(props.working?.reviewStates.get(f.path)) === 'changed',
    ).length,
);
const workingSelected = computed(() =>
  props.cursor?.section === 'working'
    ? workingFiles.value.findIndex((f) => f.path === props.cursor?.path)
    : -1,
);

function onOpenWorkingIndex(index: number, pinned: boolean): void {
  const file = workingFiles.value[index];
  if (!file) return;
  emit('openedWorking', file.path);
  props.working?.onOpen(file.path, pinned);
}

const selectedIndex = computed(() => {
  if (props.cursor?.section === 'working') return -1;
  const path = props.reviewFiles.selectedPath.value;
  if (path === null) return -1;
  return files.value.findIndex((f) => f.path === path);
});

const deltaStatusText = computed(() => {
  switch (props.reviewFiles.deltaSource.value) {
    case 'noSnapshot':
      return t('git.review.files.neverReviewed.status');
    case 'unchanged':
      return t('git.review.files.unchanged.status');
    case 'fast':
    case 'slow':
      return t('git.review.changedSince.hint');
    case 'snapshotUnavailable':
      return t('git.review.files.snapshotUnavailable.status');
    default:
      return undefined;
  }
});

// G21 D13: pinned comes straight from FileTree's own openFile emit — a click (or arrow-key move)
// is false, a double click/Enter is true.
function onOpenFileIndex(index: number, pinned: boolean): void {
  const file = files.value[index];
  if (file) props.reviewFiles.selectFile(file.path, { pinned });
}

function onToggleReviewed(path: string): void {
  const entry = props.reviewFiles.files.value.find((e) => e.change.path === path);
  void props.reviewFiles.mark(path, markOnToggle(reviewAttention(entry?.review)));
}

function onToggleReviewedFolder(paths: string[], reviewed: boolean): void {
  void props.reviewFiles.markMany(paths, reviewed);
}

function onToggleWorkingReviewed(path: string): void {
  props.working?.onToggleReviewed(
    path,
    markOnToggle(reviewAttention(props.working.reviewStates.get(path))),
  );
}
</script>

<template>
  <div class="flex flex-col min-h-0 h-full">
    <FieldError v-if="reviewFiles.loadError.value" class="p-3">
      {{ t('git.review.files.loadError.error', { error: reviewFiles.loadError.value }) }}
    </FieldError>

    <template v-else>
      <!-- G12 D12/D16: which two revisions a click opens in the host's diff editor — the one real
           capability removing DiffView would otherwise have lost. -->
      <ViewToolbar class="font-ui">
        <SecondaryTabs
          variant="segmented"
          :aria-label="t('git.review.files.compare.aria')"
          :model-value="reviewFiles.diffMode.value"
          :items="diffModeItems"
          @update:model-value="(v) => reviewFiles.setDiffMode(v as ReviewDiffMode)"
        >
          <template #item="{ item }">
            <CodiconIcon :name="item.icon ?? ''" :size="13" />
            <span class="sr-only">{{ item.label }}</span>
          </template>
        </SecondaryTabs>
        <TooltipIconButton
          icon="arrow-up"
          :label="t('git.review.files.prevUnreviewed.tooltip')"
          :kbd="reviewShortcuts ? formatShortcut('review.previousUnreviewed') : undefined"
          :disabled="!canStep"
          data-testid="review-prev-unreviewed"
          @click="emit('step', 'previous')"
        />
        <TooltipIconButton
          icon="arrow-down"
          :label="t('git.review.files.nextUnreviewed.tooltip')"
          :kbd="reviewShortcuts ? formatShortcut('review.nextUnreviewed') : undefined"
          :disabled="!canStep"
          data-testid="review-next-unreviewed"
          @click="emit('step', 'next')"
        />
        <span v-if="deltaStatusText" class="ml-auto text-muted-foreground text-graph-sm">{{
          deltaStatusText
        }}</span>
      </ViewToolbar>
      <FieldError v-if="reviewFiles.diffError.value" class="p-3">
        {{ t('git.review.files.diffError.error', { error: reviewFiles.diffError.value }) }}
      </FieldError>
      <!-- G30 round-1 functional-correctness review, finding #7: a failed review.mark used to be
           an unhandled promise rejection with nothing shown here — the checkbox just silently
           reverted on the next render. Mirrors loadError/diffError's own pattern exactly. -->
      <FieldError v-if="reviewFiles.markError.value" class="p-3">
        {{ t('git.review.files.markError.error', { error: reviewFiles.markError.value }) }}
      </FieldError>

      <div
        class="flex items-center gap-1.5 px-2 py-1 text-graph-sm font-medium"
        data-testid="review-committed-header"
      >
        <CodiconIcon name="git-commit" :size="13" />
        <span>{{ t('git.review.committed.title') }}</span>
        <span
          v-if="changedCount > 0"
          class="text-warn font-normal"
          data-testid="review-committed-changed-count"
          >{{
            t('git.review.changedCount.label', { count: changedCount })
          }}</span
        >
        <span class="ml-auto text-muted-foreground font-normal">{{
          t('git.review.reviewedCount.label', {
            reviewed: reviewedCount,
            total: files.length,
          })
        }}</span>
      </div>
      <FileTree
        data-testid="review-files-tree"
        class="flex-auto min-h-0"
        :files="files"
        :selected-file="selectedIndex"
        :list-mode="listMode"
        :filter="filter"
        :show-toolbar="false"
        :parents="[]"
        :parent-index="0"
        :store="store"
        :actions="actions"
        :review-states="reviewStatesMap"
        @open-file="onOpenFileIndex"
        @toggle-reviewed="onToggleReviewed"
        @toggle-reviewed-folder="onToggleReviewedFolder"
      />
      <section
        v-if="workingFiles.length > 0"
        class="flex flex-col min-h-0 max-h-[40%] shrink-0 border-l-2 border-l-warn"
        data-testid="review-working"
      >
        <div
          class="flex items-center gap-1.5 px-2 py-1 text-graph-sm font-medium"
          data-testid="review-working-header"
        >
          <CodiconIcon name="edit" :size="13" class="text-warn" />
          <span>{{ t('space.ade.changes.uncommitted.label') }}</span>
          <span
            v-if="workingChangedCount > 0"
            class="text-warn font-normal"
            data-testid="review-working-changed-count"
            >{{
              t('git.review.changedCount.label', { count: workingChangedCount })
            }}</span
          >
          <span class="ml-auto text-muted-foreground font-normal">{{
            t('git.review.reviewedCount.label', {
              reviewed: workingReviewedCount,
              total: workingFiles.length,
            })
          }}</span>
        </div>
        <p class="px-2 pb-1 text-muted-foreground text-graph-sm">{{
          t('git.review.working.hint')
        }}</p>
        <FileTree
          data-testid="review-working-tree"
          class="flex-auto min-h-0 overflow-auto"
          :files="workingFiles"
          :selected-file="workingSelected"
          :list-mode="listMode"
          :filter="filter"
          :show-toolbar="false"
          :parents="[]"
          :parent-index="0"
          :store="store"
          :actions="actions"
          :review-states="working?.reviewStates"
          @open-file="onOpenWorkingIndex"
          @toggle-reviewed="onToggleWorkingReviewed"
          @toggle-reviewed-folder="
            (paths, reviewed) => working?.onToggleReviewedFolder(paths, reviewed)
          "
        />
      </section>
      <InlineState v-if="reviewFiles.loading.value && files.length === 0" kind="loading">
        {{ t('common.state.loading.loading') }}
      </InlineState>
    </template>
  </div>
</template>
