<script setup lang="ts">
import { Checkbox } from '@theme/components/ui/checkbox';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';
import { formatTools, parseTools } from '../../board/workflowForm';
import { useAdeWorkflowDraftStore } from '../../state/adeWorkflowDraft';
import type { WorkflowStep } from '../../wire';
import { useStepScope } from '../stepScope';
import AdeInspectorRow from './AdeInspectorRow.vue';
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
  <AdeInspectorRow
    v-if="hasDbMcp"
    :label="t('space.ade.workflows.stepInspector.dbMcp.label')"
    :for="id('db-mcp')"
  >
    <Checkbox
      :id="id('db-mcp')"
      :model-value="step.dbMcp"
      data-testid="ade-wf-step-db-mcp"
      @update:model-value="(v) => emit('patch', { dbMcp: v === true })"
    />
    <template #hint>{{ t('space.ade.workflows.stepInspector.dbMcp.hint') }}</template>
  </AdeInspectorRow>
  <AdeStepSettings
    v-if="hasSession"
    :settings="step.settings"
    :step-id="step.id"
    @patch="(settings) => emit('patch', { settings })"
  />
  <AdeInspectorRow
    v-if="!library"
    :label="t('space.ade.stepBlock.skipThis.label')"
    :for="id('skip')"
  >
    <Checkbox
      :id="id('skip')"
      :model-value="step.skip"
      :disabled="!step.skip && !draft.canSkip"
      data-testid="ade-wf-step-skip"
      @update:model-value="(v) => emit('patch', { skip: v === true })"
    />
  </AdeInspectorRow>
</template>
