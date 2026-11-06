<script setup lang="ts">
import { grpcCodeClass, grpcCodeHint } from '@shared/domain/grpc';
import {
  GRPC_HISTORY_PER_SCOPE_LIMIT,
  type GrpcCallHistoryEntry,
} from '@shared/domain/grpcHistory';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { formatRelative } from '@workbench/behaviour/format';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { matchesQuery } from '@workbench/behaviour/match';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref } from 'vue';
import type { GrpcRequestTabRecord } from '../../../state/tabDomain';
import { useTabIncognitoStore } from '../../state/tabIncognito';
import { useGrpcCallHistoryStore, useGrpcHistoryList, useGrpcHistoryViewing } from './history';

const confirmDialogStore = useConfirmDialogStore();
const tabIncognitoStore = useTabIncognitoStore();
const grpcCallHistoryStore = useGrpcCallHistoryStore();

// Mirroring views/httprequest/ResponseHistoryList.vue's shape exactly — a real toolbar (count +
// Clear, the clearGrpcHistory finally reachable), rows as a <div> rather than a <button> nested
// inside a <button> (invalid HTML the parser silently hoisted the delete control out of),
// is-viewing highlighting, and formatRelative instead of an absolute clock time.
//
// No Compare: HTTP's rides Monaco's diff editor over two response *bodies*; a gRPC call is a
// message *sequence* with metadata, a genuinely different design left for a future row.
const props = defineProps<{ tab: GrpcRequestTabRecord }>();

const { query: listQuery, entries } = useGrpcHistoryList(() => props.tab);
const { viewingId, error: snapshotError } = useGrpcHistoryViewing(() => props.tab.id);
const errorMessage = computed(
  () =>
    grpcCallHistoryStore.ui[props.tab.id]?.actionError ??
    listQuery.error.value?.message ??
    snapshotError.value?.message ??
    null,
);
// P18 D6: HTTP's own "the list is full" predicate, restated for gRPC's cap.
const atCap = computed(() => entries.value.length >= GRPC_HISTORY_PER_SCOPE_LIMIT);
// P71 §3.5: ResponseHistoryList.vue's own explanation for the silence, restated for gRPC.
const incognito = computed(() => tabIncognitoStore.isIncognito(props.tab.id));

// P22b D14: HTTP's own ResponseHistoryList.vue idiom (P16 D15) — an always-visible filter box
// above the list, not a toggle (a compact history pane, not a big document). Matches method,
// status name, and time — case-folded substring, the fields already on screen in each row.
const filterQuery = computed({
  get: () => grpcCallHistoryStore.ui[props.tab.id]?.filterQuery ?? '',
  set: (v: string) => grpcCallHistoryStore.setFilterQuery(props.tab.id, v),
});
const isFiltered = computed(() => filterQuery.value.trim() !== '');
const filteredEntries = computed<GrpcCallHistoryEntry[]>(() => {
  const q = filterQuery.value;
  return entries.value.filter(
    (e) =>
      matchesQuery(e.method, q) ||
      matchesQuery(e.codeName, q) ||
      matchesQuery(formatRelative(e.calledAt), q),
  );
});

function onRowClick(id: string): void {
  void grpcCallHistoryStore.viewGrpcHistoryEntry(props.tab.id, id);
}

// The list navigation claims Enter and the arrows; Space mirrors a click. Controls nested in a row
// stay their own tab stops.
function onRowKeydown(e: KeyboardEvent, id: string): void {
  if (e.key !== ' ' || e.target !== e.currentTarget) return;
  e.preventDefault();
  onRowClick(id);
}

const listEl = ref<HTMLElement | null>(null);
const activeKey = ref<string | null>(null);
const nav = useListNavigation<{ id: string }>({
  container: listEl,
  items: () => filteredEntries.value,
  key: (entry) => entry.id,
  activeKey,
  onSelect: (entry) => onRowClick(entry.id),
});

function onDelete(id: string): void {
  void grpcCallHistoryStore.deleteGrpcHistoryEntry(props.tab.id, id);
}

