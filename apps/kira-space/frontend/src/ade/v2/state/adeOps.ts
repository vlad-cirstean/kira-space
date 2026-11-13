import { toast } from '@theme/components/ui/sonner';
import { t } from '@theme/i18n';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import { defineStore } from 'pinia';
import { reactive } from 'vue';
import { useAdeBoardUiStore } from './adeBoardUi';
import { slowNotice } from './slowNotice';

export type AdeOpKind =
  // P618: destructive, always toasted.
  | 'removeBranch'
  | 'removeRepo'
  | 'archive'
  // Client ops wrapping an awaited bridge call.
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
  | 'agentFetch'
  | 'agentPush';

export interface AdeOp {
  /** One op per id at a time: `<kind>:<target>`, the target being what the op acts on (branch, session,
   *  task, or the run or step of a task, so two runs of one task do not collide); backend ops `be:<record id>`. */
  id: string;
  kind: AdeOpKind;
  /** What the op acts on, named in the toast. */
  name: string;
  taskId?: string;
  branchId?: string;
  sessionId?: string;
  source: 'client' | 'backend';
}

/**
 * `always`: loading toast at once, result toast by id. `slow`: loading toast only once the op passes
 * `TIMING.slowOp`, success toast only if it showed. `never`: chip only. A failed client op toasts under
 * every policy. Backend kinds follow `settleBackend`, not this table.
 */
type ToastPolicy = 'always' | 'slow' | 'never';

const TOAST_POLICY: Record<AdeOpKind, ToastPolicy> = {
  removeBranch: 'always',
  removeRepo: 'always',
  archive: 'always',
  forcePush: 'slow',
  setBranch: 'slow',
  attachBranch: 'slow',
  stopRuns: 'slow',
  startRun: 'slow',
  approve: 'slow',
  retry: 'slow',
  resume: 'slow',
  nextPass: 'slow',
  setStep: 'slow',
  launch: 'slow',
  sendReview: 'slow',
  finishReview: 'slow',
  restartSession: 'slow',
  closeSession: 'slow',
  leaseRequest: 'slow',
  leaseRelease: 'slow',
  agentApproval: 'never',
  agentBranches: 'never',
  agentRename: 'never',
  agentChange: 'never',
  agentFetch: 'never',
  agentPush: 'never',
};

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
    case 'agentFetch':
      return t('space.ade.ops.agentFetch.loading');
    case 'agentPush':
      return t('space.ade.ops.agentPush.loading');
  }
}

function opDoneText(op: AdeOp): string {
  switch (op.kind) {
    case 'removeBranch':
      return t('space.ade.ops.removeBranch.status', { name: op.name });
    case 'removeRepo':
      return t('space.ade.ops.removeRepo.status', { name: op.name });
    case 'archive':
      return t('space.ade.ops.archive.status', { name: op.name });
    case 'forcePush':
      return t('space.ade.ops.forcePush.status', { name: op.name });
    case 'setBranch':
      return t('space.ade.ops.setBranch.status', { name: op.name });
    case 'attachBranch':
      return t('space.ade.ops.attachBranch.status', { name: op.name });
    case 'stopRuns':
      return t('space.ade.ops.stopRuns.status', { name: op.name });
    case 'startRun':
      return t('space.ade.ops.startRun.status', { name: op.name });
    case 'approve':
      return t('space.ade.ops.approve.status', { name: op.name });
    case 'retry':
      return t('space.ade.ops.retry.status', { name: op.name });
    case 'resume':
      return t('space.ade.ops.resume.status', { name: op.name });
    case 'nextPass':
      return t('space.ade.ops.nextPass.status', { name: op.name });
    case 'setStep':
      return t('space.ade.ops.setStep.status', { name: op.name });
    case 'launch':
      return t('space.ade.ops.launch.status', { name: op.name });
    case 'sendReview':
      return t('space.ade.ops.sendReview.status', { name: op.name });
    case 'finishReview':
      return t('space.ade.ops.finishReview.status', { name: op.name });
    case 'restartSession':
      return t('space.ade.ops.restartSession.status', { name: op.name });
    case 'closeSession':
      return t('space.ade.ops.closeSession.status', { name: op.name });
    case 'leaseRequest':
      return t('space.ade.ops.leaseRequest.status', { name: op.name });
    case 'leaseRelease':
      return t('space.ade.ops.leaseRelease.status', { name: op.name });
    case 'agentBranches':
      return t('space.ade.ops.agentBranches.status', { name: op.name });
    case 'agentRename':
      return t('space.ade.ops.agentRename.status', { name: op.name });
    case 'agentChange':
      return t('space.ade.ops.agentChange.status', { name: op.name });
    case 'agentFetch':
      return t('space.ade.ops.agentFetch.status', { name: op.name });
    case 'agentPush':
      return t('space.ade.ops.agentPush.status', { name: op.name });
    case 'agentApproval':
      return '';
  }
}

