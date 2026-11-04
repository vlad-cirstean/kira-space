<script setup lang="ts">
import { httpMethodToken, statusClass, statusHint } from '@shared/domain/http';
import { HISTORY_PER_SCOPE_LIMIT, type ResponseHistoryEntry } from '@shared/domain/responseHistory';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { methodTextClass } from '@theme/methodColor';
import { formatBytes, formatDuration, formatRelative } from '@workbench/behaviour/format';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { matchesQuery } from '@workbench/behaviour/match';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref } from 'vue';
import { patchHttpRequestTabState } from '../../api/tabs';
import type { HttpRequestTabRecord } from '../../state/tabDomain';
import { useTabIncognitoStore } from '../../state/tabIncognito';
import { useHttpHistoryList, useHttpHistoryStore, useHttpHistoryViewing } from './history';

// P8 D15: the History pane's list — one row per response, capped at HISTORY_PER_SCOPE_LIMIT by
// construction (P18 D4/D6), so no VirtualList/TreeHost involvement.
const confirmDialogStore = useConfirmDialogStore();
const tabIncognitoStore = useTabIncognitoStore();
const httpHistoryStore = useHttpHistoryStore();
const props = defineProps<{ tab: HttpRequestTabRecord }>();
const emit = defineEmits<{ compare: [ids: [string, string]] }>();

const { query: listQuery, entries } = useHttpHistoryList(() => props.tab);
const { viewingId, error: snapshotError } = useHttpHistoryViewing(() => props.tab.id);
const ui = computed(() => httpHistoryStore.ui[props.tab.id]);
const selected = computed(() => ui.value?.selected ?? []);
const errorMessage = computed(
  () =>
    ui.value?.actionError ?? listQuery.error.value?.message ?? snapshotError.value?.message ?? null,
);
const isScratch = computed(() => !props.tab.state.itemId);
// P71 §3.5: nothing is suppressed here — with nothing recorded, historyList simply returns the
// empty set, so this only exists to explain the silence rather than leave it looking broken.
const incognito = computed(() => tabIncognitoStore.isIncognito(props.tab.id));
const emptyDescription = computed(() => {
  if (incognito.value) return t('studio.http.history.incognito.description');
  const hint = t('studio.http.history.recordHint.description');
  return isScratch.value ? `${hint} ${t('studio.http.history.scratchHint.description')}` : hint;
});
// The list is ≤ HISTORY_PER_SCOPE_LIMIT by construction (Record's own trim), so "the list is full"
// is exactly this predicate — not a stored eviction count (List can never say more than "the list
// is full" without a new column and write path). Not suppressed by the filter: the note describes
// what is *stored*, not what is currently shown.
const atCap = computed(() => entries.value.length >= HISTORY_PER_SCOPE_LIMIT);

// P16 D15: matches method, URL, status text, or environment name — the fields already on screen
// in each row.
const filterQuery = computed({
  get: () => ui.value?.filterQuery ?? '',
  set: (v: string) => httpHistoryStore.setFilterQuery(props.tab.id, v),
});
const isFiltered = computed(() => filterQuery.value.trim() !== '');
const filteredEntries = computed<ResponseHistoryEntry[]>(() => {
  const q = filterQuery.value;
  return entries.value.filter(
    (e) =>
      matchesQuery(e.method, q) ||
      matchesQuery(e.url, q) ||
      matchesQuery(e.statusText, q) ||
      matchesQuery(String(e.status), q) ||
      matchesQuery(e.environment, q),
  );
});

/** The URL is shown on a second line only when it differs from the row above it — within
 *  one request's history the URL is usually identical, and repeating it twenty times is noise.
 *  Indexes into `filteredEntries` (the rendered list), not `entries` — the row "above" is the
 *  row visibly above it, under a filter too. */
function showUrl(i: number): boolean {
  return i === 0 || filteredEntries.value[i - 1]?.url !== filteredEntries.value[i]?.url;
}

