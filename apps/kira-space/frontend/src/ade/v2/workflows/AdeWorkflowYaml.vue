<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { useDebounceFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import CodeTextEditor from '@workbench/editor/CodeTextEditor.vue';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, onBeforeUnmount, ref, useTemplateRef, watch } from 'vue';
import { useSaveWorkflowYaml, useValidateWorkflowYaml, useWorkflowYaml } from '../queries';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import type { WorkflowEntry, WorkflowError } from '../wire';
import AdeWorkflowSaveBar from './AdeWorkflowSaveBar.vue';

// YAML mode: validates 300 ms after a keystroke; Save writes the draft, Discard restores the
// saved text. The draft is the source while it has unsaved edits, so a push never resets it. Save
// sends the hash of the text the draft was edited from; a file changed elsewhere is refused with
// Reload and Overwrite, as in the graph editor.
const props = defineProps<{ entry: WorkflowEntry }>();

const wfUi = useAdeWorkflowsUiStore();
const yaml = useWorkflowYaml(() => props.entry.fileName);
const validate = useValidateWorkflowYaml();
const save = useSaveWorkflowYaml();

const draft = ref('');
const dirty = ref(false);
const saveError = ref('');
const baseHash = ref('');
const conflict = ref(false);
const checked = ref<{ error: WorkflowError | null } | null>(null);

watch(
  () => yaml.data.value,
  (data) => {
    if (!data || dirty.value) return;
    draft.value = data.yaml;
    baseHash.value = data.hash;
  },
  { immediate: true },
);

const error = computed(() => (checked.value ? checked.value.error : props.entry.error));
const message = computed(() => {
  const e = error.value;
  if (!e) return '';
  const text = e.message.replace(/^✕\s*/, '');
  return e.line > 0
    ? t('space.ade.workflows.yaml.lineError.error', { line: e.line, text })
    : t('space.ade.workflows.yaml.fileError.error', { text });
});

const problem = computed(() => {
  const e = error.value;
  return e && e.line > 0 ? { line: e.line, message: message.value } : null;
});

const runValidate = useDebounceFn(async () => {
  const text = draft.value;
  try {
    const res = await validate.mutateAsync({ yaml: text });
    if (draft.value === text) checked.value = { error: res.error };
  } catch (err) {
    if (draft.value === text) {
      checked.value = { error: { line: 0, message: errorText(err) } };
    }
  }
}, TIMING.preview);

watch(dirty, (v) => {
  wfUi.dirty = v;
});

async function saveNow(): Promise<void> {
  if (!dirty.value) return;
  const text = draft.value;
  try {
    const entry = await save.mutateAsync({
      fileName: props.entry.fileName,
      yaml: text,
      baseHash: baseHash.value,
    });
    saveError.value = '';
    conflict.value = false;
    baseHash.value = entry.hash;
    if (draft.value === text) dirty.value = false;
  } catch (err) {
    saveError.value = errorText(err);
    conflict.value = (err as { code?: string }).code === 'E_CONFLICT';
  }
}

function resetTo(data: { yaml: string; hash: string } | undefined): void {
  draft.value = data?.yaml ?? '';
  baseHash.value = data?.hash ?? '';
  dirty.value = false;
  saveError.value = '';
  conflict.value = false;
  checked.value = null;
}

function discard(): void {
  resetTo(yaml.data.value);
}

async function reload(): Promise<void> {
  resetTo((await yaml.refetch()).data);
}

async function overwrite(): Promise<void> {
  const fresh = (await yaml.refetch()).data;
  if (!fresh) return;
  baseHash.value = fresh.hash;
  conflict.value = false;
  await saveNow();
}

function onInput(v: string | number): void {
  draft.value = String(v);
  dirty.value = true;
  void runValidate();
}

const root = useTemplateRef<HTMLElement>('root');
useShortcut(root, ['form.save'], () => void saveNow());

onBeforeUnmount(() => {
  wfUi.dirty = false;
});
</script>

<template>
  <div class="flex flex-col gap-1.5" ref="root" data-testid="ade-wf-yaml-pane">
    <AdeWorkflowSaveBar
      :dirty="dirty"
      :saving="save.isPending.value"
      @save="saveNow"
      @discard="discard"
    />
    <CodeTextEditor
      :model-value="draft"
      language="yaml"
      :ariaLabel="t('space.ade.workflows.yaml.field.aria')"
      :problem="problem"
      class="h-72"
      data-testid="ade-wf-yaml"
      @update:model-value="onInput"
      @save="saveNow"
      @submit="saveNow"
    />
    <div v-if="!error" class="text-kira-sm text-ok" data-testid="ade-wf-yaml-msg">
      {{ t('space.ade.workflows.yaml.valid.status') }}
    </div>
    <FieldError v-else id="ade-wf-yaml-msg-text" class="font-data" data-testid="ade-wf-yaml-msg">{{
      message
    }}</FieldError>
    <Alert v-if="saveError" variant="error" data-testid="ade-wf-save-error">
      <AlertDescription>{{ saveError }}</AlertDescription>
      <AlertAction v-if="conflict">
        <Button variant="link" size="inline" data-testid="ade-wf-reload" @click="reload">
          {{ t('common.action.refresh.label') }}
        </Button>
        <Button variant="link" size="inline" data-testid="ade-wf-overwrite" @click="overwrite">
          {{ t('space.ade.workflows.conflict.overwrite.label') }}
        </Button>
      </AlertAction>
    </Alert>
  </div>
</template>
