<script setup lang="ts">
import type { ScriptRun } from '@shared/domain/scriptRuns';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { usePersistedState } from '@workbench/behaviour/persisted';
import { computed } from 'vue';
import { useAutomationsModule } from '../module';
import { keyOf, useScriptTabs } from '../scriptTabs';
import SmartBadge from '../smart/SmartBadge.vue';
import RunElapsed from './RunElapsed.vue';
import RunStatusBadge from './RunStatusBadge.vue';
import { useScriptRuns, useStopScriptRun } from './runsQueries';

// Live runs only; finished ones are in the Operations panel.
const ctx = useAutomationsModule();
const tabs = useScriptTabs();
const open = usePersistedState('kira.automations.runsOpen', true);
const { data } = useScriptRuns();
const stop = useStopScriptRun();

const rows = computed(() =>
  (data.value ?? []).filter((r) => r.state === 'running' || r.state === 'waiting'),
);

function show(run: ScriptRun): void {
  // A run without a terminal (smart or headless) lives in its own tab; a terminal run focuses its tab.
  if (run.kind === 'smart' || run.terminalId === '') {
    tabs.openRun({ ...keyOf(run), runId: run.id, label: run.scriptName });
    return;
  }
  if (ctx.tabs.list().some((tab) => tab.id === run.terminalId)) ctx.tabs.activate(run.terminalId);
}
</script>

<template>
  <section :class="cn('flex min-h-0 flex-col', open && 'max-h-1/2')" data-testid="runs-section">
    <SectionHeading
      :label="t('common.state.running.status')"
      collapsible
      :expanded="open"
      :count="rows.length"
      data-testid="runs-toggle"
      @toggle="open = !open"
    />
    <div v-if="open" class="flex min-h-0 flex-col overflow-y-auto">
      <template v-if="rows.length === 0">
        <InlineState kind="empty" data-testid="runs-empty">
          {{ t('automations.runs.none.emptyHint') }}
        </InlineState>
        <div class="px-1.5 pb-1.5">
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="runs-show-history"
            @click="ctx.showOperations()"
          >
            {{ t('automations.runs.showHistory.label') }}
          </Button>
        </div>
      </template>
      <div
        v-for="run in rows"
        :key="run.id"
        class="flex flex-col"
        data-testid="run-row"
        :data-state="run.state"
      >
        <div :class="cn(rowVariants({ layout: 'tree' }), 'py-1 pl-1.5')">
          <button
            type="button"
            class="flex min-w-0 flex-1 items-center gap-1 border-0 bg-transparent p-0 text-left text-inherit"
            @click="show(run)"
          >
            <RunStatusBadge :state="run.state" />
            <SmartBadge v-if="run.kind === 'smart'" />
            <span class="min-w-0 flex-1 truncate">
              {{ run.scriptName }}
              <span
                v-if="run.trigger === 'ade'"
                class="text-muted-foreground"
                data-testid="run-trigger"
              >
                · {{ t('automations.run.triggerAde.label', { task: run.taskTitle }) }}
              </span>
              <span
                v-else-if="run.trigger === 'scheduled'"
                class="text-muted-foreground"
                data-testid="run-trigger"
              >
                · {{ t('automations.run.triggerScheduled.label') }}
              </span>
            </span>
            <RunElapsed :run="run" class="text-kira-sm text-muted-foreground" />
          </button>
          <Button
            v-if="run.state === 'running'"
            variant="dialog"
            size="kira-lg"
            data-testid="run-stop"
            @click="stop.mutate(run.id)"
          >
            {{ t('common.action.stop.label') }}
          </Button>
        </div>
      </div>
    </div>
  </section>
</template>
