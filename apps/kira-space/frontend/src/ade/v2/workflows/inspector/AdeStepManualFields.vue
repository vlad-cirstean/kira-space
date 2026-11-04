<script setup lang="ts">
import { Checkbox } from '@theme/components/ui/checkbox';
import { FieldDescription } from '@theme/components/ui/field';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { usesRepoTokens } from '../../board/workflowForm';
import type { WorkflowStep } from '../../wire';
import AdeInspectorRow from './AdeInspectorRow.vue';

const props = defineProps<{ step: WorkflowStep }>();
const emit = defineEmits<{ patch: [p: Partial<WorkflowStep>] }>();
const id = (k: string): string => `ade-wf-step-${k}-${props.step.id}`;
const outsideVars = computed(() => props.step.outsideRepo && usesRepoTokens(props.step.prompt));
</script>

<template>
  <FieldDescription data-testid="ade-wf-manual-note">{{
    t('space.ade.workflows.stepInspector.manualNote.hint')
  }}</FieldDescription>
  <AdeInspectorRow
    :label="t('space.ade.workflows.stepInspector.session.label')"
    :for="id('session')"
  >
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
            outsideRepo: v === true ? step.outsideRepo : false,
          })
      "
    />
  </AdeInspectorRow>
  <template v-if="step.session">
    <AdeInspectorRow
      stack
      :label="t('space.ade.workflows.stepInspector.prompt.aria')"
      :for="id('session-prompt')"
    >
      <Textarea
        :id="id('session-prompt')"
        :model-value="step.prompt"
        :placeholder="t('space.ade.workflows.stepInspector.prompt.placeholder')"
        class="min-h-20 w-full resize-y leading-normal"
        data-testid="ade-wf-step-session-prompt"
        @update:model-value="(v: string | number) => emit('patch', { prompt: String(v) })"
      />
      <template v-if="outsideVars" #error>
        <span data-testid="ade-wf-step-outside-vars-error">{{
          t('space.ade.workflows.stepInspector.outsideVars.error', {
            repo: '{repo}',
            branch: '{branch}',
          })
        }}</span>
      </template>
    </AdeInspectorRow>
    <AdeInspectorRow
      :label="t('space.ade.workflows.stepInspector.repoAware.label')"
      :for="id('repo-aware')"
    >
      <Checkbox
        :id="id('repo-aware')"
        :model-value="!step.outsideRepo"
        data-testid="ade-wf-step-repo-aware"
        @update:model-value="(v) => emit('patch', { outsideRepo: v !== true })"
      />
      <template #hint>{{ t('space.ade.workflows.stepInspector.repoAware.hint') }}</template>
    </AdeInspectorRow>
  </template>
</template>
