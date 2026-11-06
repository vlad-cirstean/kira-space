<script setup lang="ts" generic="M extends { row: number }">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { type MessageKey, t } from '@theme/i18n';
import CatalogText from '@theme/i18n/CatalogText.vue';
import { formatCount } from '@workbench/behaviour/format';
import { errorText } from '@workbench/bridge/codedError';
import FindBar from '@workbench/components/FindBar.vue';
import SearchOptionToggles from '@workbench/components/SearchOptionToggles.vue';
import ToolbarSeparator from '@workbench/components/ToolbarSeparator.vue';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import type { SearchHandle } from './scan';
import type { PageSearchApi } from './search';
import { type FindInput, usePageSearchFilterStore } from './searchFilter';

// Shared by the grid, document and key-value views (testidPrefix '', 'document-', 'keyvalue-'):
// a chunked scan over the loaded rows on the workbench `FindBar` shell.
const props = defineProps<{
  tabId: string;
  testidPrefix: string;
  kind: 'rows' | 'documents';
  api: PageSearchApi<M>;
  /** Live read of the owner's open flag: at unmount the parent's `v-if` has flipped but this
   *  component's props are stale, so a boolean prop would still read true. */
  isOpen: () => boolean;
  /** The grid's own mask preview state — this component is shared with documents/
   *  keyvalue, neither of which has a masking concept, so this stays `undefined` (falsy) there.
   *  Search itself is unaffected either way (it always scans stored values, the "not a
   *  security boundary" framing) — this only controls whether the one-line note below shows. */
  maskPreviewOn?: boolean;
}>();

const emit = defineEmits<{ goToMatch: [match: M]; close: [] }>();

const pageSearchFilterStore = usePageSearchFilterStore();

// P31 D22/F24: `api.pageVersion.n` is the explicit dependency — the page stores read a plain,
// non-reactive Map.
const loadedRowCount = computed(() => {
  void props.api.pageVersion.n;
  return props.api.loadedRowCount(props.tabId);
});

// P24 D9: the scope label gains a filtered form ("showing N of M loaded …") whenever the toggle
// is on and a scan has completed.
const filtering = computed(() => pageSearchFilterStore.isSearchFiltering(props.tabId));
const filteredRowCount = computed(() => props.api.matchedRows(props.tabId)?.length ?? null);

const scopeKey = computed<MessageKey>(() =>
  props.kind === 'documents'
    ? 'studio.search.scopeDocuments.status'
    : 'studio.search.scopeRows.status',
);
const scopeFilteredKey = computed<MessageKey>(() =>
  props.kind === 'documents'
    ? 'studio.search.scopeFilteredDocuments.status'
    : 'studio.search.scopeFilteredRows.status',
);

function toggleFilter(): void {
  pageSearchFilterStore.setSearchFiltering(props.tabId, !filtering.value);
}

// P375: inputs live in the store so a tab switch (which unmounts this bar) restores them.
function findField<K extends keyof FindInput>(key: K) {
  return computed<FindInput[K]>({
    get: () => pageSearchFilterStore.findInput(props.tabId)[key],
    set: (v) => pageSearchFilterStore.setFindInput(props.tabId, { [key]: v }),
  });
}
const query = findField('query');
const matchCase = findField('matchCase');
const wholeWord = findField('wholeWord');
const regex = findField('regex');
const errorMessage = ref<string | null>(null);
const scanning = ref(false);
const foundSoFar = ref(0);

let handle: SearchHandle<M> | null = null;

const entry = computed(() => props.api.searchState[props.tabId]);

