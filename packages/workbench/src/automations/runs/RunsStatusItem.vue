<script setup lang="ts">
import TooltipDetails from '@theme/components/TooltipDetails.vue';
import { t } from '@theme/i18n';
import { usePersistedState } from '@workbench/behaviour/persisted';
import StatusBarItem from '@workbench/components/StatusBarItem.vue';
import { computed } from 'vue';
import { useAutomationsModule } from '../module';
import { useScriptRuns } from './runsQueries';

// Status-bar count of live Automations runs; hidden at zero. A click opens the Runs section.
const ctx = useAutomationsModule();
const { data } = useScriptRuns();
const runsOpen = usePersistedState('kira.automations.runsOpen', true);
const running = computed(() => (data.value ?? []).filter((r) => r.state === 'running').length);

function show(): void {
  runsOpen.value = true;
  ctx.showAutomations();
}
</script>

<template>
  <StatusBarItem v-if="running > 0" icon="play" data-testid="status-runs" @click="show">
    {{ t('common.state.runningCount.status', { count: running }) }}
    <template #tip>
      <TooltipDetails
        :title="t('common.scriptRuns.label')"
        :rows="[{ label: t('common.state.running.status'), value: String(running) }]"
        :hint="t('automations.statusBar.runs.open.hint')"
      />
    </template>
  </StatusBarItem>
</template>