// D10: "click views the entry" swaps the *whole* response pane, not just the runtime pointer —
// without switching back to Body, selecting a row would leave the user staring at the same list
// they just clicked in, with no visible sign anything happened.
function onRowClick(id: string): void {
  void httpHistoryStore.viewHistoryEntry(props.tab.id, id);
  patchHttpRequestTabState(props.tab.id, { responsePane: 'body' });
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

function onToggle(id: string): void {
  httpHistoryStore.toggleSelected(props.tab.id, id);
}

function onDelete(id: string): void {
  void httpHistoryStore.deleteHistoryEntry(props.tab.id, id);
}

function onCompare(): void {
  const [a, b] = selected.value;
  if (a && b) emit('compare', [a, b]);
}

async function onClear(): Promise<void> {
  const ok = await confirmDialogStore.confirm({
    title: t('studio.http.history.clearConfirm.title'),
    message: [
      t('studio.http.history.clearConfirm.body'),
      t('common.confirm.irreversible.body'),
    ].join(' '),
    confirmLabel: t('common.action.clear.label'),
    tone: 'danger',
  });
  if (ok) await httpHistoryStore.clearHistory(props.tab.id);
}
</script>

<template>
  <div class="flex flex-1 min-h-0 flex-col" data-testid="http-history-list">
    <ViewToolbar>
      <span class="text-kira-sm text-subtle">{{
        t('studio.http.history.count.status', { count: entries.length })
      }}</span>
      <span class="ml-auto" />
      <Button
        variant="toolbar"
        size="kira"
        :disabled="selected.length !== 2"
        data-testid="http-history-compare"
        @click="onCompare"
      >
        {{ t('studio.http.history.compare.label') }}
      </Button>
      <Button
        variant="toolbar"
        size="kira"
        :disabled="entries.length === 0"
        data-testid="http-history-clear"
        @click="onClear"
      >
        <CodiconIcon :name="ICON.delete" :size="13" />
        {{ t('common.action.clearHistory.label') }}
      </Button>
    </ViewToolbar>
    <Alert v-if="errorMessage" variant="error" data-testid="http-history-error">
      <AlertDescription>{{ errorMessage }}</AlertDescription>
    </Alert>

    <EmptyState
      v-if="entries.length === 0"
      data-testid="http-history-empty"
      :icon="ICON.history"
      :title="t('studio.http.history.empty')"
      :description="emptyDescription"
    />

    <template v-else>
      <SearchField
        v-model="filterQuery"
        :placeholder="t('studio.common.filterHistory.placeholder')"
        data-testid="http-history-filter"
      />
      <EmptyState
        v-if="isFiltered && filteredEntries.length === 0"
        data-testid="http-history-filter-empty"
        :icon="ICON.search"
        :title="t('common.state.noMatches.empty')"
      />
    </template>

    <div
      v-if="entries.length > 0 && filteredEntries.length > 0"
      class="flex flex-1 min-h-0 flex-col overflow-auto"
      ref="listEl"
      role="listbox"
      :aria-label="t('studio.http.history.list.aria')"
    >
      <div
        v-for="(entry, i) in filteredEntries"
        :key="entry.id"
        :class="cn(rowVariants({ layout: 'double', selected: entry.id === viewingId }))"
        data-testid="http-history-row"
        role="option"
        v-bind="nav.rowProps(entry.id)"
        :tabindex="nav.tabindexFor(entry.id)"
        :aria-selected="entry.id === viewingId"
        @click="onRowClick(entry.id)"
        @keydown="onRowKeydown($event, entry.id)"
      >
        <Checkbox
          data-testid="http-history-checkbox"
          :model-value="selected.includes(entry.id)"
          :disabled="!selected.includes(entry.id) && selected.length >= 2"
          @click.stop
          @update:model-value="onToggle(entry.id)"
        >
          <CodiconIcon name="check" :size="12" />
        </Checkbox>
        <div class="flex min-w-0 flex-1 flex-col gap-0.5">
          <div class="flex items-center gap-1">
            <Tooltip>
              <TooltipTrigger as-child>
                <span class="text-kira-sm text-subtle min-w-16">{{
                  formatRelative(entry.sentAt)
                }}</span>
              </TooltipTrigger>
              <TooltipContent>{{ entry.sentAt }}</TooltipContent>
            </Tooltip>
            <Badge data variant="chip" :class="methodTextClass(httpMethodToken(entry.method))">{{
              entry.method
            }}</Badge>
            <Tooltip>
              <TooltipTrigger as-child>
                <Badge
                  data
                  :variant="statusClass(entry.status)"
                  data-testid="http-history-status-chip"
                  >{{ entry.status }} {{ entry.statusText }}</Badge
                >
              </TooltipTrigger>
              <TooltipContent>{{ statusHint(entry.status) }}</TooltipContent>
            </Tooltip>
            <span class="text-kira-sm text-subtle">{{ formatDuration(entry.elapsedMs) }}</span>
            <span class="text-kira-sm text-subtle">{{ formatBytes(entry.bodyBytes) }}</span>
            <span v-if="entry.environment" class="text-kira-sm text-subtle">{{
              entry.environment
            }}</span>
            <span class="ml-auto" />
            <TooltipIconButton
              :icon="ICON.delete"
              :label="t('common.action.delete.label')"
              data-testid="http-history-delete"
              @click.stop="onDelete(entry.id)"
            />
          </div>
          <div v-if="showUrl(i)" class="min-w-0 text-kira-sm text-subtle">
            <TruncatedText :text="entry.url" />
          </div>
        </div>
      </div>
    </div>

    <div
      v-if="atCap"
      class="text-kira-sm text-subtle shrink-0 px-1.5 py-1"
      data-testid="http-history-cap-note"
    >
      {{ t('studio.http.history.capNote.hint', { limit: HISTORY_PER_SCOPE_LIMIT }) }}
    </div>
    <!-- P110 I2-17: `.history-row:hover`/`.history-row.is-viewing` (same background either way)
         folded into one ternary -- no test locates by either class (http-history-row is the real
         testid), so both markers drop cleanly. -->
  </div>
</template>
