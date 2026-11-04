<script setup lang="ts">
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
/**
 * G-UX (item 4): "current changes on a branch aren't visible in the git graph at all" — a small,
 * pinned strip above the graph, not a synthetic row inside it. The literal ask (a GitLens-style
 * virtual row in-line with real commits) was rejected for v1: "grid row index" and "store row
 * index" are the same untyped integer in ~25 places across `columns.ts`/`graphColumn.ts`/
 * `layoutStore.ts`/`CommitGrid.vue`/`state/selection.ts`/`App.vue` (including a persisted
 * scroll-row value saved across sessions) — inserting a row upstream of real data would mean
 * converting every one of those to an explicit grid-row<->store-row mapping, its own project. This
 * sits entirely outside SlickGrid's coordinate space instead: a `flex-shrink: 0` sibling mounted
 * above `<CommitGrid>` in `App.vue`'s own flex-column `graph-region`, so none of those ~25
 * row-index assumptions are touched.
 *
 * Data: `OpsState.statusSummary` — already reactive, already refreshed on every `repo.changed`
 * (`OpsState`'s own constructor). Shown only while `!isClean`.
 *
 * Alignment: a bounded scan of the first `HEAD_SCAN_LIMIT` loaded rows for the HEAD decoration —
 * the same `isHeadDecoration` single source of truth `graphColumn.ts`'s own formatter already
 * reads, so this can never disagree with which row the real graph bolds — then that row's lane
 * via `GraphViewState.layout`, positioned with the exact `laneX` math
 * `rowSvg.ts`/`geometry.ts` use for the real graph column, so the two line up visually. The node
 * glyph itself reuses `rowSvg.ts`'s own stash shape (an unfilled dashed ring, lane-coloured) —
 * dashed reads as "not a real commit" the same way it already does for a stash entry.
 *
 * P7 (item 2): now clickable — a real `<button>`, not a hand-rolled `role="button"` div, so focus/
 * `Enter`/`Space` activation and the accessible name (its own text content) all come from the
 * platform rather than being reimplemented. `role="status"` is dropped from the outer element for
 * exactly that reason: a live-region role on a focusable, interactive control is the wrong ARIA
 * shape (the count updating is no longer the only thing this element does). `@click` emits
 * `select`, which `App.vue` routes to `WorkingDetailState.select(true)` — the IPC method the
 * original v1 note said this needed, `working.detail`, now exists.
 */
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { computed, onBeforeUnmount, ref } from 'vue';
import { GEOMETRY } from '../graph/geometry.ts';
import { laneClass } from '../graph/palette.ts';
import { isHeadDecoration, laneX } from '../graph/rowSvg.ts';
import type { GraphViewState } from '../state/graphView.ts';
import type { OpsState } from '../state/ops.ts';

const props = defineProps<{
  graphView: GraphViewState;
  opsState: OpsState;
  /** The grid's user-set graph column width, so the strip's label lines up with its message
   *  column. */
  graphWidth: number;
}>();

const emit = defineEmits<{
  /** P7 (item 2): the strip was clicked/activated — `App.vue` routes this to
   *  `WorkingDetailState.select(true)`, mirroring how a stash row's click routes to
   *  `stashState.select(sha)`. */
  select: [];
}>();

/** Cheap: the checked-out branch's own HEAD decoration is essentially always within the first
 *  page of history (the newest commits) — a bounded scan, never a search over the whole loaded
 *  store. If HEAD has not loaded yet (a very deep, unusual case), the strip shows no node
 *  glyph until it does — see `headRow` below. */
const HEAD_SCAN_LIMIT = 200;

/** `GraphViewState.layout` is driven by `onPlanPublished` callbacks, not a reactive ref (that
 *  class's own doc comment: "Layout is driven from the append, not from the render") — bumped on
 *  every publish so `headLane`/`headColor` below recompute once a row's lane actually
 *  arrives, which can land a tick after the row's own text does (W5's "text first, graph a frame
 *  later"). */