// autoScroll is false for the page-replaced re-scan (D23): jumping the viewport because a
// background refresh landed would move the page under the user's hands, unlike a query edit
// which is the user's own action and should jump to the first hit immediately.
function startSearch(autoScroll = true): void {
  handle?.cancel();
  errorMessage.value = null;
  if (query.value === '') {
    props.api.clearSearchState(props.tabId);
    handle = null;
    return;
  }
  scanning.value = true;
  foundSoFar.value = 0;
  // `runChunkedScan`'s own `done` resolves with the partial match list on
  // cancellation (a useful contract on its own — it's how cancel() can be awaited at all) — but a
  // superseded scan's `resolve` still lands on a *later* animation frame, after `startSearch` has
  // already replaced `handle` with a new one. Capturing this call's own handle in `thisHandle` and
  // testing identity (below, both in onProgress and in `.then`) is what keeps a superseded scan's
  // tick or resolution from ever touching `searchState`/`scanning` again — including the
  // close()/onUnmounted() case, where the "newer handle" is `null` (both null it out below).
  let thisHandle: SearchHandle<M>;
  try {
    thisHandle = props.api.runSearch(
      props.tabId,
      {
        text: query.value,
        matchCase: matchCase.value,
        wholeWord: wholeWord.value,
        regex: regex.value,
      },
      // Every tick — the priority window's own pre-scan included — publishes into
      // searchState itself, not just this toolbar's local counter, so highlighting updates as the
      // scan runs rather than only once at the end. `pending: true` is what tells
      // matchedRows() (search.ts) not to filter on an answer the scan hasn't finished yet.
      (found, rowsScanned, _totalRows, soFar) => {
        if (handle !== thisHandle) return;
        foundSoFar.value = found;
        // P43 iter2 F25/D34, corrected P43 iter3 F40: `rowsScanned === 0` is scan.ts's own
        // priority-tick marker (:104) — every main-pass tick reports the row it scanned up to,
        // always > 0 on a non-empty page. The priority window's own `soFar` is a different,
        // unrelated array (scan.ts:60-61's own comment), so an index into it means nothing once
        // the main pass starts and replaces `matches` wholesale — the reset below fires
        // pre-emptively, on the priority tick itself, one animation frame *before* that
        // replacement actually happens (scan.ts:105 calls runMainPass() synchronously at the end
        // of the priority frame), not on the first main-pass tick after it. Every main-pass tick
        // after that is strictly append-only and ascending, so an existing index keeps pointing
        // at the same match as `soFar` grows underneath it.
        const previousIndex = props.api.searchState[props.tabId]?.index ?? -1;
        const index = rowsScanned === 0 ? -1 : previousIndex;
        // This used to spread `soFar` into a fresh array
        // (`[...soFar]`) on every single progress tick — a full copy of the growing, up to
        // MAX_SCAN_MATCHES-entry match array once per animation frame of the scan, for a value
        // scan.ts's own runChunkedScan already documents as "the scanner never mutates a
        // published array in place other than appending" (only ever grows via .push, never
        // reassigned or spliced mid-scan) and that every consumer here already treats as
        // read-only (matchedRows/createMatchIndex only ever iterate or index into it). Publishing
        // `soFar` by reference is therefore exactly as safe as the copy was, at O(1) instead of
        // O(matches) per tick. `soFar`'s own `readonly M[]` type (scan.ts) is the caller-facing
        // half of that same read-only contract; this cast is the other half, matching what
        // searchState's own field type already assumes every writer honors.
        props.api.searchState[props.tabId] = { matches: soFar as M[], index, pending: true, found };
      },
    );
  } catch (err) {
    // An invalid regex is reported inline, never thrown into an unhandled rejection (D28).
    errorMessage.value = errorText(err);
    scanning.value = false;
    return;
  }
  // Seeded now that runSearch has started a scan — ticks are rAF-deferred, so this always lands
  // before the first one — rather than earlier, which would also clear a still-valid previous
  // match set out from under the grid on a synchronously thrown regex error above. Without a
  // priority window (keyvalue's own search passes none), the first main-pass tick's `rowsScanned`
  // is never 0, so nothing above would otherwise reset `previousIndex` away from whatever the
  // *previous, completed* query left it at.
  props.api.searchState[props.tabId] = { matches: [], index: -1, pending: true, found: 0 };
  handle = thisHandle;
  thisHandle.done.then(({ matches, found, matchedRows }) => {
    if (handle !== thisHandle) return;
    scanning.value = false;
    // A match the user navigated to mid-scan survives the scan finishing, instead of snapping back
    // to the first hit and re-scrolling the viewport out from under them. During the main pass
    // `soFar` *is* `matches` (scan.ts:84/:88 append into and resolve the same array), strictly
    // ascending, so an index into the partial list still identifies the same match in the final one
    // — the bounds check below is belt-and-braces for an inherited index, not the mechanism this
    // relies on. autoScroll only fires when the user never navigated: a user who already moved the
    // viewport themselves has already answered the question autoScroll exists to ask.
    const navigatedIndex = props.api.searchState[props.tabId]?.index ?? -1;
    const navigated = navigatedIndex >= 0 && navigatedIndex < matches.length;
    const index = navigated ? navigatedIndex : matches.length > 0 ? 0 : -1;
    const page = props.api.pageOf?.(props.tabId);
    props.api.searchState[props.tabId] = {
      matches,
      index,
      found,
      matchedRows,
      ...(page && typeof page === 'object' ? { page: new WeakRef(page) } : {}),
    };
    if (autoScroll && !navigated && matches.length > 0) emit('goToMatch', matches[0]);
  });
}

// Every keystroke used to restart the whole (potentially many-frame) scan. useFindBar debounces
// the query run (TIMING.search); the toggles (matchCase/wholeWord/regex) stay immediate, since a
// checkbox click is a single, deliberate action, not a typing stream. `closed` drops a debounced
// run that lands after the bar closed.
let closed = false;
function onQuery(): void {
  if (!closed) startSearch();
}
watch([matchCase, wholeWord, regex], () => startSearch());

