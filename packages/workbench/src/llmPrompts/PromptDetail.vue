<script setup lang="ts">
import type { LlmPromptPart, LlmPromptView } from '@shared/domain/llmPrompts';
import CodiconIcon from '@theme/CodiconIcon.vue';
import CodeBlock from '@theme/components/CodeBlock.vue';
import DetailHeader from '@theme/components/DetailHeader.vue';
import Disclosure from '@theme/components/Disclosure.vue';
import SecondaryTabs, { type SecondaryTabItem } from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import VarText from '@theme/components/VarText.vue';
import { type MessageKey, t } from '@theme/i18n';
import type { TextPart } from '@theme/varText';
import { errorText } from '@workbench/bridge/codedError';
import MonacoDiff from '@workbench/editor/MonacoDiff.vue';
import { computed, ref, useTemplateRef, watch } from 'vue';
import { useLlmPromptDraftsStore } from '../state/llmPromptDrafts';
import PromptArgumentField from './PromptArgumentField.vue';
import PromptEditor from './PromptEditor.vue';
import PromptTitle from './PromptTitle.vue';
import { llmPromptKindKey, llmPromptVarKey, useLlmPromptPreview } from './queries';

// One entry: copy, editor with its variables, diff against the default, and the resolved preview.
// A tool entry also lists its arguments, each edited on its own.
const props = defineProps<{ view: LlmPromptView; args: readonly LlmPromptView[] }>();

const drafts = useLlmPromptDraftsStore();
const tab = ref<'edit' | 'changes' | 'preview'>('edit');
const editor = useTemplateRef<InstanceType<typeof PromptEditor>>('editor');

const text = computed({
  get: () => drafts.textOf(props.view),
  set: (next: string) => drafts.setText(props.view, next),
});

const preview = useLlmPromptPreview(() => ({
  id: props.view.id,
  text: drafts.draftFor(props.view.id),
}));
const problems = computed(() => preview.data.value?.errors ?? []);
// Only a drafted entry blocks Save: a stored override that went invalid falls back to the default.
watch(
  problems,
  (list) =>
    drafts.setProblems(props.view.id, drafts.draftFor(props.view.id) === undefined ? [] : list),
  { immediate: true },
);

const parts = computed<TextPart[]>(() =>
  (preview.data.value?.parts ?? []).map((p: LlmPromptPart) =>
    p.var === '' ? p.text : { name: p.var, value: p.value },
  ),
);

const description = computed(() =>
  props.view.purposeKey !== ''
    ? t(props.view.purposeKey as MessageKey)
    : t(llmPromptKindKey(props.view.kind) as MessageKey),
);
const tabs = computed<SecondaryTabItem[]>(() => [
  { value: 'edit', label: t('common.action.edit.label'), testid: 'llm-prompt-tab-edit' },
  {
    value: 'changes',
    label: t('space.ade.branchPanel.changes.label'),
    testid: 'llm-prompt-tab-changes',
  },
  { value: 'preview', label: t('llmPrompts.tab.preview.label'), testid: 'llm-prompt-tab-preview' },
]);
const edited = computed(() => text.value !== props.view.defaultText);
const appliesKey = computed(() =>
  props.view.applies === 'newSession'
    ? ('llmPrompts.applies.newSession.hint' as const)
    : ('llmPrompts.applies.nextCall.hint' as const),
);
const literal = computed(() => props.view.vars.length === 0);
</script>