const layoutTick = ref(0);
const unsubscribeLayout = props.graphView.onPlanPublished(() => {
  layoutTick.value++;
});
onBeforeUnmount(unsubscribeLayout);

const summary = computed(() => props.opsState.statusSummary.value);
const visible = computed(() => summary.value !== undefined && !summary.value.isClean);

const headRow = computed<number | undefined>(() => {
  // Establishes a dependency on generation/loadedRows so a reset or a freshly streamed page
  // re-scans — `store`/`layout` themselves are markRaw and never trigger this computed on their
  // own (GraphViewState's own doc comment on why only the scalars are reactive).
  void props.graphView.generation.value;
  const currentPlan = props.graphView.plan.value;
  const limit = Math.min(props.graphView.loadedRows.value, HEAD_SCAN_LIMIT);
  const store = props.graphView.store;
  for (let row = 0; row < limit; row++) {
    if (store.decorationAt(row).some(isHeadDecoration)) {
      // `layout` is keyed by display row, not store row.
      const displayRow = currentPlan.displayRowOf(row);
      return displayRow >= 0 ? displayRow : undefined;
    }
  }
  return undefined;
});

const headLane = computed<number | undefined>(() => {
  void layoutTick.value;
  const row = headRow.value;
  if (row === undefined || !props.graphView.layoutCurrent) return undefined;
  const layout = props.graphView.layout;
  if (row >= layout.rowCount) return undefined;
  return layout.laneOf(row);
});

const headColor = computed<number | undefined>(() => {
  void layoutTick.value;
  const row = headRow.value;
  if (row === undefined || !props.graphView.layoutCurrent) return undefined;
  const layout = props.graphView.layout;
  if (row >= layout.rowCount) return undefined;
  return layout.colorOf(row);
});

const nodeCx = computed(() => (headLane.value !== undefined ? laneX(headLane.value) : undefined));
const laneClassName = computed(() =>
  headColor.value !== undefined ? laneClass(headColor.value) : undefined,
);
const dashArray = `${GEOMETRY.strokeWidth} ${GEOMETRY.strokeWidth}`;

const totalCount = computed(() => {
  const counts = summary.value?.counts;
  if (!counts) return 0;
  return counts.staged + counts.unstaged + counts.untracked + counts.unmerged;
});

const tooltipText = computed(() => {
  const s = summary.value;
  if (!s) return '';
  const lines = s.dirtyPaths.join('\n');
  return s.dirtyTruncated ? `${lines}\n…` : lines;
});
</script>

<template>
  <button
    v-if="visible"
    type="button"
    :class="
      cn(
        rowVariants({ layout: 'tree' }),
        'shrink-0 w-full min-h-0 h-graph-row-compact bg-bg text-inherit text-left',
      )
    "
    data-testid="uncommitted-strip"
    @click="emit('select')"
  >
    <div
      class="shrink-0 h-full flex items-center overflow-visible"
      :style="{ width: `${graphWidth}px` }"
    >
      <svg
        v-if="nodeCx !== undefined"
        class="overflow-visible"
        aria-hidden="true"
        :width="graphWidth"
        height="100%"
      >
        <circle
          :cx="nodeCx"
          cy="50%"
          :r="GEOMETRY.nodeRadius"
          :class="laneClassName"
          :stroke-width="GEOMETRY.strokeWidth"
          :stroke-dasharray="dashArray"
          style="fill: none"
        />
      </svg>
    </div>
    <Tooltip>
      <TooltipTrigger as-child>
        <span
          class="min-w-0 truncate text-muted-foreground text-graph-sm"
          data-testid="uncommitted-strip-label"
        >
          {{ t('git.strip.uncommittedCount.label', { count: totalCount }) }}
        </span>
      </TooltipTrigger>
      <TooltipContent>{{ tooltipText }}</TooltipContent>
    </Tooltip>
  </button>
</template>
