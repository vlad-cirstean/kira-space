<script setup lang="ts">
import CodeBlock from '@theme/components/CodeBlock.vue';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Skeleton } from '@theme/components/ui/skeleton';
import { Textarea } from '@theme/components/ui/textarea';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import SmartBadge from '@workbench/automations/smart/SmartBadge.vue';
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useCustomScriptsStore } from '../../../state/customScripts';
import AdeRepoTag from '../AdeRepoTag.vue';
import { smartBodyParts } from '../automation/smartStepBody';
import { branchSlug, editablePart, previewPlain } from '../board/runMessage';
import AdeLinkStrip from '../links/AdeLinkStrip.vue';
import AdeSetupProgress from '../panel/AdeSetupProgress.vue';
import { usePlanModel } from '../plan/usePlanModel';
import { useTaskWrites } from '../plan/useTaskWrites';
import { useAdePromptPreview } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import { useAdeOpsStore } from '../state/adeOps';
import { useAdeStageDialogStore } from '../state/adeStageDialog';
import { timedCall } from '../state/refusalTiming';
import { useRepoGate } from '../state/useRepoGate';
import { RUN_PREVIEW_IDS, runPreviewVars } from './runPreview';

// Agent step Run dialog (SPEC2 section 5, R24): per-branch names for branches not created yet, the
// editable message and the read-only finish suffix the server always appends.
const ui = useAdeBoardUiStore();
const { model } = usePlanModel();
const { start } = useTaskWrites();
const ops = useAdeOpsStore();
const stage = useAdeStageDialogStore();
const dialogs = useAdeDialogsStore();
const { opAfterRepo } = useRepoGate();
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
// The workflow's definition of the step being run: its prompt and smart script params.
const def = computed(
  () => card.value?.workflow?.steps.find((x) => x.id === step.value?.id) ?? null,
);
// The texts render in Go (ade.step.prompt, headless.space.suffix, headless.finishStep.suffix), so edits in Settings show here.
const previewVars = computed(() => runPreviewVars(card.value));
const stepPreview = useAdePromptPreview(RUN_PREVIEW_IDS.step, () => previewVars.value.step);
const suffixPreview = useAdePromptPreview(RUN_PREVIEW_IDS.finish, () => previewVars.value.finish);
const spacePreview = useAdePromptPreview(RUN_PREVIEW_IDS.space, () => previewVars.value.space);
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
// An agent step leaves an empty name to the agent (state branch.request; the user approves); another step derives it.
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

// Inputs of a start the bridge refused (or the user cancelled at the repo prompt) live in `ui.runDrafts`:
// the dialog reopens with them at once when the refusal is prompt, else on the task's next open.
let alive = true;
onBeforeUnmount(() => {
  alive = false;
});

watch(open, (o) => {
  if (!o) return;
  const id = ui.runTaskId ?? '';
  const kept = ui.runDrafts[id];
  delete ui.runDrafts[id];
  opening.value = initial.value;
  message.value = kept?.message ?? initial.value;
  names.value = kept?.names ?? {};
});
// The preview lands after the dialog opens: an untouched message takes it over.
watch(initial, (next) => {
  if (!open.value || message.value !== opening.value) return;
  opening.value = next;
  message.value = next;
});

