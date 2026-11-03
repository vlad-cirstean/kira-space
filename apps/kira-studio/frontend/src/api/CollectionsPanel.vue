<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { usePanelHeaderSearch } from '@workbench/behaviour/panelSearch';
import PanelBar from '@workbench/components/PanelBar.vue';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { registerCommand } from '@workbench/shortcuts/commands';
import { computed, onMounted, onUnmounted, ref, useTemplateRef } from 'vue';
import ApiConfigProblems from './ApiConfigProblems.vue';
import CollectionsTree from './CollectionsTree.vue';
import ImportReportStrip from './ImportReportStrip.vue';
import { useCollectionSelectionStore } from './state/collectionSelection';
import { useCollectionsStore } from './state/collections';
import { useImportCurlStore } from './state/curl';
import { useDynamicValuesStore } from './state/dynamicValues';
import { useVariablesStore } from './state/variables';
import { openApiRequestTab, openVariableSetTab } from './tabs';

const dynamicValuesStore = useDynamicValuesStore();
const collectionsStore = useCollectionsStore();
const selection = useCollectionSelectionStore();
const importCurlStore = useImportCurlStore();
const variablesStore = useVariablesStore();

// The placeholder is gone — this is a real tree now, mounted through the same panel-shell
// geometry Studio's ProjectPanel.vue uses. `empty` is no longer hardcoded: it is "this app has no
// collections yet", which is also what gates the search box below.
//
// The header's own `new-request` testid is the one front door left — the empty state's duplicate
// `new-request-empty`/`new-collection-empty`/`import-collection-empty` actions were removed
// entirely; tests/ui/api-ui-consistency.spec.ts asserts none of the three exist, not that they are
// preserved.
// Back to "no collections" alone. It had been widened to also require no environments, because the
// environments category lived in this panel and a project with environments but no collections
// still had something to show; with that category gone, this panel shows collections and nothing
// else, so an environment must not keep the empty state away.
const empty = computed(() => collectionsStore.collections.length === 0);

// useCollectionsStore's own app-lifetime query observer fetches the tree on store creation
// (same as useVariablesStore's own environments query below) — there is nothing left to trigger
// on mount here. PanelShell still renders #body only once `empty` reads false, which now follows
// the query's own data rather than a mount-triggered fetch.

// Runtime-only collapse state for the two categories — a panel section is not a
// preference worth a storage round trip (ConsoleViewRuntime's own "runtime-only, never saved"
// rule). Collections starts expanded (today's only view of the tree); Environments starts
// collapsed (it was behind a dialog before this, so a user has never had it open by default).
const collectionsExpanded = ref(true);
function onSearch(value: string): void {
  collectionsStore.search = value;
}

// P104 §3, P107 T1-17: PanelShell's own header/search-reveal/type-ahead-redirect logic, via the
// shared usePanelHeaderSearch composable rather than an inline copy.
const rootEl = useTemplateRef<HTMLElement>('rootEl');
const { showSearch, toggleSearch } = usePanelHeaderSearch(rootEl, {
  searchable: () => true,
  getSearch: () => collectionsStore.search,
  setSearch: onSearch,
});

function onNewCollection(): void {
  void collectionsStore.createCollection();
}

function onImport(): void {
  void collectionsStore.importPostman();
}

// The palette's Variables… entry — opens the tab for
// whichever collection is currently selected in the tree (a collection row directly, or a
// folder/request's own collection). A no-op with nothing selected, the same "view-scoped, no-op
// elsewhere" shape view.run/api.save already have.
function onVariablesCommand(): void {
  const selected = selection.focusKey;
  if (!selected) return;
  if (selected.startsWith('c:')) {
    const id = selected.slice(2);
    const collection = collectionsStore.collectionRecord(id);
    if (collection) openVariableSetTab('collection', id, collection.name);
    return;
  }
  if (selected.startsWith('i:')) {
    const item = collectionsStore.itemRecord(selected.slice(2));
    const collection = item ? collectionsStore.collectionRecord(item.collectionId) : undefined;
    if (item && collection) {
      openVariableSetTab('collection', item.collectionId, collection.name);
    }
  }
}

