<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { Textarea } from '@theme/components/ui/textarea';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import SmartBadge from '@workbench/automations/smart/SmartBadge.vue';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed, ref, watch } from 'vue';
import { useCustomScriptsStore } from '../../../state/customScripts';
import AdeRepoTag from '../AdeRepoTag.vue';
import { smartBodyParts } from '../automation/smartStepBody';
import {
  branchSlug,
  editablePart,
  finishVars,
  previewPlain,
  runMessageVars,
} from '../board/runMessage';
import AdeSetupProgress from '../panel/AdeSetupProgress.vue';
import { usePlanModel } from '../plan/usePlanModel';
import { useAdePromptPreview, useStartRun } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { ACTION_CLASS } from '../tones';

// Agent step Run dialog (SPEC2 section 5, R24): per-branch names for branches not created yet, the
// editable message and the read-only finish_step suffix the server always appends.
const ui = useAdeBoardUiStore();
const { model } = usePlanModel();
const start = useStartRun();
const card = computed(() => (ui.runTaskId ? (model.value?.cardFor(ui.runTaskId) ?? null) : null));
const open = computed(() => card.value !== null);
// What the server adds to the prompt besides the editable message: the previous stage's handover and the task notes.
const handoverIn = computed(() => {
  const h = card.value?.task.handoverIn;
  return h?.body && h.toStepId === card.value?.progress.step?.id ? h : null;
});
const taskNotes = computed(() => card.value?.task.notes ?? '');
const close = (): void => {
  ui.runTaskId = null;
};
const step = computed(() => card.value?.progress.step ?? null);
// No mine branch: the step runs in the task folder and can pick repos itself.
const noRepo = computed(() => !card.value?.rows.some((r) => r.branch.kind === 'mine'));
// The workflow's definition of the step being run: its prompt and smart script params.
const def = computed(
  () => card.value?.workflow?.steps.find((x) => x.id === step.value?.id) ?? null,
);
// Position among the workflow's automatic steps, as `ade.step.prompt` counts them.
const numbering = computed(() => {
  const auto = (card.value?.workflow?.steps ?? []).filter((x) => x.kind !== 'manual' && !x.skip);
  return { n: auto.findIndex((x) => x.id === step.value?.id) + 1, of: auto.length };
});
// The texts render in Go (ade.step.prompt, headless.space.suffix, headless.finishStep.suffix), so edits in Settings show here.
const stepPreview = useAdePromptPreview('ade.step.prompt', () =>
  card.value && step.value
    ? runMessageVars({
        title: card.value.title,
        jira: card.value.task.jira,
        step: {
          name: step.value.name,
          prompt: def.value?.prompt ?? '',
          results: step.value.results,
        },
        stepNumber: numbering.value.n,
        stepCount: numbering.value.of,
        noRepo: noRepo.value,
      })
    : null,
);
const suffixPreview = useAdePromptPreview('headless.finishStep.suffix', () =>
  step.value ? finishVars(step.value.results) : null,
);
const spacePreview = useAdePromptPreview('headless.space.suffix', () => (step.value ? {} : null));
// The Kira Space note and the finish instruction the step ends with, as ade.step.prompt appends them.
const suffix = computed(() => {
  const finish = previewPlain(suffixPreview.data.value?.parts ?? []);
  const space = previewPlain(spacePreview.data.value?.parts ?? []);
  return space ? `${space}\n\n${finish}` : finish;
});
const initial = computed(() =>
  editablePart(previewPlain(stepPreview.data.value?.parts ?? []), suffix.value),
);
// A smart step runs its script's body: shown read-only, and no message is sent.
const scripts = useCustomScriptsStore();
const smartStep = computed(() => {
  const cur = step.value;
  return cur && cur.smartScript !== ''
    ? { smartScript: cur.smartScript, params: def.value?.params ?? {} }
    : null;
});
const smartBody = computed(() => {
  const step = smartStep.value;
  const c = card.value;
  const script = step ? scripts.records.find((r) => r.name === step.smartScript) : undefined;
  if (!step || !c || !script) return null;
  return smartBodyParts(script.command, step.params, {
    task: c.title,
    jira: c.task.jira?.key ?? '',
  });
});
const named = computed(
  () => card.value?.rows.filter((r) => r.branch.kind === 'mine' && r.branch.name === '') ?? [],
);
// An agent step leaves an empty name to the agent (request_branches; the user approves); another step derives it.
const askable = computed(() => def.value?.kind === 'agent' || Boolean(def.value?.smartScript));
const slug = computed(() => `feat/${branchSlug(card.value?.title ?? '')}`);
const placeholder = computed(() =>
  askable.value ? t('space.ade.runDialog.branchName.placeholder') : slug.value,
);

// Setups of the task's branches still running or failed: the runs queue behind them after Start.
const setups = computed(() =>
  (card.value?.rows ?? []).filter(
    (r) =>
      r.branch.kind === 'mine' &&
      r.branch.name !== '' &&
      ['running', 'failed'].includes(r.branch.setup?.state ?? ''),
  ),
);

const message = ref('');
const names = ref<Record<string, string>>({});
// Snapshot at open: a board push that renames the task must not make an untouched message read as edited.
const opening = ref('');
const edited = computed(() => message.value !== opening.value);
const dirty = computed(
  () => edited.value || Object.values(names.value).some((n) => n.trim() !== ''),
);

