<script setup lang="ts">
import type { PageSize } from '@shared/domain/tabs';
import { useQuery } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverAnchor } from '@theme/components/ui/popover';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import ToolbarSeparator from '@workbench/components/ToolbarSeparator.vue';
import { computed, ref } from 'vue';
import { useConnectionsStore } from '../../state/connections';
import { useFakeDataStore } from '../../state/fakeData';
import { loadMaskRuleCounts, maskRuleCountsQueryKey } from '../../state/maskRules';
import type { DataTabRecord } from '../../state/tabDomain';
import PagerControls from '../shared/page/PagerControls.vue';
import { pageSizeOptions } from '../shared/page/sizes';
import ColumnsMenu from './ColumnsMenu.vue';
import { canGenerateDataFor } from './fakeData/generate';
import { getPage, pageVersion } from './page';
import { usePendingChangesStore } from './pendingChanges';
import { matchedRows } from './search';
import { rowsForSelection } from './slick/rowValues';
import { useGridViewStore } from './state';

// Takes `tab` as a prop like every other view's toolbar, rather than reading the
// nullable, globally-computed activeDataTab — this component only ever renders while its tab is
// the active one (DataView.vue's own v-else-if chain), so the two agreed in practice, but the
// nullable-tab plumbing this used to need (fourteen `if (!tab.value) return` guards) existed only
// because of the divergence, not a real case.
const fakeDataStore = useFakeDataStore();
const pendingChangesStore = usePendingChangesStore();
const connectionsStore = useConnectionsStore();
const gridViewStore = useGridViewStore();
const props = defineProps<{ tab: DataTabRecord }>();

// P24 D30: SegmentedControl's generic now covers a numeric union too, so this hand-rolled .p-seg
// (kept only because two leaks.spec.ts assertions read .active, since fixed) can be the primitive.
const PAGE_SIZE_OPTIONS = pageSizeOptions('');
const PAGE_SIZE_ITEMS = PAGE_SIZE_OPTIONS.map((o) => ({
  value: String(o.value),
  label: o.label,
  testid: o.testid,
}));

const rt = computed(() => gridViewStore.runtime[props.tab.id]);
const stale = computed(() => gridViewStore.isStale(props.tab.id));

const caps = computed(() => {
  const connectionId = props.tab.connectionId;
  return connectionId ? (connectionsStore.states[connectionId]?.caps ?? null) : null;
});

// Add is gated on writability alone — never on whether the table has a primary key. A no-PK table
// still rejects at the per-cell edit level (readOnlyReasonFor) and at the server
// (assertKeyIsPrimaryKey); gating that button too would be a second, redundant guard.
// Delete is different: it has no per-cell rejection path to fall
// through to — it stages straight into pendingChanges — so an unactionable click there is silent.
//
// `&& !rt.value?.maskPreview` is the edit lockout's own toolbar half — SlickGridHost.vue's
// canEditTable() closes the grid's own inline paths, this one closes Add/Generate/Delete, all of
// which derive from this single computed.
const isWritable = computed(
  () =>
    !!caps.value?.writable &&
    !connectionsStore.connectionRecord(props.tab.connectionId)?.readOnly &&
    !rt.value?.maskPreview &&
    !stale.value &&
    !pendingChangesStore.isCommitting(props.tab.id),
);

