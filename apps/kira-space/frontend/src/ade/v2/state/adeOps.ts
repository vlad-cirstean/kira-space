import { toast } from '@theme/components/ui/sonner';
import { t } from '@theme/i18n';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import { defineStore } from 'pinia';
import { reactive } from 'vue';
import { useAdeBoardUiStore } from './adeBoardUi';

export type AdeOpKind =
  // Client ops wrapping an awaited bridge call.
  | 'removeBranch'
  | 'removeRepo'
  | 'archive'
  | 'forcePush'
  | 'setBranch'
  | 'attachBranch'
  | 'stopRuns'
  | 'startRun'
  | 'approve'
  | 'retry'
  | 'resume'
  | 'nextPass'
  | 'setStep'
  | 'launch'
  | 'sendReview'
  | 'finishReview'
  | 'restartSession'
  | 'closeSession'
  | 'leaseRequest'
  | 'leaseRelease'
  // Backend ops mirrored from the op log (`adeOpMirror.ts`).
  | 'agentApproval'
  | 'agentBranches'
  | 'agentRename'
  | 'agentChange'
  | 'agentWorktreeRemove'
  | 'agentFetch'
  | 'agentPush';

export interface AdeOp {
  /** One op per id at a time: `<kind>:<target>`, the target being what the op acts on (branch, session,
   *  task, or the run or step of a task, so two runs of one task do not collide); backend ops `be:<record id>`. */
  id: string;
  kind: AdeOpKind;
  taskId?: string;
  branchId?: string;
  sessionId?: string;
  source: 'client' | 'backend';
}

export function opLoadingText(kind: AdeOpKind): string {
  switch (kind) {
    case 'removeBranch':
      return t('space.ade.ops.removeBranch.loading');
    case 'removeRepo':
      return t('space.ade.ops.removeRepo.loading');
    case 'archive':
      return t('space.ade.ops.archive.loading');
    case 'forcePush':
      return t('git.toolbar.forcePushing.loading');
    case 'setBranch':
      return t('space.ade.ops.setBranch.loading');
    case 'attachBranch':
      return t('space.ade.ops.attachBranch.loading');
    case 'stopRuns':
      return t('space.ade.ops.stopRuns.loading');
    case 'startRun':
      return t('space.ade.ops.startRun.loading');
    case 'approve':
      return t('space.ade.ops.approve.loading');
    case 'retry':
      return t('space.ade.ops.retry.loading');
    case 'resume':
      return t('space.ade.ops.resume.loading');
    case 'nextPass':
      return t('space.ade.ops.nextPass.loading');
    case 'setStep':
      return t('space.ade.ops.setStep.loading');
    case 'launch':
      return t('space.ade.ops.launch.loading');
    case 'sendReview':
      return t('space.ade.ops.sendReview.loading');
    case 'finishReview':
      return t('space.ade.ops.finishReview.loading');
    case 'restartSession':
      return t('space.ade.ops.restartSession.loading');
    case 'closeSession':
      return t('space.ade.ops.closeSession.loading');
    case 'leaseRequest':
      return t('space.ade.ops.leaseRequest.loading');
    case 'leaseRelease':
      return t('space.ade.ops.leaseRelease.loading');
    case 'agentApproval':
      return t('space.ade.ops.agentApproval.loading');
    case 'agentBranches':
      return t('space.ade.ops.agentBranches.loading');
    case 'agentRename':
      return t('space.ade.ops.agentRename.loading');
    case 'agentChange':
      return t('space.ade.ops.agentChange.loading');
    case 'agentWorktreeRemove':
      return t('space.ade.ops.agentWorktreeRemove.loading');
    case 'agentFetch':
      return t('space.ade.ops.agentFetch.loading');
    case 'agentPush':
      return t('space.ade.ops.agentPush.loading');
  }
}

/** Thrown inside an op's `fn` to end it at once with no error toast: the caller hands the work on (a repo prompt, then a fresh op). */
export class OpAborted extends Error {}

export type AdeOpInput = Omit<AdeOp, 'source'>;

// In-flight ADE ops: client ops (an awaited bridge call run in the background once confirmed) and
// backend ops mirrored from the op log. Rows show an op as a chip; a failed op toasts its error. One
// concern: in-flight ops.
export const useAdeOpsStore = defineStore('adeOps', () => {
  const ui = useAdeBoardUiStore();
  const ops = reactive(new Map<string, AdeOp>());

  function forTask(taskId: string): AdeOp[] {
    return [...ops.values()].filter((o) => o.taskId === taskId);
  }

  function forBranch(taskId: string, branchId: string): AdeOp[] {
    return forTask(taskId).filter((o) => o.branchId === branchId);
  }

  function forSession(sessionId: string): AdeOp[] {
    return [...ops.values()].filter((o) => o.sessionId === sessionId);
  }

  function isRunning(id: string): boolean {
    return ops.has(id);
  }

  function fail(op: AdeOp, err: unknown): string {
    const message = errorText(err);
    if (op.taskId) ui.actionError[op.taskId] = message;
    return message;
  }

  /** Starts `fn`; resolves true on success. A second run of a live id is ignored (resolves false); `OpAborted` ends it silently (false). A failure toasts its error. */
  async function run(input: AdeOpInput, fn: () => Promise<unknown>): Promise<boolean> {
    if (ops.has(input.id)) return false;
    const op: AdeOp = { ...input, source: 'client' };
    ops.set(op.id, op);
    if (op.taskId) delete ui.actionError[op.taskId];
    try {
      await fn();
    } catch (err) {
      ops.delete(op.id);
      if (err instanceof OpAborted) return false;
      toast.error(fail(op, err), { id: op.id });
      return false;
    }
    ops.delete(op.id);
    return true;
  }

  /** A backend op started: chip only. */
  function track(input: AdeOpInput): void {
    if (ops.has(input.id)) return;
    ops.set(input.id, { ...input, source: 'backend' });
  }

  /**
   * A backend op ended. Progress and result show as the chip and the op row; only a failure past
   * `TIMING.slowOp` toasts (a fast refusal is an answer to the agent, not a user failure; the approval
   * wait never).
   */
  function settle(id: string, result: { ok: boolean; error?: string; durationMs: number }): void {
    const op = ops.get(id);
    if (!op) return;
    ops.delete(id);
    if (result.ok || op.kind === 'agentApproval' || result.durationMs < TIMING.slowOp) return;
    toast.error(result.error || opLoadingText(op.kind), { id });
  }

  return { ops, forTask, forBranch, forSession, isRunning, run, track, settle };
});
