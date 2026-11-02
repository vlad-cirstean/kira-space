<script setup lang="ts">
import { useMutation } from '@tanstack/vue-query';
import CodeBlock from '@theme/components/CodeBlock.vue';
import Disclosure from '@theme/components/Disclosure.vue';
import InlineState from '@theme/components/InlineState.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useAgentInfoFor } from '@workbench/agent/agentInfo';
import { useClaimPrompt } from '@workbench/prompts/promptsQueries';
import { computed, onMounted, ref } from 'vue';
import { useAutomationsModule } from '../module';
import { useRerun } from '../runScript';
import SmartBadge from '../smart/SmartBadge.vue';
import RunElapsed from './RunElapsed.vue';
import RunLog from './RunLog.vue';
import RunOutcomeBlock from './RunOutcomeBlock.vue';
import RunStatusBadge from './RunStatusBadge.vue';
import { useScriptRun, useSeenRuns, useStopScriptRun } from './runsQueries';

// The tab of one smart script run: status, result, the prompt sent, and the live log.
const props = defineProps<{ tab: { id: string; state: { runId: string; label?: string } } }>();
const ctx = useAutomationsModule();
const { run, query } = useScriptRun(() => props.tab.state.runId);
const stop = useStopScriptRun();
const decline = useMutation({
  mutationKey: ['scriptRuns', 'decline'],
  mutationFn: (id: string) => ctx.runs.confirmDecline(id),
});
const rerunRun = useRerun();
const error = ref<string | null>(null);
const { markSeen } = useSeenRuns();
onMounted(() => markSeen(props.tab.state.runId));

const { info: runAgent } = useAgentInfoFor(() => run.value?.provider ?? '');
const claim = useClaimPrompt();
const isSmart = computed(() => run.value?.kind === 'smart');
const live = computed(() => run.value?.state === 'running' || run.value?.state === 'waiting');
const ended = computed(() => run.value !== null && !live.value);
const canContinue = computed(
  () =>
    run.value !== null &&
    run.value.sessionId !== '' &&
    (run.value.state === 'blocked' || run.value.state === 'failed'),
);

async function continueInTerminal(): Promise<void> {
  const r = run.value;
  if (!r) return;
  const { command, token } = await ctx.runs.resumeCommand(r.id);
  ctx.openTerminalTab({
    cwd: r.cwd,
    launch: {
      command,
      label: runAgent.value.productLabel,
      color: 'none',
      kind: 'agent',
      launchToken: token,
    },
  });
}

async function rerun(): Promise<void> {
  if (run.value) error.value = await rerunRun(run.value);
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col gap-2 p-3" data-testid="script-run-view">
    <InlineState
      v-if="!run"
      :kind="query.isPending.value ? 'loading' : 'empty'"
      data-testid="script-run-missing"
    >
      {{
        query.isPending.value
          ? t('common.state.loading.loading')
          : t('automations.run.missing.emptyHint')
      }}
    </InlineState>
    <template v-else>
      <div class="flex items-center gap-2" data-testid="script-run-header">
        <SmartBadge v-if="isSmart" />
        <span class="min-w-0 truncate font-medium">{{ run.scriptName }}</span>
        <RunStatusBadge :state="run.state" />
        <RunElapsed :run="run" class="text-kira-sm text-muted-foreground" />
        <span v-if="isSmart" class="font-data text-kira-sm text-muted-foreground">{{
          run.model
        }}</span>
        <span
          v-if="run.resumedFrom"
          class="text-kira-sm text-muted-foreground"
          data-testid="script-run-resumed-from"
          >{{
            t('automations.run.resumedFrom.label')
          }}</span
        >
        <Button
          v-if="run.state === 'waiting'"
          variant="dialog-primary"
          size="kira-lg"
          class="ml-auto"
          data-testid="run-review"
          @click="claim.mutate(`schedule:${run.id}`)"
        >
          {{ t('automations.run.review.label') }}
        </Button>
        <Button
          v-if="run.state === 'waiting'"
          variant="dialog"
          size="kira-lg"
          data-testid="run-decline"
          @click="decline.mutate(run.id)"
        >
          {{ t('automations.run.decline.label') }}
        </Button>
        <Button
          v-if="run.state === 'running'"
          variant="dialog"
          size="kira-lg"
          class="ml-auto"
          data-testid="run-stop"
          @click="stop.mutate(run.id)"
        >
          {{ t('common.action.stop.label') }}
        </Button>
      </div>
      <div v-if="ended" class="flex flex-col gap-1.5" data-testid="script-run-result">
        <RunOutcomeBlock :run="run" @rerun="rerun" />
        <Button
          v-if="canContinue"
          variant="dialog"
          size="kira-lg"
          class="self-start"
          data-testid="run-continue"
          @click="continueInTerminal"
        >
          {{ t('automations.run.continue.label') }}
        </Button>
        <Alert v-if="error" variant="error"><AlertDescription>{{ error }}</AlertDescription></Alert>
      </div>
      <CodeBlock v-if="!isSmart" data-testid="script-run-command">{{ run.command }}</CodeBlock>
      <Disclosure
        v-if="isSmart"
        :label="t('automations.run.promptSent.label')"
        trigger-class="text-kira-sm text-muted-foreground"
        data-testid="script-run-prompt"
      >
        <CodeBlock class="mt-1">{{ run.prompt }}</CodeBlock>
      </Disclosure>
      <RunLog v-if="run.state !== 'waiting' && run.state !== 'skipped'" :run-id="run.id" />
    </template>
  </div>
</template>
