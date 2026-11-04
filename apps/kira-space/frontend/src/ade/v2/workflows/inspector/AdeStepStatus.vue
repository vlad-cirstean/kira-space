<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useBoard } from '../../queries';
import { useAdeWorkflowsUiStore } from '../../state/adeWorkflowsUi';
import type { WorkflowStep } from '../../wire';
import AdeInspectorRow from './AdeInspectorRow.vue';

// The task status while a step runs: the default of its kind, or one of the user's statuses.
const props = defineProps<{ step: WorkflowStep }>();
const emit = defineEmits<{ patch: [p: Partial<WorkflowStep>] }>();
const board = useBoard();
const wfUi = useAdeWorkflowsUiStore();
const statuses = computed(() => board.data.value?.statuses ?? []);
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
const statusMissing = computed(
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
const id = (k: string): string => `ade-wf-step-${k}-${props.step.id}`;
</script>

<template>
  <AdeInspectorRow :label="t('space.ade.workflows.stepInspector.status.label')" :for="id('status')">
    <div class="flex items-center gap-2">
      <NativeSelect
        :id="id('status')"
        variant="bordered"
        class="min-w-0 flex-1"
        :model-value="step.status"
        data-testid="ade-wf-step-status"
        @update:model-value="(v) => emit('patch', { status: String(v) as WorkflowStep['status'] })"
      >
        <option value="">{{ defaultLabel }}</option>
        <option v-if="statusMissing" :value="step.status">{{ missingLabel }}</option>
        <option v-for="s in statuses" :key="s.id" :value="s.id">{{ s.name }}</option>
      </NativeSelect>
      <Button
        variant="link"
        size="kira"
        class="shrink-0"
        data-testid="ade-step-edit-statuses"
        @click="wfUi.openStatuses()"
        >{{
          t('space.ade.workflows.stepInspector.editStatuses.label')
        }}</Button
      >
    </div>
  </AdeInspectorRow>
</template>
