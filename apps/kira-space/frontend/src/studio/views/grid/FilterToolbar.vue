<script setup lang="ts">
import type { SortSpec } from '@shared/domain/queries';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverAnchor } from '@theme/components/ui/popover';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import AutocompleteField from '@workbench/components/AutocompleteField.vue';
import { computed, ref, watch } from 'vue';
import type { DataTabRecord } from '../../../state/tabDomain';
import { control } from '../../bridge/control';
import { useConnectionsStore } from '../../state/connections';
import FilterHistoryMenu from '../shared/FilterHistoryMenu.vue';
import { identNeedsQuoting, quoteIdent, sqlDialectFor } from '../shared/sqlIdent';
import {
  orderByCandidates as buildOrderByCandidates,
  whereCandidates as buildWhereCandidates,
} from './filterCompletion';
import { useGridViewStore } from './state';

// P48 D10: takes `tab` as a prop like every other view's toolbar — see DataToolbar.vue's own note.
// SQL syntax samples, not wording.
const WHERE_EXAMPLE = "status = 'paid'";
const ORDER_BY_EXAMPLE = 'placed_at DESC';

const props = defineProps<{ tab: DataTabRecord }>();
const connectionsStore = useConnectionsStore();
const gridViewStore = useGridViewStore();

const rt = computed(() => gridViewStore.runtime[props.tab.id]);

// A query that failed is shown by the WHERE field turning error-red — the failure itself is
// already reported by DataView.vue's error strip, this just points at the field that caused it.
const hasError = computed(() => rt.value?.status === 'error');

// Mirrors PreviewCommandPanel.vue's/ConsoleView.vue's own three-line dialect computed exactly —
// undefined for every non-SQL connection kind, which filterCompletion.ts's dialect-conditional
// vocabularies (ILIKE, NULLS FIRST/LAST) already treat as "the non-Postgres list".
const dialect = computed(() =>
  sqlDialectFor(connectionsStore.connectionRecord(props.tab.connectionId)?.kind),
);
const whereCandidates = computed(() => buildWhereCandidates(props.tab.id, dialect.value));
const orderByCandidates = computed(() => buildOrderByCandidates(props.tab.id, dialect.value));

function sortToText(sort: SortSpec | null): string {
  if (!sort) return '';
  if (sort.kind === 'text') return sort.text;
  return sort.terms
    .map((t) => {
      const column = identNeedsQuoting(dialect.value, t.column)
        ? quoteIdent(dialect.value, t.column)
        : t.column;
      return `${column} ${t.direction.toUpperCase()}`;
    })
    .join(', ');
}

// Typed-but-unapplied text lives in the runtime store (P375): the tab-next/tab-prev shortcuts
// switch tabs without a blur, which would otherwise drop it.
const whereText = computed({
  get: () => gridViewStore.runtime[props.tab.id]?.whereDraft ?? props.tab.state.filter ?? '',
  set: (v: string) => gridViewStore.setFilterDraft(props.tab.id, { where: v }),
});
const orderByText = computed({
  get: () => gridViewStore.runtime[props.tab.id]?.orderByDraft ?? sortToText(props.tab.state.sort),
  set: (v: string) => gridViewStore.setFilterDraft(props.tab.id, { orderBy: v }),
});

// Watched as two separate primitive-returning getters, not `watch(tab, ...)` on the whole
// object: DataView.vue keys this component's ancestor by tab.id (remounted on tab switch, so
// `immediate: true` alone covers "just opened"/"tab changed"), but within one tab's lifetime
// `patchDataTabState` mutates `tab.state` in place rather than replacing the tab object — a
// non-deep watch on `tab` itself never sees that mutation. That silently broke this box after a
// column-header click (DataGrid.vue's onHeaderClick calls setSort, which does exactly this
// in-place patch): the ORDER BY field kept showing whatever was there before the click, so
// blurring it (or pressing Enter) re-applied the stale text and clobbered the header's sort.
// An external change to the committed value wins over any draft. Not immediate: a remount must
// keep the draft.
watch(
  () => props.tab.state.filter,
  () => gridViewStore.setFilterDraft(props.tab.id, { where: null }),
);
watch(
  () => props.tab.state.sort,
  () => gridViewStore.setFilterDraft(props.tab.id, { orderBy: null }),
);

function recordHistory(where: string | null, orderBy: SortSpec | null): void {
  if (!props.tab.connectionId) return;
  // A no-op filter (both null) is dropped server-side (D19) — nothing to special-case here.
  fireAndForget(
    control.queriesHistoryRecord(props.tab.connectionId, props.tab.path, where, orderBy),
    'recordFilterHistory',
  );
}

async function applyWhere(): Promise<void> {
  const value = whereText.value.trim() === '' ? null : whereText.value.trim();
  // A blur fires on every focus loss, not just an edit — re-applying an unchanged WHERE would
  // reset paging/count for no reason (and, worse, race an in-flight runCount for this same filter).
  if (value === (props.tab.state.filter ?? null)) {
    gridViewStore.setFilterDraft(props.tab.id, { where: null });
    return;
  }
  await gridViewStore.setFilter(props.tab.id, value);
  gridViewStore.setFilterDraft(props.tab.id, { where: null });
  recordHistory(value, props.tab.state.sort);
}

