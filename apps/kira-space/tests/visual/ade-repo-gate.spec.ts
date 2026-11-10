import { expect, test } from '../ui/fixtures';
import { adeBoard, openPlan } from '../ui/support/adeV2';
import { IPC } from '../ui/support/ipcChannels';

// P598: the repo choice before a start, with one repo chosen.

test('repo choice before start', async ({ relaunch }) => {
  const board = adeBoard((b) => {
    const alerts = b.tasks.find((x) => x.id === 'T_alerts');
    if (!alerts) throw new Error('fixture task missing');
    alerts.entryStepId = 'plan';
    alerts.workflowId = 'standard';
    alerts.branchIds = [];
    alerts.runs = [];
    b.branches = b.branches.filter((x) => x.taskId !== 'T_alerts');
  });
  const { window } = await openPlan(relaunch, [{ channel: IPC.adeTaskBoard, response: board }]);
  await window.setViewportSize({ width: 1400, height: 900 });
  await window
    .locator(
      '[data-testid="ade-task"][data-task-id="T_alerts"] [data-testid="ade-card-head"] [data-testid="ade-task-action-run"]',
    )
    .click();
  await window.locator('[data-testid="ade-repo-gate-pick"]').click();
  await window.locator('[data-testid="ade-repo-gate-pick-option-repo-api"]').click();
  await expect(window.locator('[data-testid="ade-repo-gate-chip"]')).toHaveCount(1);
  await expect(window.locator('[data-testid="ade-repo-gate"]')).toHaveScreenshot('repo-gate.png');
});
