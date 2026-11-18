<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { ref } from 'vue';
import type { CardModel } from '../plan/usePlanModel';
import AdeTaskActionButton from '../run/AdeTaskActionButton.vue';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import AdeRowDetail from './AdeRowDetail.vue';
import AdeStepBlock from './AdeStepBlock.vue';
import AdeStepMover from './AdeStepMover.vue';
import { useSwitchWorkflow } from './useSwitchWorkflow';

// Task tab Workflow section (SPEC2 section 7): the workflow picker, then one block per step.
const props = defineProps<{ card: CardModel }>();
const wfUi = useAdeWorkflowsUiStore();
const { options, switchTo } = useSwitchWorkflow();
const error = ref('');
const selectKey = ref(0);

async function pick(value: unknown): Promise<void> {
  error.value = '';
  try {
    if (!(await switchTo(props.card.task.id, String(value ?? '')))) selectKey.value++; // remount: the native select shows the declined value otherwise
  } catch (err) {
    error.value = errorText(err);
  }
}

function edit(): void {
  const hit = options.value.find((v) => v.id === props.card.task.workflowId);
  wfUi.openWorkflow(hit?.file ?? null);
}
</script>

<template>
  <div class="@container flex flex-col gap-1" data-testid="ade-workflow-block">
    <Field orientation="responsive">
      <FieldLabel for="ade-workflow-select">{{ t('space.common.workflow.label') }}</FieldLabel>
      <div class="flex min-w-0 items-center gap-1.5">
        <NativeSelect
          id="ade-workflow-select"
          variant="bordered"
          :key="selectKey"
          :model-value="card.task.workflowId"
          data-testid="ade-workflow-select"
          @update:model-value="pick"
        >
          <option v-if="card.task.workflowId === ''" value="">{{
            t('space.ade.workflowBlock.choose.option')
          }}</option>
          <option v-for="v in options" :key="v.id" :value="v.id">{{ v.name }}</option>
        </NativeSelect>
        <Tooltip v-if="card.task.workflowOutdated">
          <TooltipTrigger as-child>
            <Badge variant="warn" data-testid="ade-workflow-outdated">{{
              t('space.ade.workflowBlock.outdated.status')
            }}</Badge>
          </TooltipTrigger>
          <TooltipContent>{{ t('space.ade.workflowBlock.outdatedHint.hint') }}</TooltipContent>
        </Tooltip>
        <Button
          variant="toolbar"
          size="kira"
          class="ml-auto"
          data-testid="ade-edit-workflows"
          @click="edit"
          ><CodiconIcon name="edit" :size="13" />{{ t('space.ade.workflowBlock.edit.label') }}
        </Button>
      </div>
    </Field>
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
