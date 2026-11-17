<script setup lang="ts">
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { t } from '@theme/i18n';
import { useEventListener } from '@vueuse/core';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { usePanelHeaderSearch } from '@workbench/behaviour/panelSearch';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import PanelBar from '@workbench/components/PanelBar.vue';
import SearchOptionToggles from '@workbench/components/SearchOptionToggles.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { computed, ref, useTemplateRef } from 'vue';
import { openRepoFileTab } from '../state/repoTabs';
import RepoSearchRow from './RepoSearchRow.vue';
import { type RepoSearchRowVm, useRepoSearchStore } from './state/search';

// C7 §7.3: the query field, the three Monaco-find-widget-vocabulary toggles (D13: the same three
// codicons the in-file find widget and every other search surface in this app already use), the
// Search/Stop button, and the streamed results list.
const props = defineProps<{ repoId: string }>();
const repoSearchStore = useRepoSearchStore();

const { single: rowHeight } = useRowHeight();
const selected = ref<string | null>(null);

const query = computed({
  get: () => repoSearchStore.repoSearchQuery(props.repoId),
  set: (v: string) => repoSearchStore.setRepoSearchQuery(props.repoId, v),
});
const options = computed(() => repoSearchStore.repoSearchOptions(props.repoId));
const running = computed(() => repoSearchStore.repoSearchRunning(props.repoId));
const stats = computed(() => repoSearchStore.repoSearchStats(props.repoId));
const error = computed(() => repoSearchStore.repoSearchError(props.repoId));
const rows = computed(() => repoSearchStore.repoSearchRows(props.repoId));

const scrollEl = ref<HTMLElement | null>(null);
const { virtualItems, totalSize, onScroll, scrollToIndex } = useVirtualRows({
  count: () => rows.value.length,
  rowHeight: () => rowHeight.value,
  scrollElement: scrollEl,
});

// §7.3: "Searching…" while running, "N results in M files" when done, "(stopped at 10,000)"
// appended when the whole run hit the cap, "K files skipped" appended when non-zero — the skip
// count is surfaced, never silent (D4).
const statusLine = computed(() => {
  if (running.value) return t('common.searching.loading');
  const s = stats.value;
  if (!s) return '';
  let line = t('space.repo.search.summary.status', {
    count: s.matches,
    files: t('common.fileCount.label', { count: s.filesMatched }),
  });
  if (s.truncated) line = t('space.repo.search.stopped.status', { summary: line });
  if (s.filesSkipped > 0) {
    line = t('common.dashPair.label', {
      first: line,
      second: t('memory.import.confirm.skipped.label', { count: s.filesSkipped }),
    });
  }
  return line;
});

function runSearch(): void {
  void repoSearchStore.startRepoSearch(props.repoId);
}

function onToggleOption(key: 'regex' | 'caseSensitive' | 'wholeWord'): void {
  repoSearchStore.toggleRepoSearchOption(props.repoId, key);
}

// D10: a search runs on Enter or the Search button only, never on every keystroke.
function onQueryKeydown(e: KeyboardEvent): void {
  if (e.key === 'Enter') {
    e.preventDefault();
    runSearch();
  } else if (e.key === 'Escape') {
    repoSearchStore.setRepoSearchQuery(props.repoId, '');
  }
}

// P105 §5.1: the toolbar div itself is not interactive -- binds via VueUse instead of a raw
// template @keydown on it.
const toolbar = useTemplateRef<{ $el: HTMLElement }>('toolbar');
useEventListener(() => toolbar.value?.$el, 'keydown', onQueryKeydown);

function onSelect(row: RepoSearchRowVm): void {
  selected.value = row.key;
}

// Type-to-search: a printable key on the results lands in the query field.
const rootEl = useTemplateRef<HTMLElement>('rootEl');
usePanelHeaderSearch(rootEl, {
  stateKey: 'git:search',
  searchable: () => true,
  getSearch: () => query.value,
  setSearch: (v) => {
    query.value = v;
  },
  searchTestid: 'repo-search-query',
});

function onToggleCollapse(row: RepoSearchRowVm): void {
  repoSearchStore.toggleRepoSearchCollapse(props.repoId, row.path);
}

// §7.4/D12: a single click opens a preview tab, double-click/Enter a permanent one — the tree's
// own onSelect/onOpen split (RepoSearchRow.vue already resolved which this is). The reveal moves
// the cursor whether the tab is freshly opened or already mounted and active (D12's own fix).
function onOpen(row: RepoSearchRowVm, preview: boolean): void {
  if (row.kind !== 'match' || row.line === undefined || row.column === undefined) return;
  void openRepoFileTab(props.repoId, row.path, {
    preview,
    reveal: { line: row.line, column: row.column, endColumn: row.endColumn },
  });
}

// Enter: permanent open for a match, collapse toggle for a file header. Space: the single click.
const nav = useListNavigation<RepoSearchRowVm>({
  container: scrollEl,
  items: () => rows.value,
  key: (row) => row.key,
  activeKey: selected,
  onOpen: (row) => {
    onSelect(row);
    if (row.kind === 'file') onToggleCollapse(row);
    else onOpen(row, false);
  },
  onSelect: (row) => {
    onSelect(row);
    if (row.kind === 'file') onToggleCollapse(row);
    else onOpen(row, true);
  },
  scrollToIndex: (index) => scrollToIndex(index),
  rendered: () => virtualItems.value,
});
</script>

<template>
  <div ref="rootEl" class="h-full flex flex-col min-h-0">
    <PanelBar ref="toolbar" class="flex-row items-center">
      <div class="flex-1 min-w-0">
        <SearchField
          :model-value="query"
          data-testid="repo-search-query"
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
          testid-prefix="repo-search-"
          match-case-testid="repo-search-case"
          @update:match-case="onToggleOption('caseSensitive')"
          @update:whole-word="onToggleOption('wholeWord')"
          @update:regex="onToggleOption('regex')"
        />
      </div>
      <TooltipIconButton
        v-if="running"
        icon="debug-stop"
        :label="t('common.action.stop.label')"
        data-testid="repo-search-stop"
        @click="repoSearchStore.cancelRepoSearch(repoId)"
      />
      <TooltipIconButton
        v-else
        icon="search"
        :label="t('common.action.search.label')"
        data-testid="repo-search-run"
        @click="runSearch"
      />
    </PanelBar>
    <Alert v-if="error" variant="error" data-testid="repo-search-error">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>
    <Alert v-else-if="statusLine" variant="info" data-testid="repo-search-status">
      <AlertDescription>{{ statusLine }}</AlertDescription>
    </Alert>
    <div
      ref="scrollEl"
      class="flex-1 min-h-0 overflow-auto"
      data-testid="virtual-list"
      role="listbox"
      :aria-label="t('common.searchResults.aria')"
      @scroll="onScroll"
    >
      <div class="relative" :style="{ height: `${totalSize}px` }">
        <template v-for="item in virtualItems" :key="String(item.key)">
          <RepoSearchRow
            :class="VIRTUAL_ROW_CLASS"
            :style="{ transform: `translateY(${item.start}px)`, height: `${item.size}px` }"
            v-bind="nav.rowProps(rows[item.index].key)"
            :row="rows[item.index]"
            :selected="selected === rows[item.index].key"
            @select="onSelect"
            @toggle-collapse="onToggleCollapse"
            @open="onOpen"
          />
        </template>
      </div>
    </div>
  </div>
  <!-- P110 I2-15: `.virtual-row` moved to VIRTUAL_ROW_CLASS (packages/workbench/src/lib/
       virtualRows.ts) -- see kira-studio's ProjectTree.vue's identical note. -->
</template>
