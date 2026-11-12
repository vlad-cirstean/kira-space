<script setup lang="ts">
import { t } from '@theme/i18n';
import { useAutomationsModule } from '@workbench/automations/module';
import RunElapsed from '@workbench/automations/runs/RunElapsed.vue';
import { keyOf, useScriptTabs } from '@workbench/automations/scriptTabs';
import SmartBadge from '@workbench/automations/smart/SmartBadge.vue';
import { computed } from 'vue';
import { useTabsStore } from '../../../state/tabs';
import AdeChip from '../AdeChip.vue';
import AdeTip from '../AdeTip.vue';
import { runReason } from './automationRuns';
import { useChipRuns } from './useChipRuns';

const props = defineProps<{ taskId: string }>();
const ctx = useAutomationsModule();
const tabs = useTabsStore();
const scriptTabs = useScriptTabs();
const { byTask, markSeen } = useChipRuns();
const run = computed(() => byTask.value.get(props.taskId) ?? null);

const tone = computed(() => {
  const state = run.value?.state;
  if (state === 'failed') return 'red';
  return state === 'blocked' ? 'amber' : 'blue';
});
const label = computed(() => {
  const name = run.value?.scriptName ?? '';
  const state = run.value?.state;
  if (state === 'failed') return t('space.common.nameFailed.label', { name });
  return state === 'blocked' ? t('space.ade.automation.chipBlocked.label', { name }) : name;
});
const tip = computed(() => (run.value ? runReason(run.value) || run.value.scriptName : ''));

function open(): void {
  const r = run.value;
  if (!r) return;
  markSeen(r.id);
  ctx.showAutomations();
  if (r.kind === 'smart') {
    scriptTabs.openRun({ ...keyOf(r), runId: r.id, label: r.scriptName });
  } else if (r.terminalId) tabs.activateTab(r.terminalId);
}
</script>

<template>
  <AdeTip v-if="run" :text="tip">
    <button
      type="button"
      class="inline-flex shrink-0 items-center gap-1 border-0 bg-transparent p-0 text-inherit"
      data-testid="ade-automation-chip"
      :data-state="run.state"
      @click.stop="open"
    >
      <SmartBadge v-if="run.kind === 'smart'" />
      <AdeChip :label="label" :tone="tone" class="max-w-40 truncate" />
      <RunElapsed
        v-if="run.state === 'running'"
        :run="run"
        class="text-kira-sm text-muted-foreground"
      />
    </button>
  </AdeTip>
</template>
