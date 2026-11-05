<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { t } from '@theme/i18n';
import { paneFrame } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { useAdeWorkflowsUiStore, type WorkflowsTab } from '../state/adeWorkflowsUi';
import AdeStepsTab from './AdeStepsTab.vue';
import AdeWorkflowsTab from './AdeWorkflowsTab.vue';
import AdeTaskStatuses from './statuses/AdeTaskStatuses.vue';

// Workflows page: Statuses, Steps (the reusable step library) and Workflows tabs.
const wfUi = useAdeWorkflowsUiStore();
const tabs = [
  {
    value: 'statuses',
    label: t('space.ade.workflows.tabs.statuses.label'),
    testid: 'ade-wf-tab-statuses',
  },
  { value: 'steps', label: t('space.ade.workflows.tabs.steps.label'), testid: 'ade-wf-tab-steps' },
  {
    value: 'workflows',
    label: t('space.common.workflows.label'),
    testid: 'ade-wf-tab-workflows',
  },
];

async function pick(next: string): Promise<void> {
  await wfUi.setTab(next as WorkflowsTab);
}
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col gap-0.5" data-testid="ade-workflows">
    <SecondaryTabs
      class="px-1 py-0.5"
      size="kira-lg"
      :model-value="wfUi.tab"
      :items="tabs"
      :aria-label="t('space.ade.workflows.tabs.aria')"
      data-testid="ade-wf-tabs"
      @update:model-value="pick"
    />
    <AdeWorkflowsTab v-if="wfUi.tab === 'workflows'" />
    <AdeStepsTab v-else-if="wfUi.tab === 'steps'" />
    <div v-else :class="cn(paneFrame, 'flex flex-1 flex-col')">
      <PanelHeader>{{ t('space.ade.workflows.page.statuses.label') }}</PanelHeader>
      <div class="min-h-0 flex-1 overflow-auto p-4">
        <AdeTaskStatuses />
      </div>
    </div>
  </div>
</template>
