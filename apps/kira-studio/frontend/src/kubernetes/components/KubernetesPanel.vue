<script setup lang="ts">
import { useQueryClient } from '@tanstack/vue-query';
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { usePanelHeaderSearch } from '@workbench/behaviour/panelSearch';
import PanelBar from '@workbench/components/PanelBar.vue';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { ref, useTemplateRef } from 'vue';
import { useKubernetesUiStore } from '../state';
import ClusterList from './ClusterList.vue';

const ui = useKubernetesUiStore();
const qc = useQueryClient();

const rootEl = useTemplateRef<HTMLElement>('rootEl');
const { showSearch, toggleSearch } = usePanelHeaderSearch(rootEl, {
  searchable: () => true,
  getSearch: () => ui.clusterSearch,
  setSearch: (v) => {
    ui.clusterSearch = v;
  },
});

const refreshing = ref(false);
async function refresh(): Promise<void> {
  refreshing.value = true;
  try {
    await qc.invalidateQueries({ queryKey: ['kubernetes'] });
  } finally {
    refreshing.value = false;
  }
}
</script>

<template>
  <div ref="rootEl" class="flex h-full flex-col" data-testid="kubernetes-panel">
    <PanelHeader>
      {{ t('common.product.kubernetes.label') }}
      <template #actions>
        <TooltipIconButton
          icon="search"
          :label="
            showSearch ? t('common.action.hideSearch.label') : t('common.action.search.label')
          "
          :pressed="showSearch"
          data-testid="toggle-search"
          @click="toggleSearch"
        />
        <TooltipIconButton
          icon="refresh"
          :label="t('common.action.refresh.label')"
          data-testid="kube-refresh"
          :disabled="refreshing"
          @click="refresh"
        />
      </template>
    </PanelHeader>
    <PanelBar v-if="showSearch">
      <SearchField v-model="ui.clusterSearch" data-testid="tree-search" />
    </PanelBar>
    <ClusterList />
  </div>
</template>