<template>
  <div class="flex min-w-0 flex-col gap-3" data-testid="llm-prompt-detail" :data-id="view.id">
    <DetailHeader flush>
      <template #title><PromptTitle :view="view" /></template>
      <template #actions>
        <Button
          variant="toolbar"
          size="kira"
          data-testid="llm-prompt-reset"
          :disabled="!edited"
          @click="drafts.setText(view, view.defaultText)"
          ><CodiconIcon name="discard" :size="13" />{{ t('common.action.resetToDefault.label') }}
        </Button>
      </template>
      <template #description>
        <p class="m-0">{{ description }}</p>
        <p v-if="view.usedByKey !== ''" class="m-0">{{ t(view.usedByKey as MessageKey) }}</p>
        <p class="m-0">{{ t(appliesKey) }}</p>
      </template>
    </DetailHeader>

    <Alert v-if="view.invalid !== ''" variant="warn" data-testid="llm-prompt-invalid">
      <AlertDescription>{{
        t('llmPrompts.invalid.body', { cause: view.invalid })
      }}</AlertDescription>
    </Alert>
    <Alert v-if="view.defaultChanged" variant="info" data-testid="llm-prompt-default-changed">
      <AlertDescription>{{ t('llmPrompts.defaultChanged.body') }}</AlertDescription>
    </Alert>

    <SecondaryTabs v-model="tab" :items="tabs" />

    <div v-if="tab === 'edit'" class="flex flex-col gap-3">
      <div class="h-64" data-testid="llm-prompt-editor">
        <PromptEditor ref="editor" v-model="text" :problems="problems" />
      </div>
      <FieldError v-for="(p, i) in problems" :key="i" data-testid="llm-prompt-error">{{
        p.message
      }}</FieldError>
      <FieldError v-if="preview.error.value">{{ errorText(preview.error.value) }}</FieldError>

      <section v-if="!literal" class="flex flex-col gap-1">
        <FieldDescription>{{ t('llmPrompts.vars.hint') }}</FieldDescription>
        <ul class="m-0 flex list-none flex-col gap-1 p-0">
          <li v-for="v in view.vars" :key="v.name" class="flex items-baseline gap-2">
            <Button
              variant="toolbar"
              size="status"
              class="font-data"
              data-testid="llm-prompt-var"
              :data-name="v.name"
              @mousedown.prevent
              @click="editor?.insert(`{{.${v.name}}}`)"
            >
              {{ v.name }}
            </Button>
            <Badge v-if="v.required" variant="info">{{ t('common.required.label') }}</Badge>
            <FieldDescription>{{ t(llmPromptVarKey(v.name) as MessageKey) }}</FieldDescription>
          </li>
        </ul>
      </section>

      <section v-if="view.fragments.length > 0" class="flex flex-col gap-1">
        <FieldDescription>{{ t('llmPrompts.fragments.hint') }}</FieldDescription>
        <ul class="m-0 flex list-none flex-wrap gap-1 p-0">
          <li v-for="id in view.fragments" :key="id">
            <Button
              variant="toolbar"
              size="status"
              class="font-data"
              data-testid="llm-prompt-fragment"
              @click='editor?.insert(`{{template "${id}"}}`)'
            >
              {{ id }}
            </Button>
          </li>
        </ul>
      </section>

      <Disclosure v-if="view.schema !== ''" :label="t('llmPrompts.schema.label')">
        <FieldDescription>{{ t('llmPrompts.schema.hint') }}</FieldDescription>
        <CodeBlock data-testid="llm-prompt-schema" class="max-h-64 overflow-auto">{{
          view.schema
        }}</CodeBlock>
      </Disclosure>

      <section v-if="args.length > 0" class="flex flex-col gap-3">
        <FieldDescription>{{ t('llmPrompts.args.hint') }}</FieldDescription>
        <PromptArgumentField v-for="a in args" :key="a.id" :view="a" />
      </section>
    </div>

    <div v-else-if="tab === 'changes'" class="h-80" data-testid="llm-prompt-diff">
      <MonacoDiff :original="view.defaultText" :modified="text" />
    </div>

    <CodeBlock v-else data-testid="llm-prompt-preview" class="text-kira-md">
      <FieldError v-if="problems.length > 0">{{ problems[0].message }}</FieldError>
      <VarText v-else :parts="parts" />
    </CodeBlock>
  </div>
</template>
