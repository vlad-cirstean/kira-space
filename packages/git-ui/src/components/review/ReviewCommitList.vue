<script setup lang="ts">
import { SETTINGS } from '@kira/git-core';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useEventListener } from '@vueuse/core';
import { formatCount } from '@workbench/behaviour/format';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, ref } from 'vue';
import { keptScrollIf } from '../../lib/reviewKeep.ts';
import type { FileListMode } from '../../state/detail.ts';
import type { DetailActions } from '../../state/detailActions.ts';
import type { ReviewSessionState } from '../../state/review.ts';
import ReviewCommitRow from './ReviewCommitRow.vue';

// The Commits pane: a virtualised list of expandable commit rows with a roving-tabindex cursor, one
// Escape handler for the whole view, the stale-review banner and the Load more button.
const props = defineProps<{
  review: ReviewSessionState;
  actions: DetailActions;
  listMode: FileListMode;
  filter: string;
  /** The view's root: Escape counts only when it lands inside it (a graph tab can sit beside). */
  boundary: HTMLElement | null;
  /** View-state key of the list scroll; null keeps none. */
  keepScroll?: string | null;
}>();

// `generation` is read so a reset that lands on the same row count (a base override back to an
// equally-sized range) still recomputes this list rather than reusing stale row identities.
const shas = computed<readonly string[]>(() => {
  const r = props.review;
  void r.generation.value;
  const out: string[] = [];
  for (let i = 0; i < r.loadedRows.value; i++) out.push(r.store.shaAt(i));
  return out;
});

const rowsEl = ref<HTMLDivElement | null>(null);
const focusedSha = ref<string | null>(null);
const focusedRow = computed(() => {
  const index = focusedSha.value === null ? -1 : shas.value.indexOf(focusedSha.value);
  return index === -1 ? 0 : index;
});

// Collapsed rows are two lines; an expanded row's real height is measured (`measureRow`). Rows
// position with `top`, not `transform`: a row's context menu is `position: fixed` and a transformed
// ancestor would re-anchor it.
const COLLAPSED_ROW_ESTIMATE = 44;
const { virtualizer, virtualItems, totalSize, onScroll } = useVirtualRows({
  count: () => shas.value.length,
  rowHeight: () => COLLAPSED_ROW_ESTIMATE,
  scrollElement: rowsEl,
  pinned: () => focusedRow.value,
});

keptScrollIf(() => props.keepScroll ?? null, rowsEl, { ready: () => totalSize.value > 0 });

const focusedRowMounted = computed(() =>
  virtualItems.value.some((item) => item.index === focusedRow.value),
);

function measureRow(el: unknown): void {
  if (el instanceof HTMLElement) virtualizer.value.measureElement(el);
}

function toggleRow(sha: string): void {
  props.review.toggle(sha);
}

const nav = useListNavigation<string>({
  container: rowsEl,
  items: () => shas.value,
  key: (sha) => sha,
  activeKey: focusedSha,
  onOpen: toggleRow,
  tree: {
    depth: () => 0,
    isExpanded: (sha) => props.review.expandedShas.value.has(sha),
    hasChildren: () => true,
    expand: (sha) => {
      if (!props.review.expandedShas.value.has(sha)) toggleRow(sha);
    },
    collapse: (sha) => {
      if (props.review.expandedShas.value.has(sha)) toggleRow(sha);
    },
  },
  scrollToIndex: (index) => virtualizer.value.scrollToIndex(index),
  rendered: () => virtualItems.value,
});

// Every diff opens in the host editor, so Escape only collapses the focused row.
useEventListener(document, 'keydown', (event: KeyboardEvent) => {
  if (event.key !== 'Escape') return;
  if (
    !props.boundary ||
    !(event.target instanceof Node) ||
    !props.boundary.contains(event.target)
  ) {
    return;
  }
  const sha = shas.value[focusedRow.value];
  if (sha && props.review.expandedShas.value.has(sha)) props.review.collapse(sha);
});

const PAGE_SIZE = SETTINGS['kiraSpace.graph.pageSize'].default;

function loadMoreLabel(): string {
  const r = props.review;
  if (r.isLoadingMore.value) return t('common.state.loading.loading');
  const remaining = r.remaining.value;
  return remaining < PAGE_SIZE
    ? t('git.loadMore.last.label', { count: remaining })
    : t('git.loadMore.page.label', {
        pageSize: formatCount(PAGE_SIZE),
        remaining: formatCount(remaining),
      });
}

function loadMore(): void {
  if (!props.review.isLoadingMore.value) void props.review.loadMore();
}
</script>

<template>
  <ViewToolbar
    v-if="review.staleReview.value"
    class="font-ui"
    role="status"
    data-testid="review-stale-banner"
  >
    <span>{{ t('git.review.stale.status') }}</span>
    <TooltipIconButton
      icon="refresh"
      :label="t('common.action.refresh.label')"
      class="ml-auto"
      @click="review.acknowledgeStaleReview()"
    />
  </ViewToolbar>

  <div
    ref="rowsEl"
    data-testid="review-commit-rows"
    class="flex-1 min-h-0 overflow-auto"
    role="tree"
    :aria-label="t('common.module.commits.label')"
    :tabindex="focusedRowMounted ? -1 : 0"
    @scroll="onScroll"
  >
    <div class="relative w-full" :style="{ height: `${totalSize}px` }">
      <div
        v-for="item in virtualItems"
        :key="shas[item.index]"
        :ref="measureRow"
        role="presentation"
        :data-index="item.index"
        :class="VIRTUAL_ROW_CLASS"
        :style="{ top: `${item.start}px` }"
      >
        <ReviewCommitRow
          v-bind="nav.rowProps(shas[item.index] as string)"
          :sha="shas[item.index] as string"
          :store="review.store"
          :expanded="review.expandedShas.value.has(shas[item.index] as string)"
          :expansion="review.expansionFor(shas[item.index] as string)"
          :actions="actions"
          :focused="item.index === focusedRow"
          :aria-setsize="shas.length"
          :aria-posinset="item.index + 1"
          :list-mode="listMode"
          :filter="filter"
          @toggle="toggleRow(shas[item.index] as string)"
          @focus-row="focusedSha = shas[item.index] as string"
        />
      </div>
    </div>
  </div>
  <!-- `remaining > 0` guards an empty comparison, which never emits a chunk. Kept visible while
       loading so the affordance does not vanish mid-load. -->
  <div
    v-if="!review.exhausted.value && (review.isLoadingMore.value || review.remaining.value > 0)"
    class="flex justify-center py-1 px-1.5 shrink-0"
  >
    <Button
      data-testid="review-load-more-button"
      variant="toolbar"
      size="kira"
      :pending="review.isLoadingMore.value"
      @click="loadMore"
      ><CodiconIcon name="fold-down" :size="13" />
      {{ loadMoreLabel() }}
    </Button>
  </div>
</template>
