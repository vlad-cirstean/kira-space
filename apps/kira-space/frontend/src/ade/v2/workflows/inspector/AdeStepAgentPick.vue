<script setup lang="ts">
import { Field, FieldLabel } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { useAgentList } from '@workbench/agent/agentInfo';
import type { WorkflowStep } from '../../wire';

// Which agent runs the step; empty uses the default provider.
const props = defineProps<{ step: WorkflowStep }>();
const emit = defineEmits<{ patch: [p: Partial<WorkflowStep>] }>();
const { list: agents } = useAgentList();
const id = (k: string): string => `ade-wf-step-${k}-${props.step.id}`;
</script>

<template>
  <Field orientation="responsive">
    <FieldLabel :for="id('agent')">{{ t('memory.common.origin.agent.label') }}</FieldLabel>
    <NativeSelect
      :id="id('agent')"
      variant="bordered"
      class="w-full"
      :model-value="step.agent"
      data-testid="ade-wf-step-agent"
      @update:model-value="(v) => emit('patch', { agent: String(v) })"
    >
      <option value="">{{ t('space.ade.workflows.stepInspector.agentDefault.label') }}</option>
      <option v-for="e in agents" :key="e.id" :value="e.id">{{ e.productLabel }}</option>
    </NativeSelect>
  </Field>
</template>
