import { errorText } from '@workbench/bridge/codedError';
import { computed, type MaybeRefOrGetter, toValue } from 'vue';
import type { TaskAction } from '../board/actions';
import type { CardModel } from '../plan/usePlanModel';
import { useTaskWrites } from '../plan/useTaskWrites';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import { type AdeOpKind, useAdeOpsStore } from '../state/adeOps';
import { useAdeStageDialogStore } from '../state/adeStageDialog';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
import { useRepoGate } from '../state/useRepoGate';

/** Ops that start or move a task's work: while one runs the task's action buttons are busy. */
const BUSY_KINDS: ReadonlySet<AdeOpKind> = new Set(['startRun', 'approve', 'retry', 'nextPass']);

/** The task's rendered step action and the call behind it. */
export function useTaskAction(card: MaybeRefOrGetter<CardModel>) {
  const ui = useAdeBoardUiStore();
  const dialogs = useAdeDialogsStore();
  const takeOver = useAdeTakeOverStore();
  const ops = useAdeOpsStore();
  const writes = useTaskWrites();
  const stage = useAdeStageDialogStore();
  const { ensureRepo, opAfterRepo } = useRepoGate();

  const action = computed<TaskAction | null>(() => toValue(card).action);

  /** True while any start, approve, retry or next pass of this task runs, from any surface. */
  const busy = computed(() =>
    ops.forTask(toValue(card).task.id).some((o) => BUSY_KINDS.has(o.kind)),
  );

  /** Runs `call` as the task's op on `target` (the task, or its step); a repeat click of a live one is ignored. */
  async function runOp(
    kind: AdeOpKind,
    target: string,
    c: CardModel,
    call: () => Promise<unknown>,
  ): Promise<void> {
    const taskId = c.task.id;
    const ok = await opAfterRepo(taskId, call, (fn) =>
      ops.run({ id: `${kind}:${target}`, kind, taskId, name: c.title }, fn),
    );
    if (!ok) ui.select(taskId);
  }

  /** The step-bound actions: the ones that start or move work. */
  async function runStep(
    c: CardModel,
    a: TaskAction,
    step: NonNullable<CardModel['progress']['step']>,
  ): Promise<void> {
    const taskId = c.task.id;
    if (a.kind === 'run') {
      if (!(await ensureRepo(taskId, step.needsRepo))) return;
      if (step.kind === 'agent') ui.runTaskId = taskId;
      else
        await runOp('startRun', taskId, c, () =>
          writes.start.mutateAsync({ taskId, branchNames: {}, message: '' }),
        );
    } else if (a.kind === 'approve') {
      if (!(await ensureRepo(taskId, step.needsRepo))) return;
      await runOp('approve', `${taskId}:${a.stepId ?? ''}`, c, () =>
        writes.approve.mutateAsync({ taskId, stepId: a.stepId ?? '' }),
      );
    } else if (a.kind === 'retry') {
      const runs = c.progress.steps
        .find((s) => s.id === a.stepId)
        ?.runs.filter((r) => r.state === 'failed' || r.state === 'stuck');
      await runOp('retry', `${taskId}:${a.stepId ?? ''}`, c, () =>
        Promise.all((runs ?? []).map((r) => writes.retry.mutateAsync({ runId: r.runId }))),
      );
    } else if (a.kind === 'advance' || a.kind === 'finish') {
      stage.open({ taskId });
    }
  }

  async function run(): Promise<void> {
    const c = toValue(card);
    const a = action.value;
    if (!a || busy.value) return;
    const step = c.progress.step;
    const taskId = c.task.id;
    delete ui.actionError[taskId];
    if (a.kind === 'archive') return dialogs.archive(taskId);
    if (a.kind === 'nextPass') return stage.openPass(taskId);
    if (a.kind === 'takeOver' || a.kind === 'resume') return takeOver.request(a.sessionId ?? '');
    try {
      if (a.kind === 'session') {
        if (await ensureRepo(taskId, step?.needsRepo ?? false)) dialogs.session(taskId);
      } else if (step) {
        await runStep(c, a, step);
      }
    } catch (err) {
      ui.actionError[taskId] = errorText(err);
      ui.select(taskId);
    }
  }

  async function startNew(): Promise<void> {
    const c = toValue(card);
    const taskId = c.task.id;
    delete ui.actionError[taskId];
    try {
      if (await ensureRepo(taskId, c.progress.step?.needsRepo ?? false)) dialogs.session(taskId);
    } catch (err) {
      ui.actionError[taskId] = errorText(err);
      ui.select(taskId);
    }
  }

  return { action, perform: run, startNew, busy };
}