// Rendered only when this tab's connection has at least one masked column — a
// permanently inert toggle is worse than no toggle (deleteRowTooltip's own standing rule, just
// below: name the condition, never a silently-disabled control with nothing to explain it).
// Reactive useQuery over the counts TanStack Query already warms at bootstrap
// (main.ts's loadMaskRuleCounts) and keeps in lockstep on every loadMaskRules write, so this still
// updates in the same tick menu.ts's markColumnMaskKind also calls setMaskPreview(tabId, true).
const maskRuleCountsQuery = useQuery({
  queryKey: maskRuleCountsQueryKey,
  queryFn: loadMaskRuleCounts,
});
const hasMaskRules = computed(
  () => (maskRuleCountsQuery.data.value?.[props.tab.connectionId ?? ''] ?? 0) > 0,
);
// §6.4: turning the preview on while something is staged would mask the very text the user
// staged, with no way to tell staged text from stored text inside the transform — simpler to
// forbid the combination outright than to build that distinction for a case nobody needs.
const hasPendingChanges = computed(() => pendingChangesStore.hasPending(props.tab.id));
const maskPreviewTooltip = computed(() => {
  if (hasPendingChanges.value) return t('studio.grid.maskPreview.pendingFirst.tooltip');
  return t(
    rt.value?.maskPreview
      ? 'studio.grid.maskPreview.off.tooltip'
      : 'studio.grid.maskPreview.on.tooltip',
  );
});
const addRowTooltip = computed(() => {
  if (isWritable.value) return t('studio.grid.addRow.label');
  if (stale.value) return t('studio.common.connectionLost.tooltip');
  return t(
    rt.value?.maskPreview
      ? 'studio.grid.maskedEdit.tooltip'
      : 'studio.common.connectionReadOnly.tooltip',
  );
});
// A failed refresh keeps the old count on screen; a failed first count has none to keep.
const countTooltip = computed(() => {
  const count = rt.value?.count;
  const error = rt.value?.countError;
  if (!count) {
    return error
      ? t('studio.common.countFailed.tooltip', { error })
      : t('studio.grid.count.tooltip');
  }
  const total = t(
    count.exact ? 'studio.grid.count.exact.tooltip' : 'studio.grid.count.approx.tooltip',
    { count: count.value },
  );
  return [
    total,
    count.stale ? t('studio.grid.count.stale.tooltip') : null,
    error ? t('studio.grid.count.refreshFailed.tooltip', { error }) : null,
  ]
    .filter(Boolean)
    .join('. ');
});
function onToggleMaskPreview(): void {
  if (hasPendingChanges.value) return;
  gridViewStore.toggleMaskPreview(props.tab.id);
}

// The − row button's own gate — ClickHouse is writable (canInsert: true) but has no
// addressable row to DELETE (a MergeTree PRIMARY KEY is a sparse index, not a unique key), so
// isWritable alone is no longer enough to offer this button.
// Also gated on a selection and a primary key. onDeleteRow below opens with
// `if (!sel) return`, and stageDelete stages an op that can never resolve without a primary key
// — so before this the button was enabled and silently inert in both cases, which is the reported
// "deleting selected rows doesn't work at all". SlickGridHost's own canDeleteRows() has always
// required hasPrimaryKey; this is the same predicate, plus the selection the toolbar needs and the
// grid's own context menu does not (it acts on the row that was right-clicked).
// `runtime` is a real reactive() map (views/shared/viewOp.ts), so this needs no version read —
// unlike hasPrimaryKey below, whose getPage() reads a plain Map and depends on pageVersion.n.
const hasSelection = computed(() => !!gridViewStore.runtime[props.tab.id]?.selection);
const hasPrimaryKey = computed(() => {
  void pageVersion.n;
  return getPage(props.tab.id)?.columns.some((c) => c.isPrimaryKey) ?? false;
});
const canDeleteRows = computed(
  () => isWritable.value && !!caps.value?.canDelete && hasPrimaryKey.value && hasSelection.value,
);
// A disabled control names the actual condition rather than going grey —
// KeyValueView.vue's own standing rule ("the actual number or the actual condition, never a
// silently-disabled control with no explanation"). Ordered most-fundamental first: a read-only
// connection or an engine that cannot delete is a property of the connection, a missing primary
// key a property of the table, and an empty selection the one the user can act on right now.
const deleteRowTooltip = computed(() => {
  if (canDeleteRows.value) return t('studio.grid.deleteRows.aria');
  if (stale.value) return t('studio.common.connectionLost.tooltip');
  if (rt.value?.maskPreview) return t('studio.grid.maskedEdit.tooltip');
  if (!isWritable.value) return t('studio.common.connectionReadOnly.tooltip');
  if (!caps.value?.canDelete) return t('studio.grid.deleteUnsupported.tooltip');
  if (!hasPrimaryKey.value) return t('studio.grid.noPrimaryKey.tooltip');
  return t('studio.grid.selectRowsFirst.tooltip');
});

// M5 §6.4: canGenerateDataFor doesn't know about mask preview (fakeData/generate.ts's own
// predicate, reused unchanged by DataView.vue's palette entry too) — checked separately here, and
// named ahead of every reason that predicate itself covers.
const canGenerateData = computed(
  () =>
    canGenerateDataFor(
      caps.value,
      connectionsStore.connectionRecord(props.tab.connectionId)?.readOnly,
    ) &&
    !rt.value?.maskPreview &&
    !stale.value,
);
const generateDataTooltip = computed(() => {
  if (canGenerateData.value) return t('studio.shortcuts.palette.generateData.label');
  if (stale.value) return t('studio.common.connectionLost.tooltip');
  if (rt.value?.maskPreview) return t('studio.grid.maskedEdit.tooltip');
  if (connectionsStore.connectionRecord(props.tab.connectionId)?.readOnly)
    return t('studio.common.connectionReadOnly.tooltip');
  return t('studio.grid.generateUnsupported.tooltip');
});

