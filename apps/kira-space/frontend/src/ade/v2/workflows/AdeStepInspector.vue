<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { Textarea } from '@theme/components/ui/textarea';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref } from 'vue';
import { finishVars, previewTextParts } from '../board/runMessage';
import { overriddenKeys } from '../board/stepLibrary';
import { hasContent, KIND_TONE, kindLabel, STEP_KINDS, withKind } from '../board/workflowForm';
import { useAdePromptPreview, useNewStep } from '../queries';
import { useWorkflows } from '../readQueries';
import { useAdeWorkflowDraftStore } from '../state/adeWorkflowDraft';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import { TONE_TEXT_CLASS } from '../tones';
import type { StepKind, WorkflowStep } from '../wire';
import AdeInspectorRow from './inspector/AdeInspectorRow.vue';
import AdeInspectorSection from './inspector/AdeInspectorSection.vue';
import AdeStepAdvanced from './inspector/AdeStepAdvanced.vue';
import AdeStepAgentFields from './inspector/AdeStepAgentFields.vue';
import AdeStepEndFields from './inspector/AdeStepEndFields.vue';
import AdeStepManualFields from './inspector/AdeStepManualFields.vue';
import AdeStepReviewFields from './inspector/AdeStepReviewFields.vue';
import AdeStepRunFields from './inspector/AdeStepRunFields.vue';
import AdeStepSandbox from './inspector/AdeStepSandbox.vue';
import AdeStepScriptFields from './inspector/AdeStepScriptFields.vue';
import { provideStepScope } from './stepScope';

// The selected step: kind and name on top, then what it does, where and when it runs, how it ends.
const props = withDefaults(
  defineProps<{ step: WorkflowStep; steps: WorkflowStep[]; library?: boolean }>(),
  { library: false },
);
provideStepScope(props.library ? 'library' : 'workflow');
const draft = useAdeWorkflowDraftStore();
const confirm = useConfirmDialogStore();
const wfUi = useAdeWorkflowsUiStore();
const workflows = useWorkflows();
const newStep = useNewStep();
const libError = ref('');
const patch = (p: Partial<WorkflowStep>): void => draft.patchStep(props.step.id, p);

const libEntry = computed(() =>
  props.step.use
    ? (workflows.data.value?.steps ?? []).find((e) => e.step?.id === props.step.use)
    : undefined,
);
const lib = computed(() => libEntry.value?.step ?? null);
const overridden = computed(() => (lib.value ? overriddenKeys(props.step, lib.value) : []));
const extraPrompt = computed(
  () =>
    !!props.step.use &&
    props.step.smartScript === '' &&
    (props.step.kind === 'agent' || (props.step.kind === 'manual' && props.step.session)),
);

async function editInSteps(): Promise<void> {
  const file = libEntry.value?.fileName;
  if (file && (await wfUi.leave())) wfUi.openStep(file);
}
async function saveToLibrary(): Promise<void> {
  libError.value = '';
  try {
    const entry = await newStep.mutateAsync({
      name: props.step.name,
      kind: props.step.kind,
      from: props.step,
    });
    patch({ use: entry.step?.id ?? '' });
  } catch (err) {
    libError.value = errorText(err);
  }
}

const kindItems = computed(() =>
  STEP_KINDS.map((k) => ({
    value: k,
    label: kindLabel(k),
    testid: `ade-wf-step-kind-${k}`,
    disabled: !!props.step.use,
  })),
);
const CLEARED = {
  prompt: 'common.prompt.label',
  command: 'common.field.command.label',
  sessionPrompt: 'space.ade.workflows.stepInspector.prompt.aria',
  smartScript: 'common.smartScript.label',
} as const;
async function setKind(v: string): Promise<void> {
  const kind = v as StepKind;
  if (kind === props.step.kind) return;
  const lost = hasContent(props.step);
  if (lost) {
    const ok = await confirm.confirm({
      title: t('space.ade.workflows.stepInspector.kindChange.title'),
      message: t('space.ade.workflows.stepInspector.kindChange.body', {
        field: t(CLEARED[lost as keyof typeof CLEARED]).toLowerCase(),
      }),
      confirmLabel: t('space.ade.workflows.stepInspector.kindChange.title'),
      tone: 'danger',
    });
    if (!ok) return;
  }
  draft.replaceStep(withKind(props.step, kind));
}

const finishPreview = useAdePromptPreview('headless.finishStep.suffix', () =>
  finishVars(props.step.results),
);
const finish = computed(() => previewTextParts(finishPreview.data.value?.parts ?? []));
const position = computed(() => ({
  n: props.steps.findIndex((x) => x.id === props.step.id) + 1,
  of: props.steps.length,
}));
const id = (k: string): string => `ade-wf-step-${k}-${props.step.id}`;
</script>

