<script setup lang="ts">
import type { ScriptRun } from '@shared/domain/scriptRuns';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useAutomationsModule } from '@workbench/automations/module';
import RunElapsed from '@workbench/automations/runs/RunElapsed.vue';
import RunOutcomeBlock from '@workbench/automations/runs/RunOutcomeBlock.vue';
import RunStatusBadge from '@workbench/automations/runs/RunStatusBadge.vue';
import {
  useSeenRuns,
  useStopScriptRun,
  useTaskScriptRuns,
} from '@workbench/automations/runs/runsQueries';
import { keyOf, useScriptTabs } from '@workbench/automations/scriptTabs';
import SmartBadge from '@workbench/automations/smart/SmartBadge.vue';
import { computed, ref } from 'vue';

const props = defineProps<{ taskId: string }>();
const ctx = useAutomationsModule();
const { data } = useTaskScriptRuns(() => props.taskId);
const stop = useStopScriptRun();
const { markSeen } = useSeenRuns();
const scriptTabs = useScriptTabs();
const expanded = ref<string | null>(null);
const rows = computed(() => data.value ?? []);

function toggle(id: string): void {
  markSeen(id);
  expanded.value = expanded.value === id ? null : id;
}

function openRun(run: ScriptRun): void {
  ctx.showAutomations();
  scriptTabs.openRun({ ...keyOf(run), runId: run.id, label: run.scriptName });
}
</script>

<template>
  <section
    v-if="rows.length > 0"
    class="flex max-h-48 flex-col gap-1 overflow-y-auto"
    data-testid="ade-automations"
  >
    <SectionHeading :label="t('common.scriptRuns.label')" />
    <div
      v-for="run in rows"
      :key="run.id"
      class="flex flex-col"
      data-testid="ade-automation-row"
      :data-state="run.state"
    >
      <div class="flex items-center gap-1.5">
        <Button
          variant="toolbar"
          size="inline"
          class="min-w-0 flex-1 justify-start gap-1.5 font-normal text-fg"
          :aria-expanded="expanded === run.id"
          @click="toggle(run.id)"
        >
          <RunStatusBadge :state="run.state" />
          <SmartBadge v-if="run.kind === 'smart'" />
          <span class="min-w-0 truncate">{{ run.scriptName }}</span>
          <span
            v-if="run.branchLabel"
            class="shrink-0 font-data text-kira-sm text-muted-foreground"
            >{{
              run.branchLabel
            }}</span
          >
          <RunElapsed :run="run" class="ml-auto shrink-0 text-kira-sm text-muted-foreground" />
        </Button>
        <Button
          v-if="run.kind === 'smart'"
          variant="toolbar"
          size="kira"
          data-testid="ade-automation-open"
          @click="openRun(run)"
          ><CodiconIcon name="go-to-file" :size="13" />{{ t('common.action.open.label') }}
        </Button>
        <Button
          v-if="run.state === 'running'"
          variant="toolbar"
          size="kira"
          data-testid="ade-automation-stop"
          @click="stop.mutate(run.id)"
          ><CodiconIcon name="debug-stop" :size="13" />{{ t('common.action.stop.label') }}
        </Button>
      </div>
      <div v-if="expanded === run.id" class="pt-1">
        <RunOutcomeBlock :run="run" />
      </div>
    </div>
  </section>
</template>