// Closes at once; the start runs as the task's op (chip on the card, toast past 2 s or on failure).
// A start refused promptly reopens the dialog with the typed names and message, unless another dialog or
// mode took over; a late refusal keeps them for the task's next open (the error toast reports it).
async function send(): Promise<void> {
  const c = card.value;
  if (!c) return;
  const branchNames: Record<string, string> = {};
  for (const r of named.value) branchNames[r.id] = (names.value[r.id] ?? '').trim();
  const taskId = c.task.id;
  const body = edited.value && !smartStep.value ? message.value : '';
  const typed = { message: message.value, names: { ...names.value } };
  close();
  const id = `startRun:${taskId}`;
  const timed = timedCall(() => start.mutateAsync({ taskId, branchNames, message: body }));
  const ok = await opAfterRepo(taskId, timed.call, (fn) =>
    ops.run({ id, kind: 'startRun', taskId, name: c.title }, fn),
  );
  if (ok) {
    delete ui.runDrafts[taskId];
    return;
  }
  // A live op of the same id means the submit was a duplicate, not a failure.
  if (ops.isRunning(id)) return;
  ui.runDrafts[taskId] = typed;
  const free = ui.runTaskId === null && !stage.isOpen && dialogs.spec === null;
  if (!timed.prompt() || !free || !alive) return;
  ui.select(taskId);
  ui.runTaskId = taskId;
}
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && close()">
    <DialogContent size="lg" :dirty="dirty" data-testid="ade-run-dialog">
      <DialogHeader>
        <DialogTitle>
          {{ t('space.ade.runDialog.title.title', { step: step?.name ?? '' }) }}
        </DialogTitle>
      </DialogHeader>
      <DialogBody>
        <div v-if="named.length" class="flex flex-col gap-1.5" data-testid="ade-run-branches">
          <div v-for="(r, i) in named" :key="r.id" class="flex items-center gap-3">
            <AdeRepoTag :code-repo-id="r.branch.codeRepoId" :label="r.repo" />
            <FieldLabel :for="`ade-run-branch-${r.id}`" class="whitespace-nowrap">{{
              t('common.field.branch.label')
            }}</FieldLabel>
            <Input
              :id="`ade-run-branch-${r.id}`"
              v-model="names[r.id]"
              class="flex-1 font-data"
              :placeholder="placeholder"
              :data-dialog-initial="i === 0 || undefined"
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
        <div v-if="card" data-testid="ade-run-links">
          <AdeLinkStrip :task="card.task" mode="view" />
        </div>
        <div
          v-if="handoverIn || taskNotes"
          class="flex flex-col gap-1.5"
          data-testid="ade-run-context"
        >
          <div v-if="handoverIn" class="flex flex-col gap-1" data-testid="ade-run-handover">
            <span class="text-muted-foreground">{{
              t('space.ade.runDialog.handover.label', { stage: handoverIn.fromStepName })
            }}</span>
            <CodeBlock class="break-words">{{ handoverIn.body }}</CodeBlock>
          </div>
          <div v-if="taskNotes" class="flex flex-col gap-1" data-testid="ade-run-notes">
            <span class="text-muted-foreground">{{
              t('space.ade.runDialog.taskNotes.label')
            }}</span>
            <CodeBlock class="break-words">{{ taskNotes }}</CodeBlock>
          </div>
        </div>
        <div v-if="smartStep" class="flex flex-col gap-1" data-testid="ade-run-smart">
          <div class="flex items-center gap-2 text-muted-foreground">
            <SmartBadge />
            <span>{{ smartStep.smartScript }}</span>
          </div>
          <CodeBlock v-if="smartBody" data-testid="ade-run-smart-body">
            <VarText :parts="smartBody" />
          </CodeBlock>
          <FieldError v-else data-testid="ade-run-smart-missing">{{
            t('space.ade.runDialog.smartMissing.error')
          }}</FieldError>
          <FieldDescription data-testid="ade-run-suffix">{{ suffix }}</FieldDescription>
        </div>
        <div v-else class="flex flex-col gap-1">
          <div class="flex items-center gap-2">
            <FieldLabel for="ade-run-message" class="flex-1">{{
              t('space.ade.agentDialog.message.label')
            }}</FieldLabel>
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
            <Skeleton
              v-if="stepPreview.isPending.value"
              class="h-24 w-full"
              data-testid="ade-run-message-skeleton"
            />
            <Textarea
              class="min-h-24 max-h-90 resize-y"
              v-else
              id="ade-run-message"
              v-model="message"
              :data-dialog-initial="named.length === 0 || undefined"
              data-testid="ade-run-message"
            />
            <FieldDescription data-testid="ade-run-suffix">
              {{ suffix }}
            </FieldDescription>
          </div>
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="dialog" size="kira-lg" data-testid="ade-run-cancel" @click="close">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :data-dialog-initial="(smartStep && named.length === 0) || undefined"
          data-testid="ade-run-send"
          @click="send"
        >
          {{ t('space.ade.agentDialog.runInBackground.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