function onFirst(): void {
  void gridViewStore.goFirst(props.tab.id);
}
function onPrev(): void {
  void gridViewStore.goPrev(props.tab.id);
}
function onNext(): void {
  void gridViewStore.goNext(props.tab.id);
}
function onLast(): void {
  void gridViewStore.goLast(props.tab.id);
}
function onJump(pageIndex: number): void {
  void gridViewStore.goToPage(props.tab.id, pageIndex);
}
function onCount(): void {
  void gridViewStore.runCount(props.tab.id);
}
function onPageSize(size: PageSize): void {
  void gridViewStore.setPageSize(props.tab.id, size);
}
function onToggleSearch(): void {
  gridViewStore.toggleSearchOpen(props.tab.id);
}
function onGenerateData(): void {
  if (!canGenerateData.value) return;
  fakeDataStore.openGenerateDataDialog(props.tab.id);
}

const columnsOpen = ref(false);
const columnsAnchorRef = ref<HTMLElement | null>(null);

// P16 design system's count Badge on the Columns button: "selected / total" — both counts
// already live on data this component reads anyway (the projection list and the
// describe-derived meta), so this is a display-only derivation, not a new data source.
const columnCount = computed(() => {
  const total = rt.value?.meta?.columns.length;
  if (!total) return null;
  const projection = props.tab.state.projection;
  return { shown: projection ? projection.length : total, total };
});

// The Columns button's own corner mark — shown only when the tab's column set
// deviates from default (some columns hidden, or the order was dragged), so the icon stays clean
// until there's something to flag. A plain dot, not a count: "5 / 5" (every column still
// shown, only the order changed) reads as a count that says nothing, and a four-character label
// in a 22px button's corner was never going to fit regardless of which deviation triggered it —
// the exact numbers already live in the tooltip below (columnCount), which is where every
// other detail in this icon-only toolbar lives.
const columnsIndicator = computed(() => {
  const state = props.tab.state;
  return state.projection !== null || state.columnOrder !== null;
});

function onAddRow(): void {
  const p = getPage(props.tab.id);
  if (!p) return;
  // P36 D28: a generated column is never seeded — the server computes it, and an explicit NULL
  // for it would make the insert fail outright on an engine that enforces this (F18).
  pendingChangesStore.addInsertRow(
    props.tab.id,
    p.columns.filter((c) => !c.generated).map((c) => c.name),
  );
}

// A selected row/cell/range/column at or beyond the page's real row count addresses an appended
// pending-insert row (DataGrid.vue's synthetic row indices) — deleting one of those discards it
// outright rather than staging a delete op that could never resolve to a real primary key.
//
// This used to hand-roll its own row/cell/range switch — a third
// copy of the same conversion SlickGridHost.vue's onKeydown and onGridContextMenu each used to
// hand-roll too — and, like both of those, never grew a `column` branch, so this button did
// nothing for a column selection. Routed through `rowsForSelection` (slick/rowValues.ts) now, the
// same one every other "act on the current selection" call site uses, so a selection kind newly
// needs coverage exactly once, not here as well.
function onDeleteRow(): void {
  const r = gridViewStore.runtime[props.tab.id];
  const sel = r?.selection;
  if (!sel) return;
  const p = getPage(props.tab.id);
  const rowCount = p?.rowCount ?? 0;
  const rows = rowsForSelection(sel, matchedRows(props.tab.id), rowCount);

  const realRows = rows.filter((row) => row < rowCount);
  if (realRows.length) pendingChangesStore.stageDelete(props.tab.id, realRows);

  const inserts = pendingChangesStore.pendingFor(props.tab.id)?.inserts ?? [];
  for (const row of rows.filter((row) => row >= rowCount)) {
    const insert = inserts[row - rowCount];
    if (insert) pendingChangesStore.discardInsertRow(props.tab.id, insert.id);
  }
}
</script>