function onEnvironments(): void {
  variablesStore.openEnvironments();
}

// P6 D11: the palette's own "Dynamic values…" entry — not scoped to any selection, unlike
// Variables… above.
function onDynamicValues(): void {
  dynamicValuesStore.openDynamicValuesDialog();
}

// P7 D12: the palette's own "Import from curl…" entry — same shape as the three above, not
// tab-scoped.
function onImportCurl(): void {
  importCurlStore.openImportCurlDialog();
}

// D15: the palette's Import collection…, Variables…, Environments…, (P6) Dynamic values… and
// (P7) Import from curl… entries. Registered by the panel rather than a view, since the panel is
// mounted for the whole of Api mode — none is tab-scoped.
let unregisterCommands: Array<() => void> = [];
onMounted(() => {
  unregisterCommands = [
    registerCommand('api.import', onImport),
    registerCommand('api.variables', onVariablesCommand),
    registerCommand('api.environments', onEnvironments),
    registerCommand('api.dynamicValues', onDynamicValues),
    registerCommand('api.importCurl', onImportCurl),
  ];
});
onUnmounted(() => {
  for (const off of unregisterCommands) off();
});
</script>

<template>
  <!-- P104 §3: PanelShell inlined (no library counterpart). -->
  <div ref="rootEl" class="flex h-full min-h-0 flex-col">
    <PanelHeader>
      {{ t('studio.api.collections.title') }}
      <template #actions>
        <TooltipIconButton
          :icon="ICON.search"
          :label="
            showSearch ? t('common.action.hideSearch.label') : t('common.action.search.label')
          "
          :aria-label="t('common.action.search.label')"
          :pressed="showSearch"
          data-testid="toggle-search"
          @click="toggleSearch"
        />
        <TooltipIconButton
          :icon="ICON.add"
          :label="t('studio.api.common.newRequest.label')"
          data-testid="new-request"
          @click="openApiRequestTab"
        />
        <TooltipIconButton
          :icon="ICON.newCollection"
          :label="t('common.newCollection.label')"
          data-testid="new-collection"
          @click="onNewCollection"
        />
        <!-- The Postman import moved to the menu bar (App → Import Postman Collection…)
           and to the command palette entry it already had. collectionsState.busy still gates
           re-entry inside importPostman() itself, so a second import cannot start while one
           is running. -->
        <!-- P5 D3/D11: the environments dialog's own entry point — environments exist
           independently of collections, so this lives in the panel's header, not the tree. -->
        <TooltipIconButton
          :icon="ICON.environments"
          :label="t('studio.api.common.environments.label')"
          data-testid="api-environments"
          @click="onEnvironments"
        />
      </template>
    </PanelHeader>
    <template v-if="!empty">
      <PanelBar v-if="showSearch" data-testid="collections-search-group">
        <SearchField
          :model-value="collectionsStore.search"
          data-testid="tree-search"
          @update:model-value="onSearch"
        />
      </PanelBar>
      <div class="min-h-0 flex-1">
        <div class="flex h-full min-h-0 flex-col">
          <ImportReportStrip />
          <!-- P22b D8: the collections tree's own category -- flex: 1 always, so its own collapse
               (a rarer action than environments', which starts collapsed by default) never fights
               environments-category's fixed 40% cap below for space. -->
          <div class="flex flex-1 min-h-0 flex-col" :class="{ collapsed: !collectionsExpanded }">
            <SectionHeading
              :label="t('studio.api.collections.title')"
              collapsible
              :expanded="collectionsExpanded"
              data-testid="collections-category-toggle"
              @toggle="collectionsExpanded = !collectionsExpanded"
            />
            <CollectionsTree v-if="collectionsExpanded" class="flex-1 min-h-0" />
          </div>
        </div>
      </div>
      <ApiConfigProblems />
    </template>
    <div v-else class="flex min-h-0 flex-1 flex-col">
      <ImportReportStrip />
      <EmptyState
        fill
        data-testid="collections-empty"
        icon="folder-library"
        :title="t('studio.api.collections.empty')"
        :description="t('studio.api.collections.emptyHint')"
      />
      <ApiConfigProblems />
    </div>
  </div>
</template>
