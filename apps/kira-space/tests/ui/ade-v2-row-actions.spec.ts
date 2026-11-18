import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { adeBoard, adeFixture, FIXED_NOW, openPlan, revealHead } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { pick } from './support/branchMenu';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Plan row actions: each drives a bridge call whose backend scenario a flow test already proves.
// This file is the frontend half; every test names its backend half.

const t = (id: string) => `[data-testid="${id}"]`;
const item = (id: string) => t(`menu-item-${id}`);
const card = (id: string) => `[data-testid="ade-card"][data-task-id="${id}"]`;

const candidates = { channel: IPC.adeTaskCandidateBranches, response: adeFixture('candidates') };

// Backend half: adeflow taskbranches_test.go (SetTaskBranch).
test('the row pencil opens Change branch under itself and sends the existing branch', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [candidates]);
  await pick(page, 'b_cart', 'ade-task-branch-change', 'board');
  const form = page.locator(t('ade-branch-form'));
  await expect(form).toBeVisible();
  const pencil = page.locator(
    `${t('ade-branch-row')}[data-branch-id="b_cart"] ${t('ade-task-branch-menu')}`,
  );
  const [formBox, pencilBox] = await Promise.all([form.boundingBox(), pencil.boundingBox()]);
  expect((formBox?.x ?? 0) + (formBox?.width ?? 0)).toBeGreaterThanOrEqual(
    (pencilBox?.x ?? 0) + (pencilBox?.width ?? 0) - 1,
  );
  await page.locator('[data-testid^="ade-branch-form-option-"]').first().click();
  await expect.poll(() => calls(control, IPC.adeTaskSetTaskBranch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSetTaskBranch)[0]?.args).toMatchObject({
    branchId: 'b_cart',
    existing: true,
    discardChanges: false,
  });
});

// Backend half: adeflow taskbranches_test.go (AddTaskRepo), newtask_test.go (attach).
test('the card add button adds a repo and opens the attach dialog', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [candidates]);
  await revealHead(page, 'T_bill', 'ade-card-add');
  await page.locator(`${card('T_bill')} ${t('ade-card-add')}`).click();
  // PopoverContent flush: no padding or row gap of its own.
  await expect(page.locator(t('ade-card-add-menu'))).toHaveCSS('padding-top', '0px');
  await expect(page.locator(t('ade-card-add-menu'))).toHaveCSS('row-gap', '0px');
  await page.locator(t('ade-card-add-repo-option-repo-mobile')).click();
  await expect.poll(() => calls(control, IPC.adeTaskAddTaskRepo)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddTaskRepo)[0]?.args).toEqual({
    taskId: 'T_bill',
    codeRepoId: 'repo-mobile',
    base: null,
  });
  await revealHead(page, 'T_bill', 'ade-card-add');
  await page.locator(`${card('T_bill')} ${t('ade-card-add')}`).click();
  await page.locator(t('ade-card-add-branch')).click();
  await expect(page.locator(t('ade-add-attach'))).toContainText('Adding to');
});

// Backend half: adeflow newtask_test.go (Est), twowindows_test.go.
test('the estimate chip writes the estimate and the task menu opens the same popover', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await revealHead(page, 'T_notif', 'ade-card-est');
  await page.locator(`${card('T_notif')} ${t('ade-card-est')}`).click();
  const field = page.locator(t('ade-estimate-num'));
  await field.fill('3');
  await field.press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskUpdateTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskUpdateTask)[0]?.args).toMatchObject({
    taskId: 'T_notif',
    patch: { est: '3h' },
  });
  await page.keyboard.press('Escape');
  await expect(field).toHaveCount(0);
  await page.locator(card('T_notif')).click({ button: 'right', position: { x: 60, y: 30 } });
  await page.locator(item('ade-task-estimate')).click();
  await expect(page.locator(t('ade-estimate-num'))).toBeVisible();
});

// Backend half: adeflow stagemove_test.go.
test('the step label opens the step menu; Next opens the stage dialog, a live step is disabled', async ({
  relaunch,
}) => {
  // T_notif's agents finish, so its step can move; T_search keeps a running one.
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        for (const r of b.tasks.find((x) => x.id === 'T_notif')?.runs ?? []) r.state = 'done';
      }),
    },
  ]);
  await page.locator(`${card('T_notif')} ${t('ade-step-label')}`).click();
  const menu = page.locator(t('ade-row-step-menu'));
  await expect(menu).toBeVisible();
  const ids = await menu.locator('[data-testid^="ade-row-step-option-"]').evaluateAll((els) =>
    els.map((e) => ({
      id: (e.getAttribute('data-testid') ?? '').slice('ade-row-step-option-'.length),
      on: e.getAttribute('aria-checked') === 'true',
    })),
  );
  const at = ids.findIndex((o) => o.on);
  expect(at).toBeGreaterThanOrEqual(0);
  await menu.getByRole('menuitem', { name: 'Next step' }).click();
  await expect(page.locator(t('ade-stage-dialog'))).toBeVisible();
  await expect(page.locator(t('ade-stage-target'))).toHaveValue(ids[at + 1]?.id ?? '');
  await page.keyboard.press('Escape');

  await page.locator(`${card('T_search')} ${t('ade-step-label')}`).click();
  const live = page.locator(t('ade-row-step-menu'));
  await expect(live).toContainText('Stop its running agents first');
  await expect(live.getByRole('menuitem', { name: 'Next step' })).toHaveAttribute(
    'data-disabled',
    '',
  );
});

