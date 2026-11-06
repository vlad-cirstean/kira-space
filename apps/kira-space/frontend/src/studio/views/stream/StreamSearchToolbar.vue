<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { formatCount } from '@workbench/behaviour/format';
import FindBar from '@workbench/components/FindBar.vue';
import ToolbarSeparator from '@workbench/components/ToolbarSeparator.vue';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { usePageSearchFilterStore } from '../shared/page/searchFilter';
import { getPage, pageVersion } from './page';
import { useStreamSearchStore } from './search';

const props = defineProps<{
  tabId: string;
  /** Live read of the owner's open flag; props are stale at unmount. */
  isOpen: () => boolean;
}>();
const emit = defineEmits<{ goToMatch: [row: number]; close: [] }>();

const pageSearchFilterStore = usePageSearchFilterStore();
const streamSearchStore = useStreamSearchStore();

// README's own "search walks the loaded rows only and never issues a query" wording, borrowed
// verbatim from views/shared/page/SearchToolbar.vue's precedent — applies here too (item 5).
// P31 D22/F24: pageVersion.n is the explicit dependency — getPage reads a plain, non-reactive Map.
const page = computed(() => {
  void pageVersion.n;
  return getPage(props.tabId);
});
const loadedRowCount = computed(() => page.value?.rowCount ?? 0);

// P31 D17: the same filter-mode toggle views/shared/page/SearchToolbar.vue has (P24 D1/D9).
const filtering = computed(() => pageSearchFilterStore.isSearchFiltering(props.tabId));
const filteredRowCount = computed(() => streamSearchStore.matchedRows(props.tabId)?.length ?? null);
function toggleFilter(): void {
  pageSearchFilterStore.setSearchFiltering(props.tabId, !filtering.value);
}

// P375: restored from the kept scan after a tab switch.
const query = ref(streamSearchStore.searchState[props.tabId]?.query ?? '');
const entry = computed(() => streamSearchStore.searchState[props.tabId]);

// The scan decodes every cell of the page synchronously, so useFindBar debounces it per keystroke.
// `closed` drops a debounced run that lands after the bar closed.
let closed = false;
function searchNow(q: string): void {
  if (closed) return;
  streamSearchStore.runSearch(props.tabId, q);
  const e = streamSearchStore.searchState[props.tabId];
  if (e && e.matches.length > 0) emit('goToMatch', e.matches[0]);
}

// P31 D22/D23/F23: a Fetch more/poll/page change calls setPage and bumps pageVersion.n —
// re-scan against the new page (runSearch already resets index to the first match, or -1,
// per D23) without auto-scrolling; a background poll must not move the viewport under the user.
watch(page, () => {
  if (query.value !== '') streamSearchStore.runSearch(props.tabId, query.value);
});

function next(): void {
  const row = streamSearchStore.goToNextMatch(props.tabId);
  if (row !== null) emit('goToMatch', row);
}
function prev(): void {
  const row = streamSearchStore.goToPrevMatch(props.tabId);
  if (row !== null) emit('goToMatch', row);
}

function close(): void {
  closed = true;
  streamSearchStore.clearSearchState(props.tabId);
  // P24 D7/P31 D18: a closed toolbar must never leave rows hidden with no visible cause.
  pageSearchFilterStore.setSearchFiltering(props.tabId, false);
  emit('close');
}

onMounted(() => {
  if (query.value === '') return;
  const e = streamSearchStore.searchState[props.tabId];
  if (!e || e.page?.deref() !== getPage(props.tabId)) {
    streamSearchStore.runSearch(props.tabId, query.value);
  }
});

onUnmounted(() => {
  closed = true;
  // Still open: a tab switch, not a close. Keep the query, toggle and scan.
  if (props.isOpen()) return;
  streamSearchStore.clearSearchState(props.tabId);
  // P31 D18: Cmd+F toggling the toolbar off unmounts this component without ever calling close()
  // above — the toggle must reset here too (mirrors views/shared/page/SearchToolbar.vue's own note).
  pageSearchFilterStore.setSearchFiltering(props.tabId, false);
});
</script>

<template>
  <FindBar
    v-model="query"
    :position="entry ? entry.index + 1 : 0"
    :total="entry ? entry.matches.length : 0"
    testid-prefix="stream-search-"
    @query="searchNow"
    @next="next"
    @prev="prev"
    @close="close"
  >
    <template #options>
      <TooltipIconButton
        icon="filter"
        :label="
          t(filtering ? 'studio.search.filteringRows.tooltip' : 'studio.search.filterRows.tooltip')
        "
        :aria-label="t('studio.search.filterRows.tooltip')"
        :pressed="filtering"
        data-testid="stream-search-filter-rows"
        @click="toggleFilter"
      />
      <ToolbarSeparator />
      <span class="text-kira-sm text-subtle" data-testid="stream-search-scope">
        <template v-if="filtering && filteredRowCount !== null">
          {{
            t('studio.search.scopeFilteredRows.status', {
              shown: formatCount(filteredRowCount),
              count: loadedRowCount,
            })
          }}
        </template>
        <template v-else>{{
          t('studio.search.scopeRows.status', { count: loadedRowCount })
        }}</template>
      </span>
    </template>
    <template #count="{ text }">
      <CatalogText v-if="entry && entry.matches.length > 0" k="common.matchPosition.status">
        <template #position><b class="font-data">{{ entry.index + 1 }}</b></template>
        <template #total><b class="font-data">{{ entry.matches.length }}</b></template>
      </CatalogText>
      <template v-else>{{ text }}</template>
    </template>
  </FindBar>
</template>
