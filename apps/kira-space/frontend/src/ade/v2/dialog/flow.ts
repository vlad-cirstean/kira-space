import { type MessageKey, t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { isPreparing } from '../state/useSetupWait';
import type {
  ArchiveRisk,
  Launch,
  OpenSessionEvent,
  RebaseArgs,
  RecordMergeArgs,
  SetBranchBaseArgs,
  TaskArgs,
} from '../wire';
import {
  archiveSpec,
  atRisk,
  type DialogCtx,
  type DialogSpec,
  type DialogState,
  resolvedChoice,
  templateFor,
} from './compose';
import { type DeliverTarget, deliver, type LaunchDeps } from './deliver';
import type { TurnOutcome, TurnWatch } from './turnWatch';

// The Send button's delivery per dialog kind. Deps are injected (mutations, `adeTurns`, store
// writers) so the flow has no Vue and the specs drive it with fakes.

export interface FlowDeps {
  ctx: DialogCtx;
  launch: LaunchDeps;
  turns: { watch: (terminalId: string, opts: { requireSubmit: boolean }) => TurnWatch };
  /** Starts the rebase in the background: the op's chip and toast report it. */
  rebase: (args: RebaseArgs) => void;
  setBranchBase: (args: SetBranchBaseArgs) => Promise<void>;
  recordMerge: (args: RecordMergeArgs) => Promise<void>;
  archiveTask: (args: TaskArgs) => Promise<void>;
  archiveRisk: (args: TaskArgs) => Promise<ArchiveRisk>;
  /** Asks the person to confirm archiving a task with nothing at risk. */
  confirmArchive: (taskId: string) => Promise<boolean>;
  /** In-flight keys (`rebase:<branch>`, `merge:<branch>:<target>`, `archive:<task>`). */
  pending: { add: (key: string) => void; remove: (key: string) => void };
  /** A failure while the dialog is open. */
  setError: (message: string) => void;
  /** The launch hit a worktree still being prepared: hold the dialog open and retry when ready. */
  waitForSetup: () => void;
  /** A failure after the dialog closed, shown in the task's panel header. */
  setActionError: (taskId: string, message: string) => void;
  closeDialog: () => void;
  openDialog: (spec: DialogSpec) => void;
  /** A launch created a session: select it in its Sessions tab. */
  opened: (e: OpenSessionEvent) => void;
}

const BLOCKED_REASON = {
  notAWorktree: 'space.ade.flow.notAWorktree.status',
  mainWorktree: 'space.ade.flow.mainWorktree.status',
  currentWorktree: 'space.ade.flow.currentWorktree.status',
  openInAnotherWindow: 'common.openElsewhere.tooltip',
  locked: 'space.ade.flow.locked.status',
} as const satisfies Record<string, MessageKey>;
export const blockedReason = (reason: string): string =>
  reason in BLOCKED_REASON ? t(BLOCKED_REASON[reason as keyof typeof BLOCKED_REASON]) : reason;

const errMessage = (err: unknown): string => errorText(err);

/** A launch failure shown in the dialog; a worktree still being prepared holds it open instead. */
function failInDialog(deps: FlowDeps, err: unknown): void {
  if (isPreparing(err)) deps.waitForSetup();
  else deps.setError(errMessage(err));
}

function messageOf(deps: FlowDeps, spec: DialogSpec, state: DialogState): string {
  return state.msg ?? templateFor(deps.ctx, spec, state);
}

/** `choice` is a running session's id, or `'new'`: a fresh launch on the branch. */
function targetFor(
  deps: FlowDeps,
  branchId: string,
  choice: string,
  message: string,
): DeliverTarget {
  if (choice === 'new') return { kind: 'start', branchId, message };
  const s = deps.ctx.sessions.find((x) => x.id === choice);
  return { kind: 'send', sessionId: choice, terminalId: s?.terminalId ?? '', message };
}

function taskOfBranch(deps: FlowDeps, branchId: string): string {
  return deps.ctx.graph.byBranch.get(branchId)?.taskId ?? '';
}

function announce(deps: FlowDeps, launch: Launch | null, taskId: string, branchId: string): void {
  if (launch) deps.opened({ taskId, branchId, sessionId: launch.sessionId });
}

/** Delivers to the dialog's one branch target, arming the turn watch at the right moment. */
async function deliverToBranch(
  deps: FlowDeps,
  spec: DialogSpec,
  message: string,
): Promise<{ watch: TurnWatch | null; launch: Launch | null }> {
  const branchId = spec.branchId ?? '';
  const tg = spec.targets.find((t) => t.branchId === branchId);
  const choice = tg ? resolvedChoice(deps.ctx, tg) : 'new';
  let watch: TurnWatch | null = null;
  try {
    const launch = await deliver(
      deps.launch,
      targetFor(deps, branchId, choice, message),
      (id, requireSubmit) => {
        watch = deps.turns.watch(id, { requireSubmit });
      },
    );
    return { watch, launch };
  } catch (err) {
    // The watch can already be armed for the very delivery that failed: cancel it or it leaks.
    (watch as TurnWatch | null)?.cancel();
    throw err;
  }
}

/** Rebase / queue / change base: the server runs the rebase in the background, so the dialog
 *  closes once it accepts; a draft branch only stores its base. */
async function sendRebase(deps: FlowDeps, spec: DialogSpec, state: DialogState): Promise<void> {
  const branchId = spec.branchId ?? '';
  const key = `rebase:${branchId}`;
  deps.pending.add(key);
  try {
    if (spec.draft) {
      await deps.setBranchBase({ branchId, base: spec.onto ?? { ref: '', branchId: '' } });
    } else {
      deps.rebase({
        branchId,
        onto: spec.onto ?? null,
        queueWith: spec.queueWith ?? '',
        push: state.push,
        autostash: state.autostash,
        message: state.msg ?? '',
      });
    }
    deps.closeDialog();
  } catch (err) {
    deps.setError(errMessage(err));
  } finally {
    deps.pending.remove(key);
  }
}

/** Merge: recorded only when the dialog's own turn finishes (`Stop`), never on `SessionEnd`. */
async function sendMerge(deps: FlowDeps, spec: DialogSpec, state: DialogState): Promise<void> {
  const branchId = spec.branchId ?? '';
  const target = spec.target ?? '';
  const taskId = taskOfBranch(deps, branchId);
  const key = `merge:${branchId}:${target}`;
  deps.pending.add(key);
  try {
    const { watch, launch } = await deliverToBranch(deps, spec, messageOf(deps, spec, state));
    announce(deps, launch, taskId, branchId);
    deps.closeDialog();
    void (watch as TurnWatch | null)?.done.then(async (outcome: TurnOutcome) => {
      try {
        if (outcome === 'ended') {
          deps.setActionError(taskId, t('space.ade.flow.mergeEnded.error'));
        } else {
          await deps.recordMerge({ branchId, target });
        }
      } catch (err) {
        deps.setActionError(taskId, errMessage(err));
      } finally {
        deps.pending.remove(key);
      }
    });
  } catch (err) {
    deps.pending.remove(key);
    failInDialog(deps, err);
  }
}

async function sendLaunch(deps: FlowDeps, spec: DialogSpec, state: DialogState): Promise<void> {
  // An unedited message goes as `''`: the server composes its own default.
  const message = state.msg ?? '';
  try {
    const launch =
      spec.kind === 'session'
        ? await deliver(
            deps.launch,
            { kind: 'session', taskId: spec.taskId ?? '', message },
            () => {},
          )
        : await deliver(
            deps.launch,
            { kind: 'start', branchId: spec.branchId ?? '', message },
            () => {},
          );
    if (spec.kind === 'session') announce(deps, launch, spec.taskId ?? '', '');
    else announce(deps, launch, taskOfBranch(deps, spec.branchId ?? ''), spec.branchId ?? '');
    deps.closeDialog();
  } catch (err) {
    failInDialog(deps, err);
  }
}

/** After every watched turn ends: archive when nothing is at risk, else reopen with fresh risk. */
async function onArchiveTurns(
  deps: FlowDeps,
  taskId: string,
  outcomes: TurnOutcome[],
): Promise<void> {
  const key = `archive:${taskId}`;
  try {
    if (outcomes.includes('ended')) {
      deps.setActionError(taskId, 'The session ended before archiving; archive again when ready');
      return;
    }
    await archiveIfClear(deps, taskId);
  } catch (err) {
    deps.setActionError(taskId, errMessage(err));
  } finally {
    deps.pending.remove(key);
  }
}

async function sendArchive(deps: FlowDeps, spec: DialogSpec, state: DialogState): Promise<void> {
  const taskId = spec.taskId ?? '';
  const message = messageOf(deps, spec, state);
  const key = `archive:${taskId}`;
  const watches: TurnWatch[] = [];
  let sent = 0;
  deps.pending.add(key);
  try {
    for (const tg of spec.targets) {
      const choice = resolvedChoice(deps.ctx, tg);
      const launch = await deliver(
        deps.launch,
        targetFor(deps, tg.branchId, choice, message),
        (id, requireSubmit) => {
          watches.push(deps.turns.watch(id, { requireSubmit }));
        },
      );
      announce(deps, launch, taskId, tg.branchId);
      sent++;
    }
  } catch (err) {
    for (const w of watches) w.cancel();
    deps.pending.remove(key);
    if (sent === 0) {
      failInDialog(deps, err);
      return;
    }
    // Some branches already have the prompt: close, so Send cannot repeat it to them.
    deps.setActionError(
      taskId,
      t('space.ade.flow.archivePartial.error', {
        sent,
        count: spec.targets.length,
        error: errMessage(err),
      }),
    );
    deps.closeDialog();
    return;
  }
  deps.closeDialog();
  void Promise.all(watches.map((w) => w.done)).then((outcomes) =>
    onArchiveTurns(deps, taskId, outcomes),
  );
}

function blockedText(deps: FlowDeps, risk: ArchiveRisk): string {
  const parts = risk.branches
    .filter((r) => r.blocked !== '')
    .map((r) => {
      const name = deps.ctx.graph.byBranch.get(r.branchId)?.name ?? r.branchId;
      return t('theme.varText.aria', { name, value: blockedReason(r.blocked) });
    });
  return t('space.ade.flow.cantArchive.error', { parts: parts.join('; ') });
}

/** Reads the risk and archives when clear, after `confirm` asks when set. A blocked branch is an
 *  action error; work at risk opens the dialog, with fresh risk after a send-then-archive turn. */
async function archiveIfClear(deps: FlowDeps, taskId: string, confirm = false): Promise<void> {
  const risk = await deps.archiveRisk({ taskId });
  if (risk.branches.some((r) => r.blocked !== '')) {
    deps.setActionError(taskId, blockedText(deps, risk));
    return;
  }
  if (risk.branches.some(atRisk)) {
    deps.openDialog(archiveSpec(deps.ctx, risk));
    return;
  }
  if (confirm && !(await deps.confirmArchive(taskId))) return;
  await deps.archiveTask({ taskId });
}

/** The Archive click: nothing at risk asks to confirm, else the dialog. */
export async function requestArchive(deps: FlowDeps, taskId: string): Promise<void> {
  const key = `archive:${taskId}`;
  deps.pending.add(key);
  try {
    await archiveIfClear(deps, taskId, true);
  } catch (err) {
    deps.setActionError(taskId, errMessage(err));
  } finally {
    deps.pending.remove(key);
  }
}

/** The archive dialog's `Delete anyway`: no discard flag exists, the dialog is the confirmation. */
export async function deleteAnyway(deps: FlowDeps, spec: DialogSpec): Promise<void> {
  try {
    deps.closeDialog();
    await deps.archiveTask({ taskId: spec.taskId ?? '' });
  } catch (err) {
    deps.setError(errMessage(err));
  }
}

export async function sendDialog(
  deps: FlowDeps,
  spec: DialogSpec,
  state: DialogState,
): Promise<void> {
  switch (spec.kind) {
    case 'rebase':
    case 'queue':
    case 'changeBase':
      return sendRebase(deps, spec, state);
    case 'merge':
      return sendMerge(deps, spec, state);
    case 'archive':
      return sendArchive(deps, spec, state);
    default:
      return sendLaunch(deps, spec, state);
  }
}