watch(open, (o) => {
  if (!o) return;
  opening.value = initial.value;
  message.value = initial.value;
  names.value = {};
  sending.error.value = null;
});
// The preview lands after the dialog opens: an untouched message takes it over.
watch(initial, (next) => {
  if (!open.value || message.value !== opening.value) return;
  opening.value = next;
  message.value = next;
});

const sending = useAsyncAction(async () => {
  if (!card.value) return;
  const branchNames: Record<string, string> = {};
  for (const r of named.value) branchNames[r.id] = (names.value[r.id] ?? '').trim();
  await start.mutateAsync({
    taskId: card.value.task.id,
    branchNames,
    message: edited.value && !smartStep.value ? message.value : '',
  });
  close();
});
const { pending: busy, error } = sending;
const send = sending.run;
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && close()">
    <DialogContent size="lg" :dirty="dirty" data-testid="ade-run-dialog">
      <DialogHeader>
        <DialogTitle class="flex items-center gap-2">
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke-width="2.2"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
            class="stroke-agent"
          >
            <rect x="3" y="4" width="18" height="16" rx="3" />
            <path d="M7 10l3 2-3 2M12 15h5" />
          </svg>
          {{ t('space.ade.runDialog.title.title', { step: step?.name ?? '' }) }}
        </DialogTitle>
      </DialogHeader>
      <DialogBody class="flex flex-col gap-3 text-kira-md">
        <FieldDescription v-if="noRepo" data-testid="ade-run-no-repo">{{
          t('space.ade.runDialog.noRepo.hint')
        }}</FieldDescription>
        <div v-if="named.length" class="flex flex-col gap-1.5" data-testid="ade-run-branches">
          <div v-for="r in named" :key="r.id" class="flex items-center gap-3">
            <AdeRepoTag :code-repo-id="r.branch.codeRepoId" :label="r.repo" />
            <Label
              :for="`ade-run-branch-${r.id}`"
              class="whitespace-nowrap text-muted-foreground"
              >{{
                t('common.field.branch.label')
              }}</Label
            >
            <Input
              :id="`ade-run-branch-${r.id}`"
              v-model="names[r.id]"
              class="flex-1 font-data"
              :placeholder="placeholder"
              data-testid="ade-run-branch"
            />
          </div>
        </div>
        <AdeSetupProgress
          v-for="r in setups"
          :key="r.id"
          :branch="r.branch"
          :repo="r.repo"
          data-testid="ade-run-setup"
        />
        <div
          v-if="handoverIn || taskNotes"
          class="flex flex-col gap-1.5"
          data-testid="ade-run-context"
        >
          <div v-if="handoverIn" class="flex flex-col gap-1" data-testid="ade-run-handover">
            <span class="text-muted-foreground">{{
              t('space.ade.runDialog.handover.label', { stage: handoverIn.fromStepName })
            }}</span>
            <Card class="whitespace-pre-wrap break-words font-data text-kira-sm">{{
              handoverIn.body
            }}</Card>
          </div>
          <div v-if="taskNotes" class="flex flex-col gap-1" data-testid="ade-run-notes">
            <span class="text-muted-foreground">{{
              t('space.ade.runDialog.taskNotes.label')
            }}</span>
            <Card class="whitespace-pre-wrap break-words font-data text-kira-sm">{{
              taskNotes
            }}</Card>
          </div>
        </div>
        <div v-if="smartStep" class="flex flex-col gap-1" data-testid="ade-run-smart">
          <div class="flex items-center gap-2 text-muted-foreground">
            <SmartBadge />
            <span>{{ smartStep.smartScript }}</span>
          </div>
          <Card
            v-if="smartBody"
            class="whitespace-pre-wrap font-data text-kira-sm"
            data-testid="ade-run-smart-body"
          >
            <VarText :parts="smartBody" />
          </Card>
          <FieldError v-else data-testid="ade-run-smart-missing">{{
            t('space.ade.runDialog.smartMissing.error')
          }}</FieldError>
          <FieldDescription data-testid="ade-run-suffix">{{ suffix }}</FieldDescription>
        </div>
        <div v-else class="flex flex-col gap-1">
          <div class="flex items-center gap-2">
            <Label for="ade-run-message" class="flex-1 text-muted-foreground">{{
              t('space.ade.agentDialog.message.label')
            }}</Label>
            <Button
              v-if="edited"
              variant="toolbar"
              size="kira"
              data-testid="ade-run-reset"
              @click="message = initial"
            >
              {{ t('common.action.reset.label') }}
            </Button>
          </div>
          <div class="flex flex-col gap-1">
            <Textarea
              id="ade-run-message"
              v-model="message"
              class="max-h-90 resize-y"
              data-testid="ade-run-message"
            />
            <FieldDescription data-testid="ade-run-suffix">
              {{ suffix }}
            </FieldDescription>
          </div>
        </div>
        <Alert v-if="error" variant="error" data-testid="ade-run-error">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
      </DialogBody>
      <DialogFooter>
        <Button variant="dialog" size="kira-lg" data-testid="ade-run-cancel" @click="close">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :class="ACTION_CLASS.agent"
          :pending="busy"
          data-testid="ade-run-send"
          @click="send"
        >
          {{ t('space.ade.agentDialog.runInBackground.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