async function onClear(): Promise<void> {
  const ok = await confirmDialogStore.confirm({
    title: t('studio.grpc.history.clearConfirm.title'),
    message: [
      t('studio.grpc.history.clearConfirm.body'),
      t('common.confirm.irreversible.body'),
    ].join(' '),
    confirmLabel: t('common.action.clear.label'),
    tone: 'danger',
  });
  if (ok) await grpcCallHistoryStore.clearGrpcHistory(props.tab.id);
}
</script>

<template>
  <div class="flex flex-1 min-h-0 flex-col" data-testid="grpc-history-list">
    <ViewToolbar>
      <span class="text-kira-sm text-subtle">{{
        t('studio.grpc.history.count.status', { count: entries.length })
      }}</span>
      <span class="ml-auto" />
      <Button
        variant="toolbar"
        size="kira"
        :disabled="entries.length === 0"
        data-testid="grpc-history-clear"
        @click="onClear"
      >
        <CodiconIcon :name="ICON.delete" :size="13" />
        {{ t('common.action.clearHistory.label') }}
      </Button>
    </ViewToolbar>

    <Alert v-if="errorMessage" variant="error">
      <AlertDescription>{{ errorMessage }}</AlertDescription>
    </Alert>
    <EmptyState
      v-else-if="entries.length === 0"
      :icon="ICON.history"
      :title="incognito ? t('studio.grpc.history.incognito.empty') : t('studio.grpc.history.empty')"
    />

    <template v-else>
      <SearchField
        v-model="filterQuery"
        :placeholder="t('studio.common.filterHistory.placeholder')"
        data-testid="grpc-history-filter"
      />
      <EmptyState
        v-if="isFiltered && filteredEntries.length === 0"
        data-testid="grpc-history-filter-empty"
        :icon="ICON.search"
        :title="t('common.state.noMatches.empty')"
      />
    </template>

    <div
      v-if="entries.length > 0 && filteredEntries.length > 0"
      class="flex flex-1 min-h-0 flex-col overflow-auto"
      ref="listEl"
      role="listbox"
      :aria-label="t('studio.grpc.history.list.aria')"
    >
      <div
        v-for="entry in filteredEntries"
        :key="entry.id"
        :class="cn(rowVariants({ layout: 'tree', selected: entry.id === viewingId }), 'py-1')"
        data-testid="grpc-history-row"
        role="option"
        v-bind="nav.rowProps(entry.id)"
        :tabindex="nav.tabindexFor(entry.id)"
        :aria-selected="entry.id === viewingId"
        @click="onRowClick(entry.id)"
        @keydown="onRowKeydown($event, entry.id)"
      >
        <Tooltip>
          <TooltipTrigger as-child>
            <Badge data :variant="grpcCodeClass(entry.code)">{{ entry.codeName }}</Badge>
          </TooltipTrigger>
          <TooltipContent>{{ grpcCodeHint(entry.code) }}</TooltipContent>
        </Tooltip>
        <span class="text-kira-sm font-data">{{ entry.method }}</span>
        <Tooltip>
          <TooltipTrigger as-child>
            <span class="text-kira-sm text-subtle">{{ formatRelative(entry.calledAt) }}</span>
          </TooltipTrigger>
          <TooltipContent>{{ entry.calledAt }}</TooltipContent>
        </Tooltip>
        <span class="ml-auto" />
        <TooltipIconButton
          :icon="ICON.delete"
          :label="t('common.action.delete.label')"
          data-testid="grpc-history-delete"
          @click.stop="onDelete(entry.id)"
        />
      </div>
    </div>

    <div
      v-if="atCap"
      class="text-kira-sm text-subtle shrink-0 px-1.5 py-1"
      data-testid="grpc-history-cap-note"
    >
      {{ t('studio.grpc.history.capNote.hint', { limit: GRPC_HISTORY_PER_SCOPE_LIMIT }) }}
    </div>
    <!-- P110 I2-17: `.history-row:hover`/`.history-row.is-viewing` (same background either way)
         folded into one ternary -- no test locates by either class (checked; grpc-history-row is
         the real testid), so both markers drop cleanly. -->
  </div>
</template>
