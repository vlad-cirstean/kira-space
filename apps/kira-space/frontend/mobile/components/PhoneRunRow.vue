<script setup lang="ts">
import type { PhoneRun } from '@shared/domain/mobile';
import { t } from '@theme/i18n';
import RunElapsed from '@workbench/automations/runs/RunElapsed.vue';
import RunStatusBadge from '@workbench/automations/runs/RunStatusBadge.vue';
import { computed } from 'vue';

// One script run: state, name, task, elapsed, and why a finished one ended. Never its command or output.
const props = defineProps<{ run: PhoneRun }>();

const why = computed(() => {
  const out = props.run.outcome;
  if (!out) return '';
  if (out.reason) return out.reason;
  return out.exitCode === undefined || out.exitCode === 0
    ? ''
    : t('automations.run.outcome.exitCode.label', { code: out.exitCode });
});
</script>

<template>
  <li
    class="flex flex-col gap-1 border-b border-border px-3 py-3"
    data-testid="phone-run"
    :data-run-id="run.id"
    :data-state="run.state"
  >
    <span class="flex items-center gap-2">
      <RunStatusBadge :state="run.state" />
      <span class="min-w-0 flex-1 truncate text-kira-md" data-testid="phone-run-name">{{
        run.scriptName
      }}</span>
      <span class="text-kira-sm text-subtle"><RunElapsed :run="run" /></span>
    </span>
    <span v-if="run.taskTitle" class="truncate text-kira-sm text-muted-foreground">{{
      run.branchLabel ? `${run.taskTitle} · ${run.branchLabel}` : run.taskTitle
    }}</span>
    <span v-if="why" class="text-kira-sm text-muted-foreground" data-testid="phone-run-why">{{
      why
    }}</span>
  </li>
</template>
