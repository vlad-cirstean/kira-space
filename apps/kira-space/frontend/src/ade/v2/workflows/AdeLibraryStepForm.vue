<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { onBeforeUnmount, useTemplateRef, watch } from 'vue';
import { libraryWorkflow } from '../board/stepLibrary';
import { useSaveStep } from '../queries';
import { useAdeWorkflowDraftStore } from '../state/adeWorkflowDraft';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import type { StepEntry } from '../wire';
import AdeStepInspector from './AdeStepInspector.vue';
import AdeWorkflowSaveBar from './AdeWorkflowSaveBar.vue';

// Form mode of a library step: the step inspector over a one-step draft; Save writes the step whole,
// Discard drops the edits. A pushed version never replaces unsaved edits.
const props = defineProps<{ entry: StepEntry }>();
const wfUi = useAdeWorkflowsUiStore();
const draft = useAdeWorkflowDraftStore();
const save = useSaveStep();

function load(e: StepEntry, hard: boolean): void {
  const wf = e.step ? libraryWorkflow(e.step) : null;
  if (hard) draft.open(wf, e.hash);
  else draft.push(wf, e.hash);
  if (e.step && hard) draft.select({ stepId: e.step.id });
}
load(props.entry, true);
watch(
  () => props.entry,
  (e) => load(e, false),
);
watch(
  () => draft.dirty,
  (v) => {
    wfUi.dirty = v;
  },
);

async function saveNow(): Promise<void> {
  const step = draft.draft?.steps[0];
  if (!draft.dirty || !step) return;
  const sent = JSON.stringify(draft.draft);
  try {
    const entry = await save.mutateAsync({
      fileName: props.entry.fileName,
      step,
      baseHash: draft.baseHash,
    });
    draft.saved(sent, entry.hash);
  } catch (err) {
    draft.error = errorText(err);
    draft.conflict = (err as { code?: string }).code === 'E_CONFLICT';
  }
}
const discard = (): void => load(props.entry, true);
function overwrite(): void {
  draft.rebase(props.entry.hash);
  void saveNow();
}

const root = useTemplateRef<HTMLElement>('root');
useShortcut(root, ['form.save'], () => void saveNow());

onBeforeUnmount(() => {
  wfUi.dirty = false;
  draft.open(null);
});
</script>

<template>
  <div
    v-if="draft.draft && draft.step"
    ref="root"
    class="flex h-full min-h-0 flex-col gap-3"
    data-testid="ade-step-form"
  >
    <AdeWorkflowSaveBar
      :dirty="draft.dirty"
      :saving="save.isPending.value"
      @save="saveNow"
      @discard="discard"
    />
    <div class="min-h-96 flex-1 rounded-kira bg-bg" data-testid="ade-step-inspector">
      <AdeStepInspector
        :key="draft.step.id"
        :step="draft.step"
        :steps="draft.draft.steps"
        library
      />
    </div>
    <Alert v-if="draft.error" variant="error" data-testid="ade-wf-save-error">
      <AlertDescription>{{ draft.error }}</AlertDescription>
      <AlertAction>
        <template v-if="draft.conflict">
          <Button variant="link" size="inline" data-testid="ade-wf-reload" @click="discard">
            {{ t('common.action.refresh.label') }}
          </Button>
          <Button variant="link" size="inline" data-testid="ade-wf-overwrite" @click="overwrite">
            {{ t('space.ade.workflows.conflict.overwrite.label') }}
          </Button>
        </template>
        <Button
          v-else
          variant="link"
          size="inline"
          data-testid="ade-step-switch-yaml"
          @click="wfUi.stepMode = 'yaml'"
        >
          {{ t('space.ade.workflows.graph.switchYaml.label') }}
        </Button>
      </AlertAction>
    </Alert>
  </div>
  <EmptyState
    v-else
    fill
    :title="t('space.ade.steps.noValid.body')"
    data-testid="ade-step-no-valid"
  >
    <Button variant="link" size="inline" @click="wfUi.stepMode = 'yaml'">
      {{ t('space.ade.workflows.graph.fixInYaml.label') }}
    </Button>
  </EmptyState>
</template>