<template>
  <!-- LAW 01/10: ViewChrome's own Refresh/Stop group renders ahead of this slot's content, so
       the leading sep below matches every other view's #toolbar (KeyValueView.vue,
       DocumentView.vue) rather than assuming its own hand-rolled equivalent (F1/F3). -->
  <ToolbarSeparator />

  <!-- By user report: navigation belongs beside the page-size picker it pages through, at the
       toolbar's reading edge, not alone at the far right. The pager is a jump-to-page input rather
       than a "row 1-200 of N" readout — the cursor/offset paging has no notion of that until the
       count query has run. Deliberately not applied to DocumentView.vue, whose placement was not
       reported. -->
  <PagerControls
    :page-index="tab.state.pageIndex"
    :page-size="tab.state.pageSize"
    :count="rt?.count?.value ?? null"
    :has-more="!!rt?.hasMore"
    testid-prefix=""
    :last-tooltip="t('studio.grid.pager.countFirst.tooltip')"
    :strategy="rt?.lastStrategy"
    @first="onFirst"
    @prev="onPrev"
    @next="onNext"
    @last="onLast"
    @jump="onJump"
  />

  <SecondaryTabs
    variant="segmented"
    :model-value="String(tab.state.pageSize)"
    :items="PAGE_SIZE_ITEMS"
    data-testid="page-size-picker"
    @update:model-value="(v) => onPageSize(Number(v) as PageSize)"
  />

  <ToolbarSeparator />

  <div class="flex items-center gap-1.5 min-w-0">
    <Tooltip>
      <TooltipTrigger as-child>
        <Button
          variant="toolbar"
          size="kira-icon"
          data-testid="toolbar-count"
          :class="rt?.countError ? 'text-error' : rt?.count?.stale ? 'text-warn' : ''"
          :aria-label="t('studio.grid.count.tooltip')"
          @click="onCount"
        >
          <CodiconIcon name="symbol-number" :size="13" />
        </Button>
      </TooltipTrigger>
      <TooltipContent data-testid="toolbar-count-tooltip">
        {{
          // F13 (P108 Part 10): a failed count is no longer silent — setFilter nulls `count`
          // before a fresh count runs, so a failure right after has no old count to keep showing;
          // a failed *refresh* of an existing count keeps that count on screen (unchanged) with
          // the error appended, rather than losing it.
          countTooltip
        }}
      </TooltipContent>
    </Tooltip>

    <Popover :open="columnsOpen" @update:open="columnsOpen = $event">
      <div ref="columnsAnchorRef" class="relative">
        <Tooltip>
          <TooltipTrigger as-child>
            <Button
              variant="toolbar"
              size="kira-icon"
              class="relative"
              data-testid="toolbar-columns"
              :aria-label="t('studio.common.columns.label')"
              :aria-expanded="columnsOpen"
              @click="columnsOpen = !columnsOpen"
            >
              <CodiconIcon name="list-selection" :size="13" />
              <StatusDot
                v-if="columnsIndicator"
                tone="state-on"
                class="absolute top-0.5 right-0.5"
              />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{
            columnCount
              ? t('studio.grid.columns.shown.tooltip', columnCount)
              : t('studio.common.columns.label')
          }}</TooltipContent>
        </Tooltip>
        <PopoverAnchor :reference="columnsAnchorRef ?? undefined" />
      </div>
      <ColumnsMenu v-if="columnsOpen" :tab-id="tab.id" :caps="caps" />
    </Popover>
  </div>

  <ToolbarSeparator />

  <div class="flex items-center gap-1.5 min-w-0">
    <TooltipIconButton
      icon="add"
      :label="addRowTooltip"
      :aria-label="t('studio.grid.addRow.label')"
      disabled-trigger
      data-testid="toolbar-add-row"
      :disabled="!isWritable"
      @click="onAddRow"
    />
    <TooltipIconButton
      icon="wand"
      :label="generateDataTooltip"
      :aria-label="t('studio.shortcuts.palette.generateData.label')"
      disabled-trigger
      data-testid="toolbar-generate-data"
      :disabled="!canGenerateData"
      @click="onGenerateData"
    />
    <TooltipIconButton
      icon="trash"
      :label="deleteRowTooltip"
      :aria-label="t('studio.grid.deleteRows.aria')"
      disabled-trigger
      data-testid="toolbar-delete-row"
      :disabled="!canDeleteRows"
      @click="onDeleteRow"
    />
    <TooltipIconButton
      icon="search"
      :label="t('studio.common.searchPage.label')"
      :pressed="!!rt?.searchOpen"
      data-testid="toolbar-search"
      @click="onToggleSearch"
    />
    <TooltipIconButton
      v-if="hasMaskRules"
      icon="eye-closed"
      :label="maskPreviewTooltip"
      :aria-label="t('studio.grid.maskPreview.aria')"
      disabled-trigger
      :pressed="!!rt?.maskPreview"
      :disabled="hasPendingChanges"
      data-testid="toolbar-mask-preview"
      @click="onToggleMaskPreview"
    />
  </div>
</template>
