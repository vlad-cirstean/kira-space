<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref } from 'vue';
import type { CardModel } from '../plan/usePlanModel';
import { useSetTaskWorkflow, useWorkflows } from '../queries';
import AdeTaskActionButton from '../run/AdeTaskActionButton.vue';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import AdeRow from './AdeRow.vue';
import AdeRowDetail from './AdeRowDetail.vue';
import AdeStepBlock from './AdeStepBlock.vue';
import AdeStepMover from './AdeStepMover.vue';

// Task tab Workflow section (SPEC2 section 7): the workflow picker, then one block per step.
const props = defineProps<{ card: CardModel }>();
const wfUi = useAdeWorkflowsUiStore();
const workflows = useWorkflows();
const setWorkflow = useSetTaskWorkflow();
const error = ref('');

const valid = computed(() =>
  (workflows.data.value?.workflows ?? []).flatMap((e) =>
    e.workflow ? [{ file: e.fileName, wf: e.workflow }] : [],
  ),
);

async function pick(value: unknown): Promise<void> {
  error.value = '';
  try {
    await setWorkflow.mutateAsync({ taskId: props.card.task.id, workflowId: String(value ?? '') });
  } catch (err) {
    error.value = errorText(err);
  }
}

function edit(): void {
  const hit = valid.value.find((v) => v.wf.id === props.card.task.workflowId);
  wfUi.openWorkflow(hit?.file ?? null);
}
</script>

<template>
  <div class="flex flex-col gap-1" data-testid="ade-workflow-block">
    <AdeRow :label="t('space.common.workflow.label')" label-for="ade-workflow-select">
      <NativeSelect
        id="ade-workflow-select"
        variant="bordered"
        size="kira"
        :model-value="card.task.workflowId"
        data-testid="ade-workflow-select"
        @update:model-value="pick"
      >
        <option v-if="card.task.workflowId === ''" value="">{{
          t('space.ade.workflowBlock.choose.option')
        }}</option>
        <option v-for="v in valid" :key="v.wf.id" :value="v.wf.id">{{ v.wf.name }}</option>
      </NativeSelect>
      <Tooltip v-if="card.task.workflowOutdated">
        <TooltipTrigger as-child>
          <Badge variant="warn" data-testid="ade-workflow-outdated">{{
            t('space.ade.workflowBlock.outdated.status')
          }}</Badge>
        </TooltipTrigger>
        <TooltipContent>{{ t('space.ade.workflowBlock.outdatedHint.hint') }}</TooltipContent>
      </Tooltip>
      <template #actions>
        <Button variant="dialog" size="kira" data-testid="ade-edit-workflows" @click="edit">
          {{ t('space.ade.workflowBlock.edit.label') }}
        </Button>
      </template>
    </AdeRow>
    <AdeRowDetail v-if="error">
      <Alert variant="error" data-testid="ade-workflow-error"
        ><AlertDescription>{{ error }}</AlertDescription></Alert
      >
    </AdeRowDetail>
    <AdeStepMover :card="card" />
    <AdeStepBlock v-for="b in card.blocks" :key="b.step.id" :block="b" :card="card">
      <template #actions>
        <AdeTaskActionButton v-if="b.state === 'now'" :card="card" />
      </template>
    </AdeStepBlock>
  </div>
</template>
