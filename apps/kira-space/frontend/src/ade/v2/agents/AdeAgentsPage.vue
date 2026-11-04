<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { t } from '@theme/i18n';
import { paneFrame } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { computed } from 'vue';
import AdeContextPane from '../context/AdeContextPane.vue';
import AdeSettingsPane from '../settings/AdeSettingsPane.vue';
import { type AgentsTab, useAdeAgentsUiStore } from '../state/adeAgentsUi';

// Agent config: the Context and Settings editors as sub-tabs of one page.
const agentsUi = useAdeAgentsUiStore();

const items = computed(() => [
  {
    value: 'context',
    label: t('space.ade.context.tab.label'),
    testid: 'ade-agents-tab-context',
  },
  {
    value: 'settings',
    label: t('common.module.settings.label'),
    testid: 'ade-agents-tab-settings',
  },
]);
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 gap-0.5" data-testid="ade-agents">
    <div :class="cn(paneFrame, 'flex flex-1 flex-col')">
      <PanelHeader>
        <template #start>
          <SecondaryTabs
            :model-value="agentsUi.tab"
            :items="items"
            :aria-label="t('space.ade.agents.tab.label')"
            @update:model-value="(v) => void agentsUi.openTab(v as AgentsTab)"
          />
        </template>
      </PanelHeader>
      <AdeContextPane v-if="agentsUi.tab === 'context'" />
      <AdeSettingsPane v-else />
    </div>
  </div>
</template>
