<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { paneFrame } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import { useGuardedDraft } from '@workbench/behaviour/guardedDraft';
import SaveBar from '@workbench/components/SaveBar.vue';
import SaveConflictAlert from '@workbench/config/SaveConflictAlert.vue';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { storeToRefs } from 'pinia';
import { onBeforeUnmount, useTemplateRef, watch } from 'vue';
import { libraryWorkflow } from '../board/stepLibrary';
import { cloneWorkflow } from '../board/workflowForm';
import { useSaveStep } from '../queries';
import { useAdeWorkflowDraftStore } from '../state/adeWorkflowDraft';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import type { StepEntry, Workflow, WorkflowStep } from '../wire';
import AdeStepInspector from './AdeStepInspector.vue';

// Form mode of a library step: the step inspector over a one-step draft; Save writes the step whole,
// Discard drops the edits. A pushed version never replaces unsaved edits.
const props = defineProps<{ entry: StepEntry }>();
const wfUi = useAdeWorkflowsUiStore();
const draft = useAdeWorkflowDraftStore();
const save = useSaveStep();

function load(e: StepEntry): void {
  draft.open(e.step ? libraryWorkflow(e.step) : null, e.hash);
  if (e.step) draft.select({ stepId: e.step.id });
}
const { draft: draftRef, baseHash, dirty } = storeToRefs(draft);
const guard = useGuardedDraft<Workflow | null>({
  state: { draft: draftRef, baseHash, dirty },
  source: () =>
    props.entry.step
      ? { value: libraryWorkflow(props.entry.step), hash: props.entry.hash }
      : undefined,
  clone: (w) => (w ? cloneWorkflow(w) : null),
  key: (w) => JSON.stringify(w),
  adopt: (s) => {
    const step = s?.value?.steps[0];
    draft.open(s?.value ?? null, s?.hash ?? '');
    if (step) draft.select({ stepId: step.id });
  },
  onSaved: draft.markSaved,
  write: async (wf, hash) => {
    const entry = await save.mutateAsync({
      fileName: props.entry.fileName,
      step: (wf as Workflow).steps[0] as WorkflowStep,
      baseHash: hash,
    });
    if (entry.fileName !== props.entry.fileName) wfUi.stepFile = entry.fileName;
    return entry;
  },
});
load(props.entry);
watch(
  () => draft.dirty,
  (v) => {
    wfUi.dirty = v;
  },
);

const saveNow = (): Promise<boolean> => guard.save();

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
    <SaveBar
      shortcut="form.save"
      :dirty="draft.dirty"
      :saving="guard.saving.value"
      testid-prefix="ade-wf"
      @save="saveNow"
      @discard="guard.discard"
    >
      <template #start>{{ t('space.ade.workflows.saveBar.applies.hint') }}</template>
    </SaveBar>
    <div :class="cn(paneFrame, 'min-h-96 flex-1')" data-testid="ade-step-inspector">
      <AdeStepInspector
        :key="draft.step.id"
        :step="draft.step"
        :steps="draft.draft.steps"
        library
      />
    </div>
    <SaveConflictAlert
      :error="guard.saveError.value ?? ''"
      :conflict="guard.conflict.value"
      testid-prefix="ade-wf"
      @reload="guard.reload"
      @overwrite="guard.overwrite"
    >
      <template #action>
        <Button
          variant="link"
          size="inline"
          data-testid="ade-step-switch-yaml"
          @click="wfUi.stepMode = 'yaml'"
        >
          {{ t('space.ade.workflows.graph.switchYaml.label') }}
        </Button>
      </template>
    </SaveConflictAlert>
  </div>
  <EmptyState
    v-else
    fill
    icon="symbol-event"
    :title="t('space.ade.steps.noValid.body')"
    data-testid="ade-step-no-valid"
  >
    <Button variant="link" size="inline" @click="wfUi.stepMode = 'yaml'">
      {{ t('space.ade.workflows.graph.fixInYaml.label') }}
    </Button>
  </EmptyState>
</template>
