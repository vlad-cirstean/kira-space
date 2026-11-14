import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import type { ContractJson } from '@workbench/testing/ui/contract';
import { expect, test } from './fixtures';
import { adeBoard, adeFixture, FIXED_NOW, openPlan } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P637: desktop follows a phone's answer. The queue pushes its list and the board shows the child
// the approval created. Contract mobile-approvals; backend half: flows/mobileflow TestPhoneApprovals
// (approvals_test.go); the phone half is tests/mobile/approvals.spec.ts.

const t = (id: string) => `[data-testid="${id}"]`;
const ORCH = 'T_orch';
const CHILD = 'T_child';

type Json = ContractJson;

/** The board the desktop holds once the phone approved: the orchestrator with its new child. */
function afterPhoneApprove() {
  const fx = contract<{ tasks: Json[] }>(
    'mobile-approvals',
    'AdeTaskService.Board#afterPhoneApprove',
  );
  const base = adeFixture<{ statuses: Json[]; repos: Json[] }>('board');
  return adeBoard((board) => {
    const [orch, child] = fx.tasks as [Json, Json];
    orch.id = ORCH;
    for (const r of orch.runs) Object.assign(r, { id: 'run_orch', taskId: ORCH, stepId: 'plan' });
    child.id = CHILD;
    child.parentId = ORCH;
    child.handoverIn = { ...child.handoverIn, taskId: CHILD, fromTaskId: ORCH };
    for (const task of [orch, child]) {
      task.createdAt = FIXED_NOW - 864e5;
      task.branchIds = [];
    }
    board.tasks = [orch, child] as typeof board.tasks;
    board.statuses = base.statuses as typeof board.statuses;
    board.repos = base.repos;
    board.branches = [];
    board.history = [];
    board.plan = {
      backlog: [],
      day: { [ORCH]: '2026-09-22', [CHILD]: '2026-09-22' },
      order: [ORCH, CHILD],
      queuedAfter: {},
      unpushed: {},
    };
  });
}

function pendingTask(): ApprovalItem {
  const it = contract<ApprovalItem[]>('mobile-approvals', 'http:GET /api/approvals#pending').find(
    (i) => i.kind === 'task.create',
  );
  if (!it) throw new Error('contract has no task.create item');
  return liveItem(it, 1800, FIXED_NOW);
}

test('contract: the dialog and toast go when the phone approves and the child is on the board', async ({
  relaunch,
}) => {
  const it = pendingTask();
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: afterPhoneApprove() },
    ...seedApprovals([it]),
  ]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog).toBeVisible();
  await expect(dialog.locator(t('approval-target-name'))).toHaveText('API login');

  await pushApprovals(
    page,
    contract<ApprovalItem[]>('mobile-approvals', 'event:kira:approvals:changed#afterPhoneApprove'),
  );
  await expect(dialog).toBeHidden();
  await expect(page.locator(t('approvals-toast'))).toHaveCount(0);

  const orch = page.locator(`${t('ade-card')}[data-task-id="${ORCH}"]`);
  await expect(orch.locator(t('ade-card-children'))).toBeVisible();
  await expect(orch.locator(`${t('ade-card-child')}[data-task-id="${CHILD}"]`)).toContainText(
    'API login',
  );
});
