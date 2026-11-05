<script setup lang="ts">
import { FieldDescription } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import type { ReviewStepConfig, WorkflowStep } from '../../wire';
import { useStepScope } from '../stepScope';
import AdeInspectorRow from './AdeInspectorRow.vue';

const props = defineProps<{ step: WorkflowStep; steps: WorkflowStep[] }>();
const library = useStepScope() === 'library';
const emit = defineEmits<{ patch: [p: Partial<WorkflowStep>] }>();
// Earlier steps whose session can take the comments: agent steps with a prompt.
const implementers = computed(() => {
  const at = props.steps.findIndex((s) => s.id === props.step.id);
  return props.steps
    .slice(0, Math.max(at, 0))
    .filter((s) => s.kind === 'agent' && s.smartScript === '');
});
function patchReview(p: Partial<ReviewStepConfig>): void {
  if (props.step.review) emit('patch', { review: { ...props.step.review, ...p } });
}
const id = (k: string): string => `ade-wf-step-${k}-${props.step.id}`;
</script>

<template>
  <div v-if="step.review" class="flex flex-col gap-3" data-testid="ade-wf-step-review">
    <AdeInspectorRow
      :label="t('space.ade.workflows.stepInspector.reviewSendTo.label')"
      :for="id('review-send-to')"
    >
      <NativeSelect
        :id="id('review-send-to')"
        variant="bordered"
        class="w-full"
        :model-value="step.review.sendTo"
        data-testid="ade-wf-step-review-send-to"
        @update:model-value="(v) => patchReview({ sendTo: v === 'new' ? 'new' : 'implementer' })"
      >
        <option value="implementer">{{ t('space.ade.reviewStep.route.implementer.label') }}</option>
        <option value="new">{{ t('space.ade.reviewStep.route.new.label') }}</option>
      </NativeSelect>
    </AdeInspectorRow>
    <AdeInspectorRow
      v-if="!library && step.review.sendTo === 'implementer'"
      :label="t('space.ade.workflows.stepInspector.reviewImplementer.label')"
      :for="id('review-implementer')"
    >
      <NativeSelect
        :id="id('review-implementer')"
        variant="bordered"
        class="w-full"
        :model-value="step.review.implementer"
        data-testid="ade-wf-step-review-implementer"
        @update:model-value="(v) => patchReview({ implementer: String(v) })"
      >
        <option value="">{{ t('space.ade.workflows.stepInspector.reviewNearest.option') }}</option>
        <option v-for="s in implementers" :key="s.id" :value="s.id">{{ s.name }}</option>
      </NativeSelect>
    </AdeInspectorRow>
    <FieldDescription data-testid="ade-wf-review-note">{{
      t('space.ade.workflows.stepInspector.reviewNote.hint')
    }}</FieldDescription>
  </div>
</template>
