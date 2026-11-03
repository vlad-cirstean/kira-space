<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import { t } from '@theme/i18n';
import { computed, watch } from 'vue';
import { usePlanModel } from '../plan/usePlanModel';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import AdeArchivedPanel from './AdeArchivedPanel.vue';
import AdeBranchPanel from './AdeBranchPanel.vue';
import AdePanelPane from './AdePanelPane.vue';
import AdeTaskPanel from './AdeTaskPanel.vue';

// Right-hand panel of the Plan: the selected task, or one of its branches.
const ui = useAdeBoardUiStore();
const tabs = useAdePanelTabsStore();
const { model } = usePlanModel();

watch(
  () => model.value?.board,
  (b) => {
    if (b) tabs.prune(new Set(b.tasks.map((x) => x.id)), new Set(b.branches.map((x) => x.id)));
  },
);

const card = computed(() =>
  ui.selectedTaskId ? (model.value?.cardFor(ui.selectedTaskId) ?? null) : null,
);
/** The selected task when it is archived: it left the board and survives in History. */
const archivedId = computed(() =>
  !card.value && model.value?.board.history.some((h) => h.taskId === ui.selectedTaskId)
    ? ui.selectedTaskId
    : null,
);
const row = computed(() => card.value?.rows.find((r) => r.id === ui.selectedBranchId) ?? null);
</script>

<template>
  <AdePanelPane>
    <template v-if="model && card">
      <AdeBranchPanel v-if="row" :card="card" :row="row" :model="model" />
      <AdeTaskPanel v-else :card="card" :model="model" />
    </template>
    <AdeArchivedPanel v-else-if="archivedId" :task-id="archivedId" />
    <EmptyState
      v-else
      fill
      :title="t('space.ade.panel.empty.emptyHint')"
      data-testid="ade-panel-empty"
    />
  </AdePanelPane>
</template>
