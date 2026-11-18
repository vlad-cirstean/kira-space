<script setup lang="ts">
import { Checkbox } from '@theme/components/ui/checkbox';
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';
import { formatTools, parseTools } from '../../board/workflowForm';
import { useAdeWorkflowDraftStore } from '../../state/adeWorkflowDraft';
import type { WorkflowStep } from '../../wire';
import { useStepScope } from '../stepScope';
import AdeStepAgentPick from './AdeStepAgentPick.vue';
import AdeStepSettings from './AdeStepSettings.vue';
import AdeStepStatus from './AdeStepStatus.vue';

// Rarely changed: allowed tools, Kira Studio tools, agent settings and skipping the step. A manual step keeps its status and agent here.
const props = defineProps<{ step: WorkflowStep }>();
const emit = defineEmits<{ patch: [p: Partial<WorkflowStep>] }>();
const draft = useAdeWorkflowDraftStore();
const library = useStepScope() === 'library';
const hasSession = computed(
  () =>
    props.step.kind === 'agent' ||
    props.step.kind === 'review' ||
    (props.step.kind === 'manual' && props.step.session),
);
const hasDbMcp = computed(
  () =>
    (props.step.kind === 'agent' && props.step.smartScript === '') ||
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
  <template v-if="step.kind === 'manual'">
    <AdeStepStatus :step="step" @patch="(p) => emit('patch', p)" />
    <AdeStepAgentPick v-if="step.session" :step="step" @patch="(p) => emit('patch', p)" />
  </template>
  <Field v-if="step.kind === 'agent' && step.smartScript === ''">
    <FieldLabel :for="id('tools')">{{
      t('space.ade.workflows.stepInspector.tools.label')
    }}</FieldLabel>
    <Input
      :id="id('tools')"
      :model-value="toolsText"
      :placeholder="t('space.ade.workflows.stepInspector.tools.example')"
      class="font-data"
      data-testid="ade-wf-step-tools"
      @update:model-value="onTools"
    />
    <FieldDescription>{{ t('space.ade.workflows.stepInspector.tools.hint') }}</FieldDescription>
  </Field>
  <Field v-if="hasDbMcp" orientation="responsive">
    <FieldLabel :for="id('db-mcp')">{{
      t('space.ade.workflows.stepInspector.dbMcp.label')
    }}</FieldLabel>
    <Checkbox
      :id="id('db-mcp')"
      :model-value="step.dbMcp"
      data-testid="ade-wf-step-db-mcp"
      @update:model-value="(v) => emit('patch', { dbMcp: v === true })"
    />
    <FieldDescription>{{ t('space.ade.workflows.stepInspector.dbMcp.hint') }}</FieldDescription>
  </Field>
  <AdeStepSettings
    v-if="hasSession"
    :settings="step.settings"
    :step-id="step.id"
    @patch="(settings) => emit('patch', { settings })"
  />
  <Field v-if="!library" orientation="responsive">
    <FieldLabel :for="id('skip')">{{ t('space.ade.stepBlock.skipThis.label') }}</FieldLabel>
    <Checkbox
      :id="id('skip')"
      :model-value="step.skip"
      :disabled="!step.skip && !draft.canSkip"
      data-testid="ade-wf-step-skip"
      @update:model-value="(v) => emit('patch', { skip: v === true })"
    />
  </Field>
</template>
