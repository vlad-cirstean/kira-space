import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { errorText } from '@workbench/bridge/codedError';
import type { NeedsItem } from '../board/needsYou';
import { usePlanModel } from '../plan/usePlanModel';
import { useTaskWrites } from '../plan/useTaskWrites';
import { useReviewCode } from '../review/useReviewCode';
import { useOpenSession } from '../sessions/useOpenSession';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import { useAdeOpsStore } from '../state/adeOps';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
import { useRepoGate } from '../state/useRepoGate';

const errMessage = (err: unknown): string => errorText(err);

/** The one action behind a needs-you item: the Needs you row button and the `!` circle share it. */
export function useNeedsAction() {
  const ui = useAdeBoardUiStore();
  const tabs = useAdePanelTabsStore();
  const dialogs = useAdeDialogsStore();
  const takeOver = useAdeTakeOverStore();
  const openSession = useOpenSession();
  const { retry, resume, approve } = useTaskWrites();
  const ops = useAdeOpsStore();
  const reviewCode = useReviewCode();
  const { gated } = useRepoGate();
  const { model } = usePlanModel();

  /** The item's task action as an op: chip on the card, toast on failure, task selected. */
  async function op(
    kind: 'retry' | 'resume' | 'approve',
    n: NeedsItem,
    call: () => Promise<unknown>,
  ): Promise<void> {
    const name = model.value?.cardFor(n.taskId)?.title ?? '';
    const ok = await ops.run({ id: `${kind}:${n.taskId}`, kind, taskId: n.taskId, name }, call);
    if (!ok) ui.select(n.taskId);
  }

  async function run(n: NeedsItem): Promise<void> {
    delete ui.actionError[n.taskId];
    try {
      if (n.kind === 'rebase') {
        ui.view = 'plan';
        ui.selectBranch(n.taskId, n.branchId);
        tabs.setBranchTab(n.branchId, 'details');
      } else if (n.action === 'Take over') await takeOver.request(n.sessionId);
      else if (n.action === 'Retry') {
        await op('retry', n, () =>
          Promise.all(n.runIds.map((runId) => retry.mutateAsync({ runId }))),
        );
      } else if (n.action === 'Resume') {
        await op('resume', n, () =>
          Promise.all(n.runIds.map((runId) => resume.mutateAsync({ runId }))),
        );
      } else if (n.action === 'Open') await openSession(n.sessionId, n.taskId, n.branchId);
      else if (n.action === 'Approve') {
        const needsRepo = model.value?.cardFor(n.taskId)?.progress.step?.needsRepo ?? false;
        await gated(n.taskId, needsRepo, () =>
          op('approve', n, () => approve.mutateAsync({ taskId: n.taskId, stepId: n.stepId })),
        );
      } else if (n.action === 'Review') {
        reviewCode.open(n.taskId, n.branchId, null);
      } else if (n.action === 'See error') {
        ui.view = 'plan';
        ui.selectBranch(n.taskId, n.branchId);
        tabs.setBranchTab(n.branchId, 'details');
        ui.focusSetup = true;
      } else dialogs.merge(n.branchId, n.target);
    } catch (err) {
      ui.actionError[n.taskId] = errMessage(err);
      ui.select(n.taskId);
    }
  }

  /** True while an action is in flight: a repeat click must not run it twice. */
  const guarded = useAsyncAction(run);
  const busy = guarded.pending;
  async function perform(n: NeedsItem): Promise<void> {
    if (!busy.value) await guarded.run(n);
  }

  return { perform, openSession, busy };
}
