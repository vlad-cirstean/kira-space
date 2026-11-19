<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { useMutationAction } from '@workbench/behaviour/asyncAction';
import { useCopy } from '@workbench/behaviour/clipboard';
import ConfigFileProblem from '@workbench/config/ConfigFileProblem.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed } from 'vue';
import { useDeleteStep, useStepYaml } from '../queries';
import { useWorkflows } from '../readQueries';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import type { StepEntry } from '../wire';
import AdeLibraryStepForm from './AdeLibraryStepForm.vue';
import AdeYamlPane from './AdeYamlPane.vue';

// One library step: Form or YAML mode, the config file bar in YAML mode (the file's problem in Form mode), Copy YAML and Delete. A step that
// workflows use cannot be deleted; the workflows are listed instead.
const props = defineProps<{ entry: StepEntry }>();
const wfUi = useAdeWorkflowsUiStore();
const yaml = useStepYaml(() => props.entry.fileName);
const workflows = useWorkflows();
const del = useDeleteStep();
const confirm = useConfirmDialogStore();
const { copy, copied } = useCopy();
const deleting = useMutationAction(del);
const deleteError = deleting.error;

const modeItems = [
  { value: 'form', label: t('common.form.label'), testid: 'ade-step-mode-form' },
  { value: 'yaml', label: t('common.yaml.label'), testid: 'ade-step-mode-yaml' },
];
const users = computed(() => {
  const named = new Map((workflows.data.value?.workflows ?? []).map((e) => [e.workflow?.id, e]));
  return props.entry.usedBy.map((id) => named.get(id)?.workflow?.name ?? id);
});

async function setMode(v: unknown): Promise<void> {
  if ((v !== 'form' && v !== 'yaml') || v === wfUi.stepMode) return;
  if (await wfUi.leave()) wfUi.stepMode = v;
}

async function remove(): Promise<void> {
  const ok = await confirm.confirm({
    title: t('space.ade.steps.delete.title'),
    message: t('space.ade.steps.delete.body', {
      name: props.entry.step?.name ?? props.entry.fileName,
    }),
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
  });
  if (!ok) return;
  if (!(await deleting.run({ fileName: props.entry.fileName }))) return;
  wfUi.dirty = false;
  wfUi.stepFile = null;
}
</script>

<template>
  <div
    class="flex flex-col gap-3"
    :class="wfUi.stepMode === 'yaml' ? 'max-w-4xl' : 'h-full min-h-0'"
    data-testid="ade-step-editor"
  >
    <div class="flex flex-wrap items-center gap-3">
      <SecondaryTabs
        variant="segmented"
        :model-value="wfUi.stepMode"
        :items="modeItems"
        :aria-label="t('space.ade.steps.mode.aria')"
        data-testid="ade-step-mode"
        @update:model-value="setMode"
      />
      <Button
        variant="toolbar"
        size="kira"
        class="ml-auto"
        :disabled="!yaml.data.value"
        data-testid="ade-step-copy"
        @click="yaml.data.value && copy(yaml.data.value.yaml)"
        ><CodiconIcon name="copy" :size="13" />
        {{
          copied ? t('common.copy.copied.announce') : t('space.ade.workflows.editor.copyYaml.label')
        }}
      </Button>
      <Button
        variant="toolbar"
        size="kira"
        :disabled="users.length > 0"
        :pending="del.isPending.value"
        data-testid="ade-step-delete"
        @click="remove"
        data-tone="error"
        ><CodiconIcon name="trash" :size="13" />{{ t('common.action.delete.label') }}
      </Button>
    </div>
    <FieldDescription v-if="users.length" data-testid="ade-step-used-by">{{
      t('space.ade.steps.usedBy.hint', { workflows: users.join(', ') })
    }}</FieldDescription>
    <Alert variant="error" v-if="deleteError" data-testid="ade-step-delete-error"
      ><AlertDescription>{{ deleteError }}</AlertDescription></Alert
    >
    <ConfigFileProblem
      :error="entry.error"
      :show-yaml="wfUi.stepMode !== 'yaml' ? () => void setMode('yaml') : undefined"
      class="shrink-0"
    />
    <AdeYamlPane
      v-if="wfUi.stepMode === 'yaml'"
      kind="step"
      :file-name="entry.fileName"
      :entry-error="entry.error"
      testid-prefix="ade-step-yaml"
    />
    <AdeLibraryStepForm v-else :entry="entry" />
  </div>
</template>
