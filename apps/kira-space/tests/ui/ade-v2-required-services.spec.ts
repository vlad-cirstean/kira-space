import type { Page } from '@playwright/test';
import { runPreviewSnapshots } from './adePromptPreview';
import { expect, test } from './fixtures';
import { adeBoard, adeFixture, alertsInPlan, openPlan, PLAIN, SCRIPTS } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Starting work asks about required services that are not running. Contract ade-required-services.
// Backend half: adeflow TestRequiredServices.

const t = (id: string) => `[data-testid="${id}"]`;
const task = (id: string) => `[data-testid="ade-task"][data-task-id="${id}"]`;
const cellAction = (id: string, kind: string) =>
  `${task(id)} ${t('ade-card-head')} ${t(`ade-task-action-${kind}`)}`;

const SCENARIO = 'ade-required-services';
const stopped = contract<unknown>(SCENARIO, 'AdeTaskService.RequiredServices#stopped');
const running = contract<unknown>(SCENARIO, 'AdeTaskService.RequiredServices#running');

// The contract's ids are normalised; map them onto the fixture board's.
const IDS: Record<string, string> = { '<id:1>': 'repo-api', '<id:2>': 'd_alerts_api' };
const remap = (answer: unknown): unknown =>
  JSON.parse(JSON.stringify(answer), (_k, v) => (typeof v === 'string' ? (IDS[v] ?? v) : v));

const answer = (a: unknown): ControlSnapshot => ({
  channel: IPC.adeTaskRequiredServices,
  response: remap(a),
});

const DEV_DB = { ...PLAIN, id: 'dev-db', name: 'Dev DB' };
const scripts: ControlSnapshot = {
  ...SCRIPTS,
  response: {
    ...(SCRIPTS.response as Record<string, unknown>),
    scripts: [PLAIN, DEV_DB],
  },
};

const planBoard = (): ControlSnapshot[] => [
  { channel: IPC.adeTaskBoard, response: adeBoard(alertsInPlan) },
  ...runPreviewSnapshots(),
];

const gate = (page: Page) => page.locator(t('ade-services-gate'));

test('Run asks about stopped services and the X starts nothing', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    ...planBoard(),
    scripts,
    answer(stopped),
  ]);
  await page.locator(cellAction('T_alerts', 'run')).click();
  await expect(gate(page)).toBeVisible();
  const rows = gate(page).locator(t('ade-services-gate-row'));
  await expect(rows).toHaveCount(3);
  await expect(rows.nth(0)).toHaveAttribute('data-state', 'stopped');
  await expect(rows.nth(2)).toHaveAttribute('data-state', 'invalid');
  await expect(gate(page).locator(t('ade-services-gate-run'))).toHaveCount(2);
  await page.keyboard.press('Escape');
  await expect(gate(page)).toBeHidden();
  await expect(page.locator(t('ade-run-dialog'))).toHaveCount(0);
  expect(calls(control, IPC.adeTaskStartRun)).toHaveLength(0);
});

test('Continue goes on to the Run dialog and the task is not asked again', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    ...planBoard(),
    scripts,
    answer(stopped),
  ]);
  await page.locator(cellAction('T_alerts', 'run')).click();
  await gate(page).locator(t('ade-services-gate-continue')).click();
  await expect(gate(page)).toBeHidden();
  const dialog = page.locator(t('ade-run-dialog'));
  await expect(dialog).toBeVisible();
  await dialog.locator(t('ade-run-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStartRun)).toHaveLength(1);
  const asked = calls(control, IPC.adeTaskRequiredServices).length;

  await page.locator(cellAction('T_alerts', 'run')).click();
  await expect(dialog).toBeVisible();
  await expect(gate(page)).toHaveCount(0);
  expect(calls(control, IPC.adeTaskRequiredServices)).toHaveLength(asked);
});

test('running services go straight to the Run dialog', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [...planBoard(), scripts, answer(running)]);
  await page.locator(cellAction('T_alerts', 'run')).click();
  await expect(page.locator(t('ade-run-dialog'))).toBeVisible();
  await expect(gate(page)).toHaveCount(0);
});

test('a row Run closes the ask and opens the script run dialog for the task', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    ...planBoard(),
    scripts,
    answer(stopped),
  ]);
  await page.locator(cellAction('T_alerts', 'run')).click();
  await gate(page).locator(t('ade-services-gate-run')).first().click();
  await expect(gate(page)).toBeHidden();
  await expect(page.locator(t('run-dialog'))).toBeVisible();
  await expect
    .poll(() => (calls(control, IPC.scriptRunsPreview).at(-1)?.args as { taskId?: string })?.taskId)
    .toBe('T_alerts');
  expect(calls(control, IPC.adeTaskStartRun)).toHaveLength(0);
});

test('a branch Start asks too', async ({ relaunch }) => {
  const sessions = adeFixture<{ sessions: { branchId: string }[] }>('sessions');
  const { window: page, control } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskSessions,
      response: {
        ...sessions,
        sessions: sessions.sessions.filter((s) => s.branchId !== 'b_authui'),
      },
    },
    scripts,
    answer(stopped),
  ]);
  await page.locator('[data-testid="ade-branch-row"][data-branch-id="b_authui"]').click();
  await page.locator(t('ade-panel-action-start')).click();
  await expect(gate(page)).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(gate(page)).toBeHidden();
  await expect(page.locator(t('ade-dialog'))).toHaveCount(0);
  expect(calls(control, IPC.adeTaskStartBranch)).toHaveLength(0);
});