async function applyOrderBy(): Promise<void> {
  const text = orderByText.value.trim();
  const sort: SortSpec | null = text === '' ? null : { kind: 'text', text };
  await gridViewStore.setSort(props.tab.id, sort);
  gridViewStore.setFilterDraft(props.tab.id, { orderBy: null });
  recordHistory(props.tab.state.filter, sort);
}

// README's "the filter row is permanent — Clear, never close": empties both fields and refetches,
// using the same setFilter/setSort the blur handlers already call — there is no separate "hide
// the row" affordance to build, since the row never goes away.
async function onClear(): Promise<void> {
  gridViewStore.setFilterDraft(props.tab.id, { where: '', orderBy: '' });
  await gridViewStore.setFilter(props.tab.id, null);
  await gridViewStore.setSort(props.tab.id, null);
  gridViewStore.setFilterDraft(props.tab.id, { where: null, orderBy: null });
  recordHistory(null, null);
}

// AutocompleteField's own @escape only ever fires once its suggestion dropdown is already closed
// (an open one consumes Escape itself, to dismiss just the dropdown) — so by the time this runs,
// focus is still genuinely on the field itself, and blurring the active element is exactly
// blurring it.
function onWhereEscape(): void {
  gridViewStore.setFilterDraft(props.tab.id, { where: null });
  (document.activeElement as HTMLElement | null)?.blur();
}
function onOrderByEscape(): void {
  gridViewStore.setFilterDraft(props.tab.id, { orderBy: null });
  (document.activeElement as HTMLElement | null)?.blur();
}

const historyOpen = ref(false);
// P104: PopoverAnchor's own `:reference` takes the trigger's real DOM node directly (the
// established `.$el` idiom, e.g. ConsoleView.vue's savedMenuTriggerEl).
const historyTriggerEl = ref<{ $el: HTMLElement } | null>(null);

function applyFromHistory(where: string | null, orderBy: SortSpec | null): void {
  gridViewStore.setFilterDraft(props.tab.id, { where: where ?? '', orderBy: sortToText(orderBy) });
  void gridViewStore
    .setFilter(props.tab.id, where)
    .then(() => gridViewStore.setSort(props.tab.id, orderBy))
    .then(() => gridViewStore.setFilterDraft(props.tab.id, { where: null, orderBy: null }));
}
</script>

<template>
  <!-- LAW 02 / README: one row, two prefixed inputs, one verb — permanent, so Clear rather than
       a close button that would make the grid change height under you. -->
  <div class="relative">
    <TooltipIconButton
      :icon="ICON.history"
      :label="t('studio.common.savedAndRecentFilters.label')"
      ref="historyTriggerEl"
      :pressed="historyOpen"
      data-testid="filter-history-button"
      @click="historyOpen = !historyOpen"
    />
    <Popover :open="historyOpen" @update:open="(v) => (historyOpen = v)">
      <PopoverAnchor
        :reference="(historyTriggerEl?.$el as HTMLElement) ?? undefined"
        class="hidden"
      />
      <FilterHistoryMenu
        v-if="historyOpen"
        :connection-id="tab.connectionId"
        :path="tab.path"
        :current-filter="tab.state.filter"
        :current-sort="tab.state.sort"
        @apply="applyFromHistory"
        @close="historyOpen = false"
      />
    </Popover>
  </div>
  <div class="flex-1 min-w-0">
    <AutocompleteField
      v-model="whereText"
      class="w-full"
      prefix="WHERE"
      :prefix-active="!!tab.state.filter"
      :placeholder="WHERE_EXAMPLE"
      data-testid="filter-where-input"
      :invalid="hasError"
      :candidates="whereCandidates"
      language="sql"
      :sql-dialect="dialect"
      @enter="applyWhere"
      @escape="onWhereEscape"
      @blur="applyWhere"
    />
  </div>
  <div class="w-56 shrink-0">
    <AutocompleteField
      v-model="orderByText"
      class="w-full"
      prefix="ORDER BY"
      :prefix-active="!!tab.state.sort"
      :placeholder="ORDER_BY_EXAMPLE"
      data-testid="filter-orderby-input"
      :candidates="orderByCandidates"
      language="sql"
      :sql-dialect="dialect"
      @enter="applyOrderBy"
      @escape="onOrderByEscape"
      @blur="applyOrderBy"
    />
  </div>
  <Tooltip>
    <TooltipTrigger as-child>
      <Button variant="toolbar" size="kira" @click="onClear">{{
        t('common.action.clear.label')
      }}</Button>
    </TooltipTrigger>
    <TooltipContent>{{ t('studio.common.clearFilters.tooltip') }}</TooltipContent>
  </Tooltip>
</template>