// Paging, Fetch more, a page-size change, Refresh or a WHERE re-run all call
// setPage and bump pageVersion.n — without this, searchState[tabId].matches keeps pointing at
// rows from the page that's gone. Restarting the scan is the only way back to a consistent match
// list; not auto-scrolling matches a fresh scan's own initial state.
watch(
  () => {
    const n = props.api.pageVersion.n;
    // Other tabs' loads and closes bump the shared counter; follow this tab's page identity when
    // the view supplies it.
    return props.api.pageOf ? props.api.pageOf(props.tabId) : n;
  },
  () => {
    if (query.value !== '') startSearch(false);
  },
);

// P2 R1: searchState is a shallowReactive record now (search.ts), so a nested `e.index = …`
// mutation on an already-read entry would no longer be tracked — every writer, this one included,
// replaces the whole entry, the same way startSearch's own onProgress/done callbacks above do.
function goNext(): void {
  const e = entry.value;
  if (!e || e.matches.length === 0) return;
  const index = (e.index + 1) % e.matches.length;
  props.api.searchState[props.tabId] = { ...e, index };
  emit('goToMatch', e.matches[index]);
}
function goPrev(): void {
  const e = entry.value;
  if (!e || e.matches.length === 0) return;
  const index = (e.index - 1 + e.matches.length) % e.matches.length;
  props.api.searchState[props.tabId] = { ...e, index };
  emit('goToMatch', e.matches[index]);
}

function close(): void {
  closed = true;
  handle?.cancel();
  handle = null;
  props.api.clearSearchState(props.tabId);
  // P24 D7: a closed toolbar must never leave rows hidden with no visible cause.
  pageSearchFilterStore.clearFind(props.tabId);
  emit('close');
}

// Remount after a tab switch: keep matches when the scan finished over the current page.
onMounted(() => {
  if (query.value === '') return;
  const e = entry.value;
  const current = props.api.pageOf?.(props.tabId);
  if (!e || e.pending || e.page?.deref() !== current) startSearch(false);
});

onUnmounted(() => {
  closed = true;
  handle?.cancel();
  handle = null;
  // Still open: a tab switch, not a close. Keep input, toggle and a finished scan.
  if (props.isOpen()) {
    if (props.api.searchState[props.tabId]?.pending) props.api.clearSearchState(props.tabId);
    return;
  }
  props.api.clearSearchState(props.tabId);
  // P24 D7: Cmd+F toggling the toolbar off unmounts this component without ever calling close()
  // above — the toggle must reset here too.
  pageSearchFilterStore.clearFind(props.tabId);
});
</script>

<template>
  <FindBar
    v-model="query"
    :position="entry ? entry.index + 1 : 0"
    :total="entry ? (entry.found ?? entry.matches.length) : 0"
    :invalid="!!errorMessage"
    :testid-prefix="`${testidPrefix}search-`"
    @query="onQuery"
    @next="goNext"
    @prev="goPrev"
    @close="close"
  >
    <template #options>
      <SearchOptionToggles
        v-model:match-case="matchCase"
        v-model:whole-word="wholeWord"
        v-model:regex="regex"
        :testid-prefix="`${testidPrefix}search-`"
      />
      <ToolbarSeparator />
      <TooltipIconButton
        icon="filter"
        :label="
          t(filtering ? 'studio.search.filteringRows.tooltip' : 'studio.search.filterRows.tooltip')
        "
        :aria-label="t('studio.search.filterRows.tooltip')"
        :pressed="filtering"
        :data-testid="`${testidPrefix}search-filter-rows`"
        @click="toggleFilter"
      />
      <ToolbarSeparator />
      <span class="text-kira-sm text-subtle" :data-testid="`${testidPrefix}search-scope`">
        <template v-if="filtering && filteredRowCount !== null">
          {{ t(scopeFilteredKey, { shown: formatCount(filteredRowCount), count: loadedRowCount }) }}
        </template>
        <template v-else>{{ t(scopeKey, { count: loadedRowCount }) }}</template>
      </span>
      <span
        v-if="maskPreviewOn"
        class="text-kira-sm text-subtle"
        :data-testid="`${testidPrefix}search-mask-note`"
      >
        {{ t('studio.search.maskNote.status') }}
      </span>
    </template>
    <template #count="{ text }">
      <span v-if="errorMessage" class="text-error" :data-testid="`${testidPrefix}search-error`">{{
        errorMessage
      }}</span>
      <template v-else-if="scanning">{{
        t('studio.search.scanning.loading', { count: foundSoFar })
      }}</template>
      <template v-else-if="entry && entry.matches.length > 0">
        <CatalogText k="common.matchPosition.status">
          <template #position><b class="font-data">{{ entry.index + 1 }}</b></template>
          <template #total
            ><b class="font-data">{{
              formatCount(entry.found ?? entry.matches.length)
            }}</b></template
          >
        </CatalogText>
        <span
          v-if="entry.found !== undefined && entry.found > entry.matches.length"
          :data-testid="`${testidPrefix}search-capped`"
        >
          {{ t('studio.search.capped.status', { count: entry.matches.length }) }}
        </span>
      </template>
      <template v-else>{{ text }}</template>
    </template>
  </FindBar>
</template>
