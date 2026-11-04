<script setup lang="ts">
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { MAX_RETURNS } from '../../board/stepResults';
import type { WorkflowStep } from '../../wire';
import AdeInspectorRow from './AdeInspectorRow.vue';
import AdeStepResults from './AdeStepResults.vue';

const props = defineProps<{ step: WorkflowStep; steps: WorkflowStep[] }>();
const emit = defineEmits<{ patch: [p: Partial<WorkflowStep>] }>();
const minReturns = computed(() => (props.step.kind === 'review' ? 0 : 1));
function onReturns(v: string | number): void {
  const n = Math.trunc(Number(v));
  if (Number.isNaN(n)) return;
  emit('patch', { maxReturns: Math.min(MAX_RETURNS, Math.max(minReturns.value, n)) });
}
const id = `ade-wf-step-max-returns-${props.step.id}`;
</script>

<template>
  <AdeStepResults :step="step" :steps="steps" :fixed="step.kind === 'script'" />
  <AdeInspectorRow :label="t('space.ade.workflows.stepInspector.maxReturns.label')" :for="id">
    <Input
      :id="id"
      type="number"
      :min="minReturns"
      :max="MAX_RETURNS"
      :model-value="step.maxReturns"
      class="w-24"
      data-testid="ade-wf-step-max-returns"
      @update:model-value="onReturns"
    />
    <template #hint>
      <span data-testid="ade-wf-step-max-returns-hint">{{
        t('space.ade.workflows.stepInspector.maxReturns.hint')
      }}</span>
    </template>
  </AdeInspectorRow>
</template>