<template>
  <div
    class="flex h-full min-h-0 flex-col"
    data-testid="ade-wf-step"
    :data-step-id="step.id"
    :data-kind="step.kind"
  >
    <div class="flex flex-col gap-2 px-3 pt-3 pb-2">
      <div class="flex items-center gap-2 text-kira-sm text-subtle">
        <StatusDot :class="TONE_TEXT_CLASS[KIND_TONE[step.kind]]" color="currentColor" />
        <span v-if="!library" data-testid="ade-wf-step-position">{{
          t('space.ade.workflows.stepInspector.position.label', position)
        }}</span>
      </div>
      <template v-if="!library">
        <div
          v-if="step.use"
          class="flex flex-wrap items-center gap-x-3 gap-y-1 text-kira-sm"
          data-testid="ade-wf-step-library"
        >
          <span class="text-muted-foreground">{{
            t('space.ade.workflows.library.from.label', {
              name: lib?.name ?? step.use,
            })
          }}</span>
          <Button
            variant="link"
            size="inline"
            :disabled="!libEntry"
            data-testid="ade-wf-step-edit-library"
            @click="editInSteps"
          >
            {{ t('space.ade.workflows.library.edit.label') }}
          </Button>
          <Button
            variant="link"
            size="inline"
            data-testid="ade-wf-step-detach"
            @click="draft.detach(step.id)"
          >
            {{ t('space.ade.workflows.library.detach.label') }}
          </Button>
        </div>
        <div
          v-if="step.use && overridden.length"
          class="flex flex-wrap items-center gap-x-3 gap-y-1 text-kira-sm"
          data-testid="ade-wf-step-overrides"
        >
          <span class="text-muted-foreground">{{
            t('space.ade.workflows.library.overridden.label')
          }}</span>
          <span v-for="k in overridden" :key="k" class="flex items-center gap-1">
            <span class="font-data" data-testid="ade-wf-step-overridden">{{ k }}</span>
            <Button
              variant="link"
              size="inline"
              :aria-label="t('space.ade.workflows.library.reset.aria', { field: k })"
              :data-testid="`ade-wf-step-reset-${k}`"
              @click="lib && draft.resetField(step.id, lib, k)"
            >
              {{ t('common.action.reset.label') }}
            </Button>
          </span>
        </div>
        <template v-else-if="!step.use">
          <Button
            variant="dialog"
            size="kira-lg"
            class="self-start"
            :pending="newStep.isPending.value"
            data-testid="ade-wf-step-save-library"
            @click="saveToLibrary"
          >
            {{ t('space.ade.workflows.library.save.label') }}
          </Button>
          <FieldError v-if="libError" data-testid="ade-wf-step-library-error">{{
            libError
          }}</FieldError>
        </template>
      </template>
      <Label :for="id('name')" class="sr-only">{{
        t('space.ade.workflows.stepInspector.name.aria')
      }}</Label>
      <Input
        :id="id('name')"
        :model-value="step.name"
        size="kira-lg"
        appearance="inline"
        class="min-w-0 font-medium"
        data-testid="ade-wf-step-name"
        @update:model-value="(v: string | number) => patch({ name: String(v) })"
      />
      <SecondaryTabs
        :model-value="step.kind"
        :items="kindItems"
        data-testid="ade-wf-step-kind"
        @update:model-value="setKind"
      />
    </div>

    <div class="@container min-h-0 flex-1 overflow-auto px-3 pt-2 pb-3">
      <div class="flex flex-col gap-6">
        <AdeInspectorSection name="what" :title="t('space.ade.workflows.stepInspector.what.title')">
          <AdeStepAgentFields v-if="step.kind === 'agent'" :step="step" @patch="patch" />
          <AdeStepScriptFields v-else-if="step.kind === 'script'" :step="step" @patch="patch" />
          <AdeStepReviewFields
            v-else-if="step.kind === 'review'"
            :step="step"
            :steps="steps"
            @patch="patch"
          />
          <AdeStepManualFields v-else :step="step" @patch="patch" />
          <AdeInspectorRow
            v-if="extraPrompt"
            stack
            :label="t('space.ade.workflows.library.extraPrompt.label')"
            :for="id('extra-prompt')"
          >
            <Textarea
              :id="id('extra-prompt')"
              :model-value="step.promptExtra ?? ''"
              class="min-h-16 w-full resize-y leading-normal"
              data-testid="ade-wf-step-extra-prompt"
              @update:model-value="(v: string | number) => patch({ promptExtra: String(v) })"
            />
            <template #hint>{{ t('space.ade.workflows.library.extraPrompt.hint') }}</template>
          </AdeInspectorRow>
        </AdeInspectorSection>

        <AdeInspectorSection
          v-if="step.kind !== 'manual'"
          name="where"
          :title="t('common.action.run.label')"
        >
          <AdeStepRunFields :step="step" @patch="patch" />
        </AdeInspectorSection>

        <AdeInspectorSection
          v-if="step.kind !== 'manual' || step.session"
          name="sandbox"
          :title="t('space.ade.workflows.stepInspector.sandbox.title')"
        >
          <AdeStepSandbox
            :sandbox="step.sandbox"
            :step-id="step.id"
            :kind="step.kind"
            @patch="(sandbox) => patch({ sandbox })"
          />
        </AdeInspectorSection>

        <AdeInspectorSection name="end" :title="t('space.ade.workflows.stepInspector.end.title')">
          <FieldDescription v-if="step.kind === 'manual'" data-testid="ade-wf-manual-end">{{
            t('space.ade.workflows.stepInspector.endManual.hint')
          }}</FieldDescription>
          <template v-else>
            <AdeStepEndFields :step="step" :steps="steps" @patch="patch" />
            <template v-if="step.kind === 'agent'">
              <FieldDescription>{{
                t('space.ade.workflows.stepInspector.finishNote.hint')
              }}</FieldDescription>
              <Card class="text-kira-sm text-subtle" data-testid="ade-wf-finish-note">
                <VarText :parts="finish" />
              </Card>
            </template>
          </template>
        </AdeInspectorSection>

        <AdeInspectorSection collapsible name="advanced" :title="t('common.advanced.label')">
          <AdeStepAdvanced :step="step" @patch="patch" />
        </AdeInspectorSection>
      </div>
    </div>
  </div>
</template>
