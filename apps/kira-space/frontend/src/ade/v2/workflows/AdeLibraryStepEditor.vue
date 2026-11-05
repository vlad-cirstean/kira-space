<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Button } from '@theme/components/ui/button';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { useCopy } from '@workbench/behaviour/clipboard';
import { errorText } from '@workbench/bridge/codedError';
import ConfigFileBar from '@workbench/config/ConfigFileBar.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref } from 'vue';
import { control } from '../../../bridge/control';
import { useDeleteStep, useStepYaml } from '../queries';
import { useWorkflows } from '../readQueries';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import type { StepEntry } from '../wire';
import AdeLibraryStepForm from './AdeLibraryStepForm.vue';
import AdeLibraryStepYaml from './AdeLibraryStepYaml.vue';

// One library step: Form or YAML mode, the config file bar, Copy YAML and Delete. A step that
// workflows use cannot be deleted; the workflows are listed instead.
const props = defineProps<{ entry: StepEntry }>();
const wfUi = useAdeWorkflowsUiStore();
const yaml = useStepYaml(() => props.entry.fileName);
const workflows = useWorkflows();
const del = useDeleteStep();
const confirm = useConfirmDialogStore();
const { copy, copied } = useCopy();
const deleteError = ref('');

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
  deleteError.value = '';
  try {
    await del.mutateAsync({ fileName: props.entry.fileName });
    wfUi.dirty = false;
    wfUi.stepFile = null;
  } catch (err) {
    deleteError.value = errorText(err);
  }
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
        size="kira-lg"
        :model-value="wfUi.stepMode"
        :items="modeItems"
        :aria-label="t('space.ade.steps.mode.aria')"
        data-testid="ade-step-mode"
        @update:model-value="setMode"
      />
      <ConfigFileBar
        :path="entry.path"
        :error="entry.error"
        :open="() => control.configFilesOpen({ folder: 'steps', file: entry.fileName })"
        :reveal="() => control.configFilesReveal({ folder: 'steps', file: entry.fileName })"
        :about="() => control.configFilesOpen({ folder: 'steps', file: 'AGENTS.md' })"
      />
      <Button
        variant="dialog"
        size="kira-lg"
        :disabled="!yaml.data.value"
        data-testid="ade-step-copy"
        @click="yaml.data.value && copy(yaml.data.value.yaml)"
      >
        {{
          copied ? t('common.copy.copied.announce') : t('space.ade.workflows.editor.copyYaml.label')
        }}
      </Button>
      <Button
        variant="dialog"
        size="kira-lg"
        class="text-error"
        :disabled="users.length > 0"
        :pending="del.isPending.value"
        data-testid="ade-step-delete"
        @click="remove"
      >
        {{ t('common.action.delete.label') }}
      </Button>
    </div>
    <FieldDescription v-if="users.length" data-testid="ade-step-used-by">{{
      t('space.ade.steps.usedBy.hint', { workflows: users.join(', ') })
    }}</FieldDescription>
    <FieldError v-if="deleteError" data-testid="ade-step-delete-error">{{
      deleteError
    }}</FieldError>
    <AdeLibraryStepYaml v-if="wfUi.stepMode === 'yaml'" :entry="entry" />
    <AdeLibraryStepForm v-else :entry="entry" />
  </div>
</template>
