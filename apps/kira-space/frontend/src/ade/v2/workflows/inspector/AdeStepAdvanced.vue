<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { useAgentList } from '@workbench/agent/agentInfo';
import { computed, ref, watch } from 'vue';
import { formatTools, parseTools } from '../../board/workflowForm';
import { useBoard } from '../../queries';
import { useAdeWorkflowDraftStore } from '../../state/adeWorkflowDraft';
import { useAdeWorkflowsUiStore } from '../../state/adeWorkflowsUi';
import type { WorkflowStep } from '../../wire';
import AdeStepSandbox from './AdeStepSandbox.vue';
import AdeStepSettings from './AdeStepSettings.vue';

const props = defineProps<{ step: WorkflowStep }>();
const emit = defineEmits<{ patch: [p: Partial<WorkflowStep>] }>();
const draft = useAdeWorkflowDraftStore();
const board = useBoard();
const { list: agents } = useAgentList();
const statuses = computed(() => board.data.value?.statuses ?? []);
const wfUi = useAdeWorkflowsUiStore();
// A step with no status uses the default of its kind; a deleted status falls back to it too.
const defaultName = computed(() => {
  const d = board.data.value?.statusDefaults;
  const id = props.step.kind === 'review' ? d?.review : d?.step;
  return statuses.value.find((s) => s.id === id)?.name ?? '';
});
const defaultLabel = computed(() =>
  defaultName.value
    ? t('space.ade.workflows.stepInspector.statusDefault.label', { status: defaultName.value })
    : t('space.ade.workflows.stepInspector.statusDefaultKeep.label'),
);
const missing = computed(
  () =>
    props.step.status !== '' &&
    statuses.value.length > 0 &&
    !statuses.value.some((s) => s.id === props.step.status),
);
const missingLabel = computed(() =>
  t('space.ade.workflows.stepInspector.statusMissing.label', {
    id: props.step.status,
    status: defaultName.value || t('space.ade.workflows.stepInspector.statusDefaultKeep.label'),
  }),
);
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
  <div
    v-if="step.kind !== 'manual'"
    class="flex items-center gap-2 text-kira-sm text-muted-foreground"
  >
    <Label :for="id('timeout')">{{ t('common.field.timeout.label') }}</Label>
    <Input
      :id="id('timeout')"
      :model-value="step.timeout"
      size="kira"
      class="w-16"
      data-testid="ade-wf-step-timeout"
      @update:model-value="(v: string | number) => emit('patch', { timeout: String(v) })"
    />
  </div>
  <div v-if="step.kind === 'agent' && step.smartScript === ''" class="flex flex-col gap-1">
    <Label :for="id('tools')" class="text-kira-sm text-muted-foreground"
      >{{ t('space.ade.workflows.stepInspector.tools.label') }}
      <span class="text-subtle">{{
        t('space.ade.workflows.stepInspector.tools.hint')
      }}</span></Label
    >
    <Input
      :id="id('tools')"
      :model-value="toolsText"
      :placeholder="t('space.ade.workflows.stepInspector.tools.example')"
      size="kira"
      class="font-data"
      data-testid="ade-wf-step-tools"
      @update:model-value="onTools"
    />
  </div>
  <div class="flex items-center gap-2 text-kira-sm text-muted-foreground">
    <Label :for="id('status')">{{ t('space.ade.workflows.stepInspector.status.label') }}</Label>
    <NativeSelect
      :id="id('status')"
      variant="bordered"
      :model-value="step.status"
      data-testid="ade-wf-step-status"
      @update:model-value="(v) => emit('patch', { status: String(v) as WorkflowStep['status'] })"
    >
      <option value="">{{ defaultLabel }}</option>
      <option v-if="missing" :value="step.status">{{ missingLabel }}</option>
      <option v-for="s in statuses" :key="s.id" :value="s.id">{{ s.name }}</option>
    </NativeSelect>
    <Button
      variant="link"
      size="kira"
      data-testid="ade-step-edit-statuses"
      @click="wfUi.openStatuses()"
      >{{
        t('space.ade.workflows.stepInspector.editStatuses.label')
      }}</Button
    >
  </div>
  <div class="flex items-center gap-2">
    <Checkbox
      :id="id('skip')"
      :model-value="step.skip"
      :disabled="!step.skip && !draft.canSkip"
      data-testid="ade-wf-step-skip"
      @update:model-value="(v) => emit('patch', { skip: v === true })"
    />
    <Label :for="id('skip')" class="text-kira-sm">{{
      t('space.ade.stepBlock.skipThis.label')
    }}</Label>
  </div>
  <AdeStepSandbox
    v-if="step.kind === 'agent' || step.kind === 'review'"
    :sandbox="step.sandbox"
    :step-id="step.id"
    @patch="(sandbox) => emit('patch', { sandbox })"
  />
  <div v-if="hasSession" class="flex items-center gap-2 text-kira-sm text-muted-foreground">
    <Label :for="id('agent')">{{ t('memory.common.origin.agent.label') }}</Label>
    <NativeSelect
      :id="id('agent')"
      variant="bordered"
      :model-value="step.agent"
      data-testid="ade-wf-step-agent"
      @update:model-value="(v) => emit('patch', { agent: String(v) })"
    >
      <option value="">{{ t('space.ade.workflows.stepInspector.agentDefault.label') }}</option>
      <option v-for="e in agents" :key="e.id" :value="e.id">{{ e.productLabel }}</option>
    </NativeSelect>
  </div>
  <AdeStepSettings
    v-if="hasSession"
    :settings="step.settings"
    :step-id="step.id"
    @patch="(settings) => emit('patch', { settings })"
  />
</template>
