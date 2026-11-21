<script setup lang="ts">
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { useDebounceFn } from '@vueuse/core';
import { useGuardedDraft } from '@workbench/behaviour/guardedDraft';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import SaveBar from '@workbench/components/SaveBar.vue';
import SaveConflictAlert from '@workbench/config/SaveConflictAlert.vue';
import CodeTextEditor from '@workbench/editor/CodeTextEditor.vue';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, onBeforeUnmount, ref, useTemplateRef, watch } from 'vue';
import {
  useSaveStepYaml,
  useSaveWorkflowYaml,
  useStepYaml,
  useValidateStepYaml,
  useValidateWorkflowYaml,
  useWorkflowYaml,
} from '../queries';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import type { WorkflowError } from '../wire';

// YAML mode of a workflow or a library step (`kind`): validates 300 ms after a keystroke; Save writes the draft, Discard restores the
// saved text. The draft is the source while it has unsaved edits, so a push never resets it. Save
// sends the hash of the text the draft was edited from; a file changed elsewhere is refused with
// Reload and Overwrite, as in the graph editor. `entryError` is the file's current error from the
// folder listing, shown until a validation of the draft replaces it; `testidPrefix` names the pane,
// editor and message (`ade-wf-yaml`, `ade-step-yaml`).
const props = defineProps<{
  fileName: string;
  kind: 'step' | 'workflow';
  entryError: WorkflowError | null;
  testidPrefix: string;
}>();

const wfUi = useAdeWorkflowsUiStore();
const isStep = props.kind === 'step';
const yaml = isStep ? useStepYaml(() => props.fileName) : useWorkflowYaml(() => props.fileName);
const validate = isStep ? useValidateStepYaml() : useValidateWorkflowYaml();
const save = isStep ? useSaveStepYaml() : useSaveWorkflowYaml();

const checked = ref<{ error: WorkflowError | null } | null>(null);
const guard = useGuardedDraft<string>({
  initial: '',
  source: () =>
    yaml.data.value ? { value: yaml.data.value.yaml, hash: yaml.data.value.hash } : undefined,
  refetch: async () => {
    const d = (await yaml.refetch()).data;
    return d ? { value: d.yaml, hash: d.hash } : undefined;
  },
  write: (text, baseHash) => save.mutateAsync({ fileName: props.fileName, yaml: text, baseHash }),
});
const { draft, dirty } = guard;

const error = computed(() => (checked.value ? checked.value.error : props.entryError));
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

const saveNow = (): Promise<boolean> => guard.save();

function discard(): void {
  guard.discard();
  checked.value = null;
}

async function reload(): Promise<void> {
  await guard.reload();
  checked.value = null;
}

function onInput(v: string | number): void {
  guard.edit(String(v));
  void runValidate();
}

const root = useTemplateRef<HTMLElement>('root');
useShortcut(root, ['form.save'], () => void saveNow());

onBeforeUnmount(() => {
  wfUi.dirty = false;
});
</script>

<template>
  <div class="flex flex-col gap-1.5" ref="root" :data-testid="`${testidPrefix}-pane`">
    <SaveBar
      shortcut="form.save"
      :dirty="dirty"
      :saving="guard.saving.value"
      testid-prefix="ade-wf"
      @save="saveNow"
      @discard="discard"
    >
      <template #start>{{ t('space.ade.workflows.saveBar.applies.hint') }}</template>
    </SaveBar>
    <CodeTextEditor
      :model-value="draft"
      language="yaml"
      :ariaLabel="t('space.ade.workflows.yaml.field.aria')"
      :problem="problem"
      class="h-72"
      :data-testid="testidPrefix"
      @update:model-value="onInput"
      @save="saveNow"
      @submit="saveNow"
    />
    <div v-if="!error" class="text-kira-sm text-ok" :data-testid="`${testidPrefix}-msg`">
      {{ t('space.ade.workflows.yaml.valid.status') }}
    </div>
    <FieldError
      v-else
      :id="`${testidPrefix}-msg-text`"
      class="font-data"
      :data-testid="`${testidPrefix}-msg`"
      >{{
        message
      }}</FieldError
    >
    <SaveConflictAlert
      :error="guard.saveError.value ?? ''"
      :conflict="guard.conflict.value"
      testid-prefix="ade-wf"
      @reload="reload"
      @overwrite="guard.overwrite"
    />
  </div>
</template>
