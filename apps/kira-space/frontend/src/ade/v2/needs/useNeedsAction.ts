import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { errorText } from '@workbench/bridge/codedError';
import type { NeedsItem } from '../board/needsYou';
import { useApprove, useResumeRun, useRetryRun } from '../queries';
import { useReviewCode } from '../review/useReviewCode';
import { useOpenSession } from '../sessions/useOpenSession';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import { useAdeTakeOverStore } from '../state/adeTakeOver';

const errMessage = (err: unknown): string => errorText(err);

/** The one action behind a needs-you item: the Needs you row button and the `!` circle share it. */
export function useNeedsAction() {
  const ui = useAdeBoardUiStore();
  const tabs = useAdePanelTabsStore();
  const dialogs = useAdeDialogsStore();
  const takeOver = useAdeTakeOverStore();
  const openSession = useOpenSession();
  const retry = useRetryRun();
  const resume = useResumeRun();
  const approve = useApprove();
  const reviewCode = useReviewCode();

  async function run(n: NeedsItem): Promise<void> {
    delete ui.actionError[n.taskId];
    try {
      if (n.kind === 'rebase') {
        ui.view = 'plan';
        ui.selectBranch(n.taskId, n.branchId);
        tabs.setBranchTab(n.branchId, 'details');
      } else if (n.action === 'Take over') await takeOver.request(n.sessionId);
      else if (n.action === 'Retry') {
        await Promise.all(n.runIds.map((runId) => retry.mutateAsync({ runId })));
      } else if (n.action === 'Resume') {
        await Promise.all(n.runIds.map((runId) => resume.mutateAsync({ runId })));
      } else if (n.action === 'Open') await openSession(n.sessionId, n.taskId, n.branchId);
      else if (n.action === 'Approve') {
        await approve.mutateAsync({ taskId: n.taskId, stepId: n.stepId });
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