/** Thrown inside an op's `fn` to end it at once with no toast and no error: the caller hands the work on (a repo prompt, then a fresh op). */
export class OpAborted extends Error {}

export type AdeOpInput = Omit<AdeOp, 'source'>;

// In-flight ADE ops: client ops (an awaited bridge call run in the background once confirmed) and
// backend ops mirrored from the op log. Rows show an op as a chip; a toast reports the result by the
// kind's policy. One concern: in-flight ops.
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

  /** Starts `fn`; resolves true on success. A second run of a live id is ignored (resolves false); `OpAborted` ends it silently (false). */
  async function run(input: AdeOpInput, fn: () => Promise<unknown>): Promise<boolean> {
    if (ops.has(input.id)) return false;
    const op: AdeOp = { ...input, source: 'client' };
    ops.set(op.id, op);
    if (op.taskId) delete ui.actionError[op.taskId];
    const policy = TOAST_POLICY[op.kind];
    let aborted = false;
    const showLoading = () => toast.loading(opLoadingText(op.kind), { id: op.id });
    const notice =
      policy === 'slow'
        ? slowNotice({
            delayMs: TIMING.slowOp,
            show: showLoading,
            onSettle: ({ ok, error, shown }) => {
              if (aborted) {
                if (shown) toast.dismiss(op.id);
              } else if (!ok) toast.error(error ?? '', { id: op.id });
              else if (shown) toast.success(opDoneText(op), { id: op.id });
            },
          })
        : null;
    if (policy === 'always') showLoading();
    try {
      await fn();
    } catch (err) {
      ops.delete(op.id);
      if (err instanceof OpAborted) {
        aborted = true;
        if (notice) notice.settle(false);
        else toast.dismiss(op.id);
        return false;
      }
      const message = fail(op, err);
      if (notice) notice.settle(false, message);
      else toast.error(message, { id: op.id });
      return false;
    }
    ops.delete(op.id);
    if (notice) notice.settle(true);
    else if (policy === 'always') toast.success(opDoneText(op), { id: op.id });
    return true;
  }

  /** A backend op started: chip only, no toast until it settles. */
  function track(input: AdeOpInput): void {
    if (ops.has(input.id)) return;
    ops.set(input.id, { ...input, source: 'backend' });
  }

  /**
   * A backend op ended. A refused MCP call is an answer to the agent, not a user failure, so the
   * toast keys off duration alone: ok or error, only past `TIMING.slowOp` (the approval wait counts), and
   * never for the approval wait itself.
   */
  function settle(id: string, result: { ok: boolean; error?: string; durationMs: number }): void {
    const op = ops.get(id);
    if (!op) return;
    ops.delete(id);
    if (op.kind === 'agentApproval' || result.durationMs < TIMING.slowOp) return;
    if (result.ok) toast.success(opDoneText(op), { id });
    else toast.error(result.error || opLoadingText(op.kind), { id });
  }

  return { ops, forTask, forBranch, forSession, isRunning, run, track, settle };
});
