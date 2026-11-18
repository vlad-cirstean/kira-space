<script setup lang="ts">
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import {
  PLACEHOLDER_TOKENS,
  PLACEHOLDER_TOKENS_OUTSIDE,
  usesRepoTokens,
} from '../../board/workflowForm';
import type { WorkflowStep } from '../../wire';

const props = defineProps<{ step: WorkflowStep }>();
const emit = defineEmits<{ patch: [p: Partial<WorkflowStep>] }>();
const id = `ade-wf-step-command-${props.step.id}`;
const outsideVars = computed(() => props.step.outsideRepo && usesRepoTokens(props.step.command));
</script>

<template>
  <Field>
    <FieldLabel :for="id">{{ t('common.field.command.label') }}</FieldLabel>
    <Input
      :id="id"
      :model-value="step.command"
      :placeholder="t('space.ade.workflows.stepInspector.command.example')"
      class="font-data"
      data-testid="ade-wf-step-command"
      @update:model-value="(v: string | number) => emit('patch', { command: String(v) })"
    />
    <FieldDescription>
      {{
        step.outsideRepo
          ? t('space.ade.workflows.stepInspector.commandOutside.hint', {
              tokens: PLACEHOLDER_TOKENS_OUTSIDE,
            })
          : t('space.ade.workflows.stepInspector.command.hint', { tokens: PLACEHOLDER_TOKENS })
      }}
    </FieldDescription>
    <FieldError v-if="outsideVars">
      <span data-testid="ade-wf-step-outside-vars-error">{{
        t('space.ade.workflows.stepInspector.outsideVars.error', {
          repo: '{repo}',
          branch: '{branch}',
        })
      }}</span>
    </FieldError>
  </Field>
</template>
