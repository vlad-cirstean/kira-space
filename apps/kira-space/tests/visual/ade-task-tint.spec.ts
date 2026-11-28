import { expect, reviewTest, test } from '../ui/fixtures';
import { openWindow } from '../ui/support/adeReviewStep';
import { openPlan, openTask } from '../ui/support/adeV2';

// P764: the task colour band. Plan board with every slot (rest), one card hovered, one selected, then
// the details header and the review window title bar on the same band.

const t = (id: string) => `[data-testid="${id}"]`;
const head = (id: string) =>
  `[data-testid="ade-card"][data-task-id="${id}"] [data-testid="ade-card-head"]`;

test('task band: rest, hover and selected card heads', async ({ relaunch }) => {
  const { window } = await openPlan(relaunch);
  await window.setViewportSize({ width: 1400, height: 2400 });
  await window.locator(t('ade-load-all')).click();
  await window.locator(head('T_search')).click();
  await expect(window.locator(`[data-task-id="T_search"][aria-current="true"]`)).toHaveCount(1);
  await window.locator(head('T_auth')).hover();
  await expect(window.locator(t('ade-plan'))).toHaveScreenshot('task-tint-plan.png');
});

test('task band: details header', async ({ relaunch }) => {
  const { window } = await openPlan(relaunch);
  await window.setViewportSize({ width: 1400, height: 900 });
  await openTask(window, 'T_alerts');
  await window.mouse.move(2, 2);
  await expect(window.locator(t('ade-panel-head'))).toHaveScreenshot('task-tint-details.png');
});

reviewTest('task band: review window title bar', async ({ relaunch }) => {
  const { page } = await openWindow(relaunch, 'answered');
  await page.setViewportSize({ width: 1400, height: 900 });
  await expect(page.locator(t('ade-task-band'))).toBeVisible();
  await expect(page.locator(t('ade-task-band'))).toHaveScreenshot('task-tint-review.png');
});
