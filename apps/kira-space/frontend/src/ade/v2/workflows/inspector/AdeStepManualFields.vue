<script setup lang="ts">
import { Checkbox } from '@theme/components/ui/checkbox';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { defaultSandbox } from '../../board/stepSandbox';
import { usesRepoTokens } from '../../board/workflowForm';
import type { WorkflowStep } from '../../wire';

const props = defineProps<{ step: WorkflowStep }>();
const emit = defineEmits<{ patch: [p: Partial<WorkflowStep>] }>();
const id = (k: string): string => `ade-wf-step-${k}-${props.step.id}`;
const outsideVars = computed(() => props.step.outsideRepo && usesRepoTokens(props.step.prompt));
</script>

<template>
  <FieldDescription data-testid="ade-wf-manual-note">{{
    t('space.ade.workflows.stepInspector.manualNote.hint')
  }}</FieldDescription>
  <Field orientation="responsive">
    <FieldLabel :for="id('session')">{{
      t('space.ade.workflows.stepInspector.session.label')
    }}</FieldLabel>
    <Checkbox
      :id="id('session')"
      :model-value="step.session"
      data-testid="ade-wf-step-session"
      @update:model-value="
        (v) =>
          emit('patch', {
            session: v === true,
            prompt: v === true ? step.prompt : '',
            settings: v === true ? step.settings : {},
            dbMcp: v === true,
            outsideRepo: v === true ? step.outsideRepo : false,
            sandbox: v === true ? step.sandbox : defaultSandbox(),
          })
      "
    />
  </Field>
  <template v-if="step.session">
    <Field>
      <FieldLabel :for="id('session-prompt')">{{
        t('space.ade.workflows.stepInspector.prompt.aria')
      }}</FieldLabel>
      <Textarea
        class="min-h-24 max-h-90 resize-y"
        :id="id('session-prompt')"
        :model-value="step.prompt"
        :placeholder="t('space.ade.workflows.stepInspector.prompt.placeholder')"
        data-testid="ade-wf-step-session-prompt"
        @update:model-value="(v: string | number) => emit('patch', { prompt: String(v) })"
      />
      <FieldError v-if="outsideVars">
        <span data-testid="ade-wf-step-outside-vars-error">{{
          t('space.ade.workflows.stepInspector.outsideVars.error', {
            repo: '{repo}',
            branch: '{branch}',
          })
        }}</span>
      </FieldError>
    </Field>
    <Field orientation="responsive">
      <FieldLabel :for="id('repo-aware')">{{
        t('space.ade.workflows.stepInspector.repoAware.label')
      }}</FieldLabel>
      <Checkbox
        :id="id('repo-aware')"
        :model-value="!step.outsideRepo"
        data-testid="ade-wf-step-repo-aware"
        @update:model-value="(v) => emit('patch', { outsideRepo: v !== true })"
      />
      <FieldDescription>{{
        t('space.ade.workflows.stepInspector.repoAware.hint')
      }}</FieldDescription>
    </Field>
  </template>
</template>
