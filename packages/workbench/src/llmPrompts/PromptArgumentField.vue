<script setup lang="ts">
import type { LlmPromptView } from '@shared/domain/llmPrompts';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { computed, watch } from 'vue';
import { useLlmPromptDraftsStore } from '../state/llmPromptDrafts';
import { useLlmPromptPreview } from './queries';

// One tool argument: its description is one line, so a Textarea rather than a Monaco instance.
const props = defineProps<{ view: LlmPromptView }>();

const drafts = useLlmPromptDraftsStore();
const text = computed({
  get: () => drafts.textOf(props.view),
  set: (next: string | number) => drafts.setText(props.view, String(next)),
});

const preview = useLlmPromptPreview(() => ({
  id: props.view.id,
  text: drafts.draftFor(props.view.id),
}));
const problems = computed(() => preview.data.value?.errors ?? []);
watch(
  problems,
  (list) =>
    drafts.setProblems(props.view.id, drafts.draftFor(props.view.id) === undefined ? [] : list),
  { immediate: true },
);
</script>

<template>
  <Field data-testid="llm-prompt-arg" :data-id="view.id">
    <div class="flex items-center justify-between gap-1">
      <FieldLabel class="font-data" :for="view.id">{{ view.name }}</FieldLabel>
      <TooltipIconButton
        icon="discard"
        :label="t('common.action.resetToDefault.label')"
        data-testid="llm-prompt-reset"
        :disabled-trigger="text === view.defaultText"
        :disabled="text === view.defaultText"
        @click="drafts.setText(view, view.defaultText)"
      />
    </div>
    <Textarea :id="view.id" v-model="text" :aria-invalid="problems.length > 0" />
    <FieldError v-for="(p, i) in problems" :key="i" data-testid="llm-prompt-error">{{
      p.message
    }}</FieldError>
    <FieldError v-if="preview.error.value">{{ errorText(preview.error.value) }}</FieldError>
  </Field>
</template>
