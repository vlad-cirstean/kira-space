import { expect, test } from './fixtures';
import { openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Panel edit forward: pending task panel edits offered to the agents running in the step. Contract
// ade-panel-forward; backend half: flows/adeflow TestPanelForward (panelforward_test.go).

const t = (id: string) => `[data-testid="${id}"]`;
const TASK = 'T_alerts';

interface OfferFx {
  stepId: string;
  stepName: string;
  items: { key: string; kind: string; label: string; at: number }[];
  targets: { sessionId: string; badge: string }[];
  unreachable: number;
  dropped: number;
  preview: string;
}

const pending = (): OfferFx =>
  contract<OfferFx>('ade-panel-forward', 'AdeTaskService.PanelForward#pending');
const headless = (): OfferFx =>
  contract<OfferFx>('ade-panel-forward', 'AdeTaskService.PanelForward#headless');
const EMPTY: OfferFx = {
  stepId: '',
  stepName: '',
  items: [],
  targets: [],
  unreachable: 0,
  dropped: 0,
  preview: '',
};

test('no pending edit: no forward bar', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, TASK);
  await expect(page.locator(t('ade-panel-forward-bar'))).toHaveCount(0);
});

test('contract: the bar names the step, lists each change and previews the exact message', async ({
  relaunch,
}) => {
  const fx = pending();
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskPanelForward, response: fx },
  ]);
  await openTask(page, TASK);
  const bar = page.locator(t('ade-panel-forward-bar'));
  await expect(bar).toBeVisible();
  await expect(bar.locator(t('ade-panel-forward-text'))).toContainText('3 panel changes');
  await expect(bar.locator(t('ade-panel-forward-text'))).toContainText(fx.stepName);
  for (const it of fx.items) {
    await expect(bar.locator(t('ade-panel-forward-items'))).toContainText(it.label);
  }
  await expect(bar.locator(t('ade-panel-forward'))).toContainText('Send to 1 agent');
  await expect(bar.locator(t('ade-panel-forward-preview'))).toBeHidden();
  await bar.locator(t('ade-panel-forward-toggle')).click();
  await expect(bar.locator(t('ade-panel-forward-preview'))).toHaveText(fx.preview);
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(bar).toBeVisible();
});

test('contract: Forward sends ForwardPanelEdits with the task and the bar hides', async ({
  relaunch,
}) => {
  const sent = contract<{ sent: string[]; ended: string[]; failed: unknown[] }>(
    'ade-panel-forward',
    'AdeTaskService.ForwardPanelEdits',
  );
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskPanelForward, response: pending() },
    { channel: IPC.adeTaskForwardPanelEdits, response: sent },
  ]);
  await openTask(page, TASK);
  control.respond(IPC.adeTaskPanelForward, EMPTY);
  await page.locator(t('ade-panel-forward')).click();
  await expect.poll(() => calls(control, IPC.adeTaskForwardPanelEdits)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskForwardPanelEdits)[0]?.args).toEqual({ taskId: TASK });
  await expect(page.locator(t('ade-panel-forward-bar'))).toHaveCount(0);
  await expect(page.locator(t('ade-panel-forward-failed'))).toHaveCount(0);
});

test('a partial failure names the session that did not take the message', async ({ relaunch }) => {
  const fx = pending();
  const target = fx.targets[0];
  if (!target) throw new Error('contract offer has no target');
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskPanelForward, response: fx },
    {
      channel: IPC.adeTaskForwardPanelEdits,
      response: {
        sent: ['other'],
        ended: [],
        failed: [{ sessionId: target.sessionId, error: 'terminal closed' }],
      },
    },
  ]);
  await openTask(page, TASK);
  await page.locator(t('ade-panel-forward')).click();
  const alert = page.locator(t('ade-panel-forward-failed'));
  await expect(alert).toContainText(target.badge);
  await expect(alert).toContainText('terminal closed');
});

test('only background runs: Forward is disabled and the tooltip says to take over', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskPanelForward, response: headless() },
  ]);
  await openTask(page, TASK);
  const send = page.locator(t('ade-panel-forward'));
  await expect(send).toBeDisabled();
  await send.locator('xpath=..').hover();
  await expect(page.getByRole('tooltip')).toContainText('take over a run to send');
  expect(calls(control, IPC.adeTaskForwardPanelEdits)).toHaveLength(0);
});

test('Dismiss sends DismissPanelEdits and the bar hides', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskPanelForward, response: pending() },
  ]);
  await openTask(page, TASK);
  control.respond(IPC.adeTaskPanelForward, EMPTY);
  await page.locator(t('ade-panel-forward-dismiss')).click();
  await expect.poll(() => calls(control, IPC.adeTaskDismissPanelEdits)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskDismissPanelEdits)[0]?.args).toEqual({ taskId: TASK });
  await expect(page.locator(t('ade-panel-forward-bar'))).toHaveCount(0);
});

test('a saved change shows the bar after the board push refetches the offer', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, TASK);
  await expect(page.locator(t('ade-panel-forward-bar'))).toHaveCount(0);
  control.respond(IPC.adeTaskPanelForward, pending());
  const title = page.locator(t('ade-panel-title'));
  await title.fill('Renamed task');
  await title.blur();
  await expect(page.locator(t('ade-panel-forward-bar'))).toBeVisible();
});