// Backend half: adeflow panelforward_test.go, ids_test.go (Title).
test('a double-click renames the task in place; Escape writes nothing; the task menu opens it', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  const title = page.locator(`${card('T_notif')} ${t('ade-card-title')}`);
  await title.dblclick();
  const input = page.locator(t('ade-card-title-input'));
  await input.fill('Discard me');
  await input.press('Escape');
  await expect(input).toHaveCount(0);
  expect(calls(control, IPC.adeTaskUpdateTask)).toHaveLength(0);
  await title.dblclick();
  await input.fill('Renamed task');
  await input.press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskUpdateTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskUpdateTask)[0]?.args).toMatchObject({
    taskId: 'T_notif',
    patch: { title: 'Renamed task' },
  });
  await page.locator(card('T_notif')).click({ button: 'right', position: { x: 60, y: 30 } });
  await page.locator(item('ade-task-rename')).click();
  await expect(page.locator(t('ade-card-title-input'))).toBeVisible();
});

// Backend half: adeflow workflow_switch_test.go (contract ade-workflow-switch).
test('the card menu switches the workflow; a refusal opens the confirm and Yes resends', async ({
  relaunch,
}) => {
  const refusal = JSON.parse(
    contract<string>('ade-workflow-switch', 'error:AdeTaskService.SetTaskWorkflow#loses-work'),
  ) as { code: string; message: string; details: { runs: number; logs: number } };
  const first = { taskId: 'T_bill', workflowId: 'bugfix' };
  const { runs, logs } = refusal.details;
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskSetTaskWorkflow, args: first, error: refusal },
    {
      channel: IPC.adeTaskSetTaskWorkflow,
      args: { ...first, confirmed: true, confirmedRuns: runs, confirmedLogs: logs },
      response: adeFixture('task'),
    },
  ]);
  await page.locator(`${card('T_bill')} ${t('ade-card-head')}`).click({ button: 'right' });
  await page.locator(item('ade-task-workflow')).hover();
  await page.locator(item('ade-task-workflow-bugfix')).click();
  await expect(page.locator(t('confirm-dialog-message'))).toContainText('runs (1)');
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSetTaskWorkflow)).toHaveLength(2);
  expect(calls(control, IPC.adeTaskSetTaskWorkflow)[1]?.args).toEqual({
    ...first,
    confirmed: true,
    confirmedRuns: runs,
    confirmedLogs: logs,
  });
});

// Backend half: adeflow branchrequest_test.go (contract ade-branch-request).
test('the branch request chip on the card reopens the approvals item', async ({ relaunch }) => {
  const [it] = contract<ApprovalItem[]>('ade-branch-request', 'ApprovalsService.List#branch');
  if (!it) throw new Error('contract has no item');
  const live = liveItem(it, 1800, FIXED_NOW);
  const { window: page } = await openPlan(relaunch, [
    ...seedApprovals([live]),
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        const task = b.tasks.find((x) => x.id === 'T_alerts');
        if (!task) throw new Error('T_alerts missing');
        task.branchRequestId = live.id.slice('branch.request:'.length);
      }),
    },
  ]);
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
  await page.keyboard.press('Shift+Escape');
  await expect(page.locator(t('approvals-dialog'))).toHaveCount(0);
  await page.locator(`${card('T_alerts')} ${t('ade-card-branch-request')}`).click();
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
});

// Backend half: adeflow trackedupdates_test.go (seen). Opening the host view has its own flow.
test('the PR glyph opens the change request and marks its pending update seen', async ({
  relaunch,
}) => {
  const crFx = contract<{ updates: Record<string, unknown>[] }>(
    'ade-tracked-updates',
    'AdeTaskService.TrackedUpdates#changed',
  ).updates.find((u) => u.kind === 'changeRequest');
  if (!crFx) throw new Error('contract lacks a change request update');
  const update = { ...crFx, taskId: 'T_cart', itemKey: 'cr:b_cart', branchId: 'b_cart' };
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskTrackedUpdates, response: { updates: [update] } },
    { channel: IPC.adeTaskMarkTrackedSeen, hold: true },
    { channel: IPC.hostViewOpen, response: { key: 'hostview-1', opened: true } },
  ]);
  await page
    .locator(`${t('ade-branch-row')}[data-branch-id="b_cart"] ${t('ade-branch-pr')}`)
    .click();
  await expect.poll(() => calls(control, IPC.hostViewOpen)).toHaveLength(1);
  expect(calls(control, IPC.hostViewOpen)[0]?.args).toEqual({
    url: 'https://github.com/acme/web-app/pull/1431',
  });
  await expect.poll(() => calls(control, IPC.adeTaskMarkTrackedSeen)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskMarkTrackedSeen)[0]?.args).toEqual({
    taskId: 'T_cart',
    itemKeys: ['cr:b_cart'],
  });
});
