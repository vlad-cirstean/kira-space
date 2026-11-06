<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Label } from '@theme/components/ui/label';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { rowIndent, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { refDebounced } from '@vueuse/core';
import PanelBar from '@workbench/components/PanelBar.vue';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { storeToRefs } from 'pinia';
import { computed, ref, useId, useTemplateRef, watch } from 'vue';
import { useFocusOnMount } from '../behaviour/focus';
import { useListNavigation } from '../behaviour/listNavigation';
import { usePanelHeaderSearch } from '../behaviour/panelSearch';
import { TIMING } from '../behaviour/timing';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '../behaviour/virtualRows';
import QueryState from '../components/QueryState.vue';
import { useRowHeight } from '../lib/rowHeight';
import AddMemoryDialog from './AddMemoryDialog.vue';
import ImportConfirmDialog from './import/ImportConfirmDialog.vue';
import ImportMenu from './import/ImportMenu.vue';
import ImportStatus from './import/ImportStatus.vue';
import { useImportChangeSync } from './import/importQueries';
import { useImportUiStore } from './import/importStore';
import { KIND_LABEL, MATCH_LABEL, originText, SCOPE_LABEL } from './labels';
import MemoryFilters from './MemoryFilters.vue';
import MemoryScore from './MemoryScore.vue';
import MemorySetupHint from './MemorySetupHint.vue';
import { useMemoryChangeSync, useMemorySearch, useMemorySemanticStatus } from './queries';
import { useMemoryUiStore } from './store';

// P201: the Memory module's left panel — recall-first search over the shared memory store, the
// same service the kira-memory MCP server writes through.
const ui = useMemoryUiStore();
const { query, includeHistory, selectedId, addOpen, scopes, origins, project } = storeToRefs(ui);
const debounced = refDebounced(query, TIMING.search);
const search = useMemorySearch(debounced, includeHistory, () => ({
  scopes: scopes.value,
  origins: origins.value,
  project: project.value,
}));
useMemoryChangeSync();
useImportChangeSync();

const imports = useImportUiStore();
const rootEl = useTemplateRef<HTMLElement>('root');
const searchInput = computed(
  () => rootEl.value?.querySelector<HTMLInputElement>('[data-testid="memory-search"]') ?? null,
);
useFocusOnMount(searchInput);
// Recall-first: the field is always shown; a printable key on the list redirects into it.
const { showSearch } = usePanelHeaderSearch(rootEl, {
  searchable: () => true,
  getSearch: () => query.value,
  setSearch: (value) => {
    query.value = value;
  },
  searchTestid: 'memory-search',
});
showSearch.value = true;

const resultsEl = useTemplateRef<HTMLElement>('resultsEl');
const activeKey = ref<string | null>(null);
watch(
  selectedId,
  (id) => {
    activeKey.value = id;
  },
  { immediate: true },
);
const rowHeight = useRowHeight();
const activeIndex = computed(() =>
  (search.data.value ?? []).findIndex((m) => m.id === activeKey.value),
);
const { virtualizer, virtualItems, totalSize, scrollToIndex } = useVirtualRows({
  count: () => search.data.value?.length ?? 0,
  rowHeight: () => rowHeight.double.value + 8,
  scrollElement: resultsEl,
  pinned: () => (activeIndex.value < 0 ? null : activeIndex.value),
});
const windowRows = computed(() =>
  virtualItems.value.flatMap((item) => {
    const memory = search.data.value?.[item.index];
    return memory ? [{ item, memory }] : [];
  }),
);
function measure(el: unknown): void {
  if (el instanceof Element) virtualizer.value.measureElement(el);
}
const nav = useListNavigation({
  container: resultsEl,
  items: () => search.data.value ?? [],
  key: (m) => m.id,
  activeKey,
  onOpen: (m) => selectMemory(m.id),
  onSelect: (m) => selectMemory(m.id),
  scrollToIndex: (index) => scrollToIndex(index),
  rendered: () => virtualItems.value,
});
const semanticStatus = useMemorySemanticStatus();
const semanticState = computed(() => semanticStatus.data.value?.state);
const semanticNeedsSetup = computed(
  () => semanticState.value === 'notInstalled' || semanticState.value === 'unavailable',
);

function selectMemory(id: string): void {
  selectedId.value = id;
  ui.view = 'memory';
}

const historyToggleId = useId();
</script>

<template>
  <div ref="root" class="flex h-full flex-col" data-testid="memory-panel">
    <PanelHeader>
      {{ t('common.module.memory.label') }}
      <template #actions>
        <TooltipIconButton
          icon="add"
          :label="t('memory.add.label')"
          data-testid="memory-add"
          @click="addOpen = true"
        />
        <TooltipIconButton
          icon="sparkle"
          :label="t('memory.distill.open.label')"
          data-testid="memory-distill"
          @click="ui.view = 'distill'"
        />
        <ImportMenu />
      </template>
    </PanelHeader>
    <PanelBar>
      <SearchField
        v-model="query"
        :placeholder="t('memory.panel.search.placeholder')"
        data-testid="memory-search"
      />
      <div class="flex items-center gap-1.5 text-kira-sm text-muted-foreground">
        <Switch
          :id="historyToggleId"
          v-model="includeHistory"
          data-testid="memory-include-history"
        />
        <Label :for="historyToggleId">{{ t('memory.panel.includeHistory.label') }}</Label>
      </div>
      <MemoryFilters v-model:scopes="scopes" v-model:origins="origins" v-model:project="project" />
      <MemorySetupHint
        v-if="semanticNeedsSetup"
        :message="
          semanticState === 'notInstalled'
            ? t('memory.panel.semanticOff.hint')
            : t('memory.panel.semanticUnavailable.hint')
        "
        data-testid="memory-setup-hint-semantic"
      />
      <ImportStatus />
    </PanelBar>
    <div ref="resultsEl" class="min-h-0 flex-1 overflow-y-auto" data-testid="memory-results">
      <QueryState :query="search" :loading-label="t('memory.panel.list.loading')">
        <template #error="{ message }">
          <Alert variant="error" strip data-testid="memory-error">
            <AlertDescription>{{ message }}</AlertDescription>
          </Alert>
        </template>
        <template #empty>
          <EmptyState
            fill
            data-testid="memory-empty"
            :title="
              query.trim() === '' ? t('memory.panel.list.empty') : t('common.state.noMatches.empty')
            "
            :description="query.trim() !== '' ? t('memory.panel.list.emptyHint') : undefined"
          >
            <MemorySetupHint
              v-if="query.trim() === ''"
              :message="t('memory.panel.connectAgent.hint')"
              data-testid="memory-setup-hint-agent"
            />
          </EmptyState>
        </template>
        <template #default>
          <div class="relative" :style="{ height: `${totalSize}px` }">
            <button
              v-for="{ item, memory } in windowRows"
              :key="memory.id"
              :ref="measure"
              type="button"
              :data-index="item.index"
              v-bind="nav.rowProps(memory.id)"
              :class="
                cn(
                  rowVariants({ layout: 'double', selected: selectedId === memory.id }),
                  'w-full flex-col gap-1 whitespace-normal py-1.5 text-left',
                  memory.historical && 'opacity-50',
                  VIRTUAL_ROW_CLASS,
                )
              "
              :style="{ ...rowIndent(0), transform: `translateY(${item.start}px)` }"
              :data-testid="`memory-row-${memory.id}`"
              @click="selectMemory(memory.id)"
            >
              <span class="line-clamp-2 text-kira-md">{{ memory.fact }}</span>
              <span class="flex items-center gap-1">
                <Badge
                  :variant="
                    memory.origin === 'user'
                      ? 'info'
                      : memory.origin === 'external'
                        ? 'warn'
                        : 'default'
                  "
                  data-testid="memory-row-origin"
                  >{{
                    originText(memory.origin, memory.sourceTag)
                  }}</Badge
                >
                <Badge v-if="memory.kind === 'rule'" data-testid="memory-row-kind">{{
                  t(KIND_LABEL.rule)
                }}</Badge>
                <Badge data-testid="memory-row-scope">{{ t(SCOPE_LABEL[memory.scope]) }}</Badge>
                <span
                  v-if="memory.project"
                  class="min-w-0 max-w-40 font-data text-kira-sm text-muted-foreground"
                  data-testid="memory-row-project"
                  ><TruncatedText :text="memory.project" /></span
                >
                <Badge v-if="memory.historical" variant="warn">{{
                  t('memory.common.historical.status')
                }}</Badge>
                <Badge
                  v-if="memory.match"
                  :variant="memory.match === 'keyword' ? 'default' : 'info'"
                  :data-testid="`memory-match-${memory.match}`"
                  >{{
                    t(MATCH_LABEL[memory.match])
                  }}</Badge
                >
                <MemoryScore v-if="memory.score" :score="memory.score" />
                <span v-if="memory.versions > 1" class="text-kira-sm text-muted-foreground">{{
                  t('memory.common.version.label', { version: memory.version })
                }}</span>
              </span>
            </button>
          </div>
        </template>
      </QueryState>
    </div>
    <AddMemoryDialog v-if="addOpen" @close="addOpen = false" />
    <ImportConfirmDialog v-if="imports.confirmJobId" @close="imports.confirmJobId = null" />
  </div>
</template>
