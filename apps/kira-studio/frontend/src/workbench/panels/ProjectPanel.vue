<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { usePanelHeaderSearch } from '@workbench/behaviour/panelSearch';
import PanelBar from '@workbench/components/PanelBar.vue';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { computed, useTemplateRef } from 'vue';
import CredentialsUpdateDialog from '../../project/CredentialsUpdateDialog.vue';
import FiltersDialog from '../../project/FiltersDialog.vue';
import ProjectTree from '../../project/ProjectTree.vue';
import SchemaDialog from '../../project/SchemaDialog.vue';
import { useCredentialsDialogStore } from '../../project/state/credentialsDialog';
import { useTreeStore } from '../../project/state/tree';
import { useConnectionConfig } from '../../state/connectionConfig';
import { useConnectionDialogStore } from '../../state/connectionDialogs';
import { useConnectionsStore } from '../../state/connections';
import { useSchemaDialogStore } from '../../state/schemas';

const connectionsStore = useConnectionsStore();
const connectionDialogStore = useConnectionDialogStore();
const schemaDialogStore = useSchemaDialogStore();
const credentialsDialogStore = useCredentialsDialogStore();
const treeStore = useTreeStore();

const config = useConnectionConfig();
// A file that never held a valid connection is listed in the tree, so it keeps the panel off the
// empty state.
const hasBrokenFile = computed(() =>
  (config.data.value?.files ?? []).some((f) => f.problem !== null && f.id === ''),
);
const empty = computed(() => connectionsStore.records.length === 0 && !hasBrokenFile.value);
// P104 §3: PanelShell's own header/search-reveal/type-ahead-redirect logic, inlined via the
// shared usePanelHeaderSearch composable rather than kept as a wrapper component.
const rootEl = useTemplateRef<HTMLElement>('rootEl');
const { showSearch, toggleSearch } = usePanelHeaderSearch(rootEl, {
  searchable: () => true,
  getSearch: () => treeStore.search,
  setSearch: (v) => {
    treeStore.search = v;
  },
});
</script>

<template>
  <div ref="rootEl" class="flex h-full min-h-0 flex-col">
    <PanelHeader>
      {{ t('common.module.connections.label') }}
      <template #actions>
        <TooltipIconButton
          :icon="ICON.search"
          :label="
            showSearch ? t('common.action.hideSearch.label') : t('common.action.search.label')
          "
          :pressed="showSearch"
          data-testid="toggle-search"
          @click="toggleSearch"
        />
        <!-- The DataGrip import moved to the menu bar (App → Import DataGrip
             Connections…). It is a once-per-machine action and did not earn a permanent slot in a
             four-button header. StudioStart.vue's own first-run button stays: an empty state is
             exactly when a menu-bar-only affordance is hardest to find. -->
        <TooltipIconButton
          :icon="ICON.add"
          :label="t('studio.project.connectionDialog.newConnection.title')"
          data-testid="add-connection"
          @click="connectionDialogStore.openNew"
        />
      </template>
    </PanelHeader>
    <template v-if="!empty">
      <PanelBar v-if="showSearch">
        <SearchField v-model="treeStore.search" data-testid="tree-search" />
      </PanelBar>
      <div class="min-h-0 flex-1">
        <ProjectTree />
      </div>
    </template>
    <!-- FirstRun.html's empty pane: says what the panel is for, nothing more — the headline
         already lives on the main start page, so it is not repeated here. -->
    <EmptyState v-else fill icon="database" :title="t('studio.project.panel.empty.empty')" />
  </div>
  <FiltersDialog />
  <CredentialsUpdateDialog v-if="credentialsDialogStore.open" />
  <SchemaDialog v-if="schemaDialogStore.open" />
</template>
