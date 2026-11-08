<script setup lang="ts">
import type { ScriptRun } from '@shared/domain/scriptRuns';
import { Button } from '@theme/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { useCopy } from '@workbench/behaviour/clipboard';
import { formatCurrency } from '@workbench/behaviour/format';
import { computed } from 'vue';
import { errorText } from '../../bridge/codedError';
import { useResumeScriptRun } from './runsQueries';
import { runReportText } from './runText';

// The run's result: reason, exit code, and the two actions. `tail` supplies the last terminal
// lines for Copy for agent; `compact` drops the detail lines for the terminal tab strip.
const props = defineProps<{
  run: ScriptRun;
  compact?: boolean;
  tail?: () => Promise<string>;
}>();
const emit = defineEmits<{ rerun: [] }>();

const { copy, copied, error: copyError } = useCopy();
const resume = useResumeScriptRun();
const resumeError = computed(() => (resume.error.value ? errorText(resume.error.value) : ''));

async function copyForAgent(): Promise<void> {
  const tail = props.tail ? await props.tail() : '';
  await copy(runReportText(props.run, tail), {
    message: t('automations.run.reportCopied.announce'),
  });
}
</script>

<template>
  <div class="flex flex-col gap-1 text-kira-sm" data-testid="run-outcome">
    <span class="select-text" data-testid="run-outcome-reason">{{
      run.outcome?.reason || t('automations.run.outcome.none.emptyHint')
    }}</span>
    <span v-if="run.outcome?.summary" class="select-text" data-testid="run-outcome-summary">{{
      run.outcome.summary
    }}</span>
    <span
      v-if="run.outcome?.costUsd !== undefined"
      class="text-muted-foreground"
      data-testid="run-outcome-cost"
    >
      {{ t('common.cost.label', { value: formatCurrency(run.outcome.costUsd) }) }}
    </span>
    <span
      v-if="run.outcome?.permissionDenials?.length"
      class="text-muted-foreground"
      data-testid="run-outcome-denials"
    >
      {{
        t('automations.run.outcome.denied.label', {
          tools: run.outcome.permissionDenials.join(', '),
        })
      }}
    </span>
    <template v-if="!compact">
      <span
        v-if="run.outcome?.exitCode !== undefined"
        class="text-muted-foreground"
        data-testid="run-outcome-exit"
      >
        {{ t('automations.run.outcome.exitCode.label', { code: run.outcome.exitCode }) }}
      </span>
      <span
        v-if="run.outcome?.lastError"
        class="select-text font-data text-error whitespace-pre-wrap"
        data-testid="run-outcome-error"
      >
        {{ run.outcome.lastError }}
      </span>
      <span
        v-if="run.outcome?.outputTail"
        class="select-text font-data text-error whitespace-pre-wrap"
        data-testid="run-outcome-tail"
      >
        {{ run.outcome.outputTail }}
      </span>
    </template>
    <span v-if="copyError" class="text-error">{{ copyError }}</span>
    <span v-if="resumeError" class="text-error" data-testid="run-resume-error">{{
      resumeError
    }}</span>
    <div v-if="run.state !== 'running' && run.state !== 'waiting'" class="flex gap-1">
      <Button variant="dialog" size="kira-lg" data-testid="run-copy" @click="copyForAgent">
        {{ copied ? t('common.copy.copied.announce') : t('common.copyForAgent.label') }}
      </Button>
      <Tooltip v-if="run.resumable">
        <TooltipTrigger as-child>
          <Button
            variant="dialog"
            size="kira-lg"
            :pending="resume.isPending.value"
            data-testid="run-resume"
            @click="resume.mutate(run)"
          >
            {{ t('common.action.resume.label') }}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{{ t('automations.run.resume.tooltip') }}</TooltipContent>
      </Tooltip>
      <Button
        v-if="run.scriptId"
        variant="dialog"
        size="kira-lg"
        data-testid="run-again"
        @click="emit('rerun')"
      >
        {{ t('automations.run.again.label') }}
      </Button>
    </div>
  </div>
</template>
