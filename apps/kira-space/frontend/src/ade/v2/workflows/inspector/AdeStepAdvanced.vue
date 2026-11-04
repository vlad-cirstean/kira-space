<script setup lang="ts">
import { Checkbox } from '@theme/components/ui/checkbox';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';
import { formatTools, parseTools } from '../../board/workflowForm';
import { useAdeWorkflowDraftStore } from '../../state/adeWorkflowDraft';
import type { WorkflowStep } from '../../wire';
import AdeInspectorRow from './AdeInspectorRow.vue';
import AdeStepSettings from './AdeStepSettings.vue';

// Rarely changed: allowed tools, agent settings and skipping the step.
const props = defineProps<{ step: WorkflowStep }>();
const emit = defineEmits<{ patch: [p: Partial<WorkflowStep>] }>();
const draft = useAdeWorkflowDraftStore();
const hasSession = computed(
  () =>
    props.step.kind === 'agent' ||
    props.step.kind === 'review' ||
    (props.step.kind === 'manual' && props.step.session),
);
const id = (k: string): string => `ade-wf-step-${k}-${props.step.id}`;

const toolsText = ref(formatTools(props.step.allowedTools));
watch(
  () => props.step.allowedTools,
  (tools) => {
    if (formatTools(parseTools(toolsText.value)) !== formatTools(tools))
      toolsText.value = formatTools(tools);
  },
);
function onTools(v: string | number): void {
  toolsText.value = String(v);
  emit('patch', { allowedTools: parseTools(toolsText.value) });
}
</script>

<template>
  <AdeInspectorRow
    v-if="step.kind === 'agent' && step.smartScript === ''"
    stack
    :label="t('space.ade.workflows.stepInspector.tools.label')"
    :for="id('tools')"
  >
    <Input
      :id="id('tools')"
      :model-value="toolsText"
      :placeholder="t('space.ade.workflows.stepInspector.tools.example')"
      class="font-data"
      data-testid="ade-wf-step-tools"
      @update:model-value="onTools"
    />
    <template #hint>{{ t('space.ade.workflows.stepInspector.tools.hint') }}</template>
  </AdeInspectorRow>
  <AdeStepSettings
    v-if="hasSession"
    :settings="step.settings"
    :step-id="step.id"
    @patch="(settings) => emit('patch', { settings })"
  />
  <AdeInspectorRow :label="t('space.ade.stepBlock.skipThis.label')" :for="id('skip')">
    <Checkbox
      :id="id('skip')"
      :model-value="step.skip"
      :disabled="!step.skip && !draft.canSkip"
      data-testid="ade-wf-step-skip"
      @update:model-value="(v) => emit('patch', { skip: v === true })"
    />
  </AdeInspectorRow>
</template>
