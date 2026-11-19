<script setup lang="ts">
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { t } from '@theme/i18n';
import { useEventListener } from '@vueuse/core';
import { usePanelHeaderSearch } from '@workbench/behaviour/panelSearch';
import PanelBar from '@workbench/components/PanelBar.vue';
import SearchOptionToggles from '@workbench/components/SearchOptionToggles.vue';
import VirtualList from '@workbench/components/VirtualList.vue';
import { computed, ref, useTemplateRef } from 'vue';
import AdeFileSearchRow from './AdeFileSearchRow.vue';
import { type FileSearchRowVm, useAdeFileSearchStore } from './state/fileSearch';

// C7 §7.3: the query field, the three Monaco-find-widget-vocabulary toggles (D13: the same three
// codicons the in-file find widget and every other search surface in this app already use), the
// Search/Stop button, and the streamed results list.
const props = defineProps<{ branchId: string }>();
// A match opens the file in the viewer at its line and column.
const emit = defineEmits<{
  open: [path: string, reveal: { line: number; column: number; endColumn?: number }];
}>();
const searchStore = useAdeFileSearchStore();

const selected = ref<string | null>(null);

const query = computed({
  get: () => searchStore.searchQuery(props.branchId),
  set: (v: string) => searchStore.setSearchQuery(props.branchId, v),
});
const options = computed(() => searchStore.searchOptions(props.branchId));
const running = computed(() => searchStore.searchRunning(props.branchId));
const stats = computed(() => searchStore.searchStats(props.branchId));
const error = computed(() => searchStore.searchError(props.branchId));
const rows = computed(() => searchStore.searchRows(props.branchId));

// §7.3: "Searching…" while running, "N results in M files" when done, "(stopped at 10,000)"
// appended when the whole run hit the cap, "K files skipped" appended when non-zero — the skip
// count is surfaced, never silent (D4).
const statusLine = computed(() => {
  if (running.value) return t('common.searching.loading');
  const s = stats.value;
  if (!s) return '';
  let line = t('space.ade.files.search.summary.status', {
    count: s.matches,
    files: t('common.fileCount.label', { count: s.filesMatched }),
  });
  if (s.truncated) line = t('space.ade.files.search.stopped.status', { summary: line });
  if (s.filesSkipped > 0) {
    line = t('common.dashPair.label', {
      first: line,
      second: t('common.files.skipped.label', { count: s.filesSkipped }),
    });
  }
  return line;
});

function runSearch(): void {
  void searchStore.startSearch(props.branchId);
}

function onToggleOption(key: 'regex' | 'caseSensitive' | 'wholeWord'): void {
  searchStore.toggleSearchOption(props.branchId, key);
}

// D10: a search runs on Enter or the Search button only, never on every keystroke.
function onQueryKeydown(e: KeyboardEvent): void {
  if (e.key === 'Enter') {
    e.preventDefault();
    runSearch();
  } else if (e.key === 'Escape') {
    searchStore.setSearchQuery(props.branchId, '');
  }
}

// P105 §5.1: the toolbar div itself is not interactive -- binds via VueUse instead of a raw
// template @keydown on it.
const toolbar = useTemplateRef<{ $el: HTMLElement }>('toolbar');
useEventListener(() => toolbar.value?.$el, 'keydown', onQueryKeydown);

function onSelect(row: FileSearchRowVm): void {
  selected.value = row.key;
}

// Type-to-search: a printable key on the results lands in the query field.
const rootEl = useTemplateRef<HTMLElement>('rootEl');
usePanelHeaderSearch(rootEl, {
  stateKey: 'ade:files:search',
  searchable: () => true,
  getSearch: () => query.value,
  setSearch: (v) => {
    query.value = v;
  },
  searchTestid: 'ade-files-search-input',
});

function onToggleCollapse(row: FileSearchRowVm): void {
  searchStore.toggleSearchCollapse(props.branchId, row.path);
}

// A match opens the file in the viewer and moves its cursor, whether the file is freshly opened
// or already showing.
function onOpen(row: FileSearchRowVm): void {
  if (row.kind !== 'match' || row.line === undefined || row.column === undefined) return;
  emit('open', row.path, { line: row.line, column: row.column, endColumn: row.endColumn });
}

// Enter: permanent open for a match, collapse toggle for a file header. Space: the single click.
function onRowOpen(row: FileSearchRowVm): void {
  onSelect(row);
  if (row.kind === 'file') onToggleCollapse(row);
  else onOpen(row);
}

function onRowSelect(row: FileSearchRowVm): void {
  onSelect(row);
  if (row.kind === 'file') onToggleCollapse(row);
  else onOpen(row);
}
</script>

<template>
  <div ref="rootEl" class="h-full flex flex-col min-h-0">
    <PanelBar ref="toolbar" class="flex-row items-center">
      <div class="flex-1 min-w-0">
        <SearchField
          :model-value="query"
          data-testid="ade-files-search-input"
          :aria-invalid="!!error"
          @update:model-value="(v) => (query = v)"
        />
      </div>
      <!-- Case/Word/Regex: three independent toggles, not a single-value picker — the same three
           codicons SearchToolbar.vue's own find widget uses, so the two surfaces read as one
           vocabulary (D13). -->
      <div class="flex">
        <SearchOptionToggles
          :match-case="options.caseSensitive"
          :whole-word="options.wholeWord"
          :regex="options.regex"
          testid-prefix="ade-files-search-"
          match-case-testid="ade-files-search-case"
          @update:match-case="onToggleOption('caseSensitive')"
          @update:whole-word="onToggleOption('wholeWord')"
          @update:regex="onToggleOption('regex')"
        />
      </div>
      <TooltipIconButton
        v-if="running"
        icon="debug-stop"
        :label="t('common.action.stop.label')"
        data-testid="ade-files-search-stop"
        @click="searchStore.cancelSearch(branchId)"
      />
      <TooltipIconButton
        v-else
        icon="search"
        :label="t('common.action.search.label')"
        data-testid="ade-files-search-run"
        @click="runSearch"
      />
    </PanelBar>
    <Alert v-if="error" variant="error" data-testid="ade-files-search-error">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>
    <Alert v-else-if="statusLine" variant="info" data-testid="ade-files-search-status">
      <AlertDescription>{{ statusLine }}</AlertDescription>
    </Alert>
    <VirtualList
      v-model:active-key="selected"
      :rows="rows"
      :keep-scroll-key="`git:search:${branchId}`"
      testid="virtual-list"
      :aria-label="t('common.searchResults.aria')"
      :open="onRowOpen"
      :select="onRowSelect"
    >
      <template #row="{ row, nav }">
        <AdeFileSearchRow
          v-bind="nav"
          :row="row"
          :selected="selected === row.key"
          @select="onSelect"
          @toggle-collapse="onToggleCollapse"
          @open="onOpen"
        />
      </template>
    </VirtualList>
  </div>
</template>
