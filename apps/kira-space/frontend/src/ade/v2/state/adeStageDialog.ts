import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { stageSessions } from '../board/stageSessions';
import { stepMoves } from '../board/stepMoves';
import { usePlanModel } from '../plan/usePlanModel';
import { useDismissHandover, useNextPass, useSetTaskStep, useStepDone } from '../queries';
import { useSessions } from '../readQueries';
import type { Handover } from '../wire';
import { useAdeBoardUiStore } from './adeBoardUi';
import { useAdeOpsStore } from './adeOps';

// The stage-change dialog: which task it is open for, the target stage and the handover notes typed
// for it. One concern: the stage move with notes. Every opener (step mover, task menu, Next stage action,
// the routed `handover` prompt) goes through here. Pass mode is the same dialog for a finished
// orchestrator: the target is fixed to its first step and the notes are the optional pass message.
export const useAdeStageDialogStore = defineStore('adeStageDialog', () => {
  const { model } = usePlanModel();
  const stepDone = useStepDone();
  const setStep = useSetTaskStep();
  const nextPass = useNextPass();
  const dismissM = useDismissHandover();
  const sessions = useSessions();
  const ops = useAdeOpsStore();
  const ui = useAdeBoardUiStore();

  const taskId = ref<string | null>(null);
  /** Opened by the routed `handover` prompt rather than by a board action. */
  const routed = ref(false);
  /** `pass` starts the next pass of a finished orchestrator instead of moving between stages. */
  const mode = ref<'move' | 'pass'>('move');
  /** The stage the dialog was asked to move to; '' = the natural next one. */
  const asked = ref('');
  /** What the user picked or typed; null until then, so the defaults follow the board as it loads. */
  const picked = ref<string | null>(null);
  const typed = ref<string | null>(null);
  /** Whether the move closes the sessions of the stage left. */
  const closeChoice = ref<'close' | 'keep'>('close');
  /** A draft the opener already holds: the board snapshot may not list it yet. */
  const seeded = ref<Handover | null>(null);

  const card = computed(() => (taskId.value ? (model.value?.cardFor(taskId.value) ?? null) : null));
  const fromStepId = computed(() => {
    const c = card.value;
    if (!c) return '';
    return c.progress.finished ? 'done' : (c.progress.step?.id ?? c.task.entryStepId);
  });
  const moves = computed(() =>
    card.value ? stepMoves(card.value.blocks, fromStepId.value, card.value.task.runs) : null,
  );
  /** The open draft of the stage being left, the notes' starting text; a pass has none. */
  const draft = computed(() => {
    if (mode.value === 'pass') return null;
    const board = card.value?.task.handoverDrafts.find((d) => d.fromStepId === fromStepId.value);
    if (board) return board;
    const seed = seeded.value;
    return seed && seed.taskId === taskId.value && seed.fromStepId === fromStepId.value
      ? seed
      : null;
  });
  /** The first step a pass enters. */
  const passStep = computed(
    () => moves.value?.options.find((o) => o.id !== 'done' && !o.skipped) ?? null,
  );
  const prefill = computed(() => draft.value?.body ?? '');
  const toStepId = computed({
    get: () =>
      mode.value === 'pass'
        ? (passStep.value?.id ?? '')
        : (picked.value ?? (asked.value || (moves.value?.next?.id ?? ''))),
    set: (v: string) => {
      picked.value = v;
    },
  });
  const notes = computed({
    get: () => typed.value ?? prefill.value,
    set: (v: string) => {
      typed.value = v;
    },
  });
  /** Sessions the move leaves behind; follows the picked target. */
  const closable = computed(() =>
    stageSessions(
      sessions.data.value?.sessions ?? [],
      taskId.value ?? '',
      fromStepId.value,
      toStepId.value,
    ),
  );
  const isOpen = computed(() => taskId.value !== null && card.value !== null);

  function open(args: {
    taskId: string;
    toStepId?: string;
    routed?: boolean;
    seed?: Handover | null;
  }): void {
    taskId.value = args.taskId;
    mode.value = 'move';
    routed.value = args.routed ?? false;
    asked.value = args.toStepId ?? '';
    picked.value = null;
    typed.value = null;
    closeChoice.value = 'close';
    seeded.value = args.seed ?? null;
  }

  /** Opens the dialog for the next pass of a finished orchestrator. */
  function openPass(id: string): void {
    open({ taskId: id });
    mode.value = 'pass';
  }

  function close(): void {
    taskId.value = null;
    seeded.value = null;
  }

  /**
   * Closes at once; the move runs as the task's op (chip, toast past 2 s or on failure). A refused move
   * reopens the dialog on the board mount (the routed popup is gone by then) with the typed notes and
   * choices kept, unless another dialog took the slot meanwhile.
   */
  async function submit(): Promise<boolean> {
    const c = card.value;
    const m = moves.value;
    if (!c || !m || !toStepId.value) return false;
    const id = c.task.id;
    const name = c.title;
    const sendNotes = notes.value !== prefill.value ? { notes: notes.value } : {};
    const closing =
      closable.value.length > 0 && closeChoice.value === 'close' ? { closeSessions: true } : {};
    const from = fromStepId.value;
    const to = toStepId.value;
    const manual = c.progress.step?.kind === 'manual' && !c.progress.finished;
    const isPass = mode.value === 'pass';
    const passNotes = notes.value.trim() === '' ? {} : { notes: notes.value };
    const advance = manual && to === m.next?.id;
    const kept = {
      mode: mode.value,
      asked: asked.value,
      picked: picked.value,
      typed: typed.value,
      closeChoice: closeChoice.value,
      seeded: seeded.value,
    };
    close();
    const kind = isPass ? 'nextPass' : 'setStep';
    const ok = await ops.run({ id: `${kind}:${id}`, kind, taskId: id, name }, async () => {
      if (isPass) {
        await nextPass.mutateAsync({ taskId: id, fromStepId: from, ...passNotes, ...closing });
      } else if (advance) {
        await stepDone.mutateAsync({ taskId: id, ...sendNotes, ...closing });
      } else {
        await setStep.mutateAsync({
          taskId: id,
          stepId: to,
          fromStepId: from,
          ...sendNotes,
          ...closing,
        });
      }
    });
    if (!ok) {
      ui.select(id);
      if (taskId.value === null) {
        taskId.value = id;
        routed.value = false;
        mode.value = kept.mode;
        asked.value = kept.asked;
        picked.value = kept.picked;
        typed.value = kept.typed;
        closeChoice.value = kept.closeChoice;
        seeded.value = kept.seeded;
      }
    }
    return true;
  }

  /** Not now on the routed prompt: keeps the draft and withdraws the popup. */
  const dismissal = useAsyncAction(async () => {
    if (taskId.value) await dismissM.mutateAsync({ taskId: taskId.value });
  });

  const error = computed(() => dismissal.error.value ?? '');
  /** Edits the draft would lose on close. */
  const dirty = computed(() => notes.value !== prefill.value);

  return {
    taskId,
    mode,
    passStep,
    toStepId,
    notes,
    error,
    dirty,
    card,
    moves,
    fromStepId,
    draft,
    closable,
    closeChoice,
    routed,
    isOpen,
    open,
    openPass,
    close,
    submit,
    dismiss: dismissal.run,
  };
});
