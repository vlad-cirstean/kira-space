<script setup lang="ts">
import type { ScriptRun } from '@shared/domain/scriptRuns';
import { FieldError } from '@theme/components/ui/field';
import { computed } from 'vue';
import { useAutomationsModule } from '../automations/module';
import { useRerun } from '../automations/runScript';
import RunElapsed from '../automations/runs/RunElapsed.vue';
import RunOutcomeBlock from '../automations/runs/RunOutcomeBlock.vue';
import RunStatusBadge from '../automations/runs/RunStatusBadge.vue';
import { useScriptRunByTerminal } from '../automations/runs/runsQueries';
import { useAsyncAction } from '../behaviour/asyncAction';
// P128 §2.5: the one shared terminal tab view, wrapping TerminalHostView.vue for both apps. A repo
// terminal and a module terminal are the same tab kind, differing only in workspaceId/codeRepoId.
// P242: a script tab also shows its run status strip and, once the run ends, its result block.
import TerminalHostView, { type TerminalHostTabState } from './TerminalHostView.vue';
import { loadTerminalRenderer } from './terminalRendererLoader';

const props = defineProps<{ tab: { id: string; state: TerminalHostTabState } }>();
const ctx = useAutomationsModule();
const run = useScriptRunByTerminal(() => props.tab.id);
const scriptRun = computed(() => (props.tab.state.launchKind === 'script' ? run.value : null));
const ended = computed(() => scriptRun.value !== null && scriptRun.value.state !== 'running');

const rerunRun = useRerun();
const rerun = useAsyncAction((run: ScriptRun) => rerunRun(run));

async function tail(): Promise<string> {
  return (await loadTerminalRenderer()).tailTerminal(props.tab.id, 40);
}
</script>

<template>
  <TerminalHostView :tab="tab" :deps="ctx.host" :hide-footer="ended">
    <div
      v-if="scriptRun"
      class="flex shrink-0 items-center gap-2 bg-chrome px-1 py-0.5 text-kira-sm"
      data-testid="script-run-strip"
    >
      <RunStatusBadge :state="scriptRun.state" />
      <RunElapsed :run="scriptRun" class="text-muted-foreground" />
    </div>
    <template v-if="scriptRun && ended" #outcome>
      <div class="shrink-0 bg-chrome px-1 py-1" data-testid="script-run-outcome">
        <RunOutcomeBlock :run="scriptRun" compact :tail="tail" @rerun="rerun.run(scriptRun)" />
        <FieldError v-if="rerun.error.value" data-testid="script-run-error">{{
          rerun.error.value
        }}</FieldError>
      </div>
    </template>
  </TerminalHostView>
</template>
