import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// The task context menu: right-click, Shift+F10 and the panel's More button.

const t = (id: string) => `[data-testid="${id}"]`;
const card = (id: string) => `[data-testid="ade-card"][data-task-id="${id}"]`;
const item = (id: string) => t(`menu-item-${id}`);

async function menuIds(page: Page): Promise<string[]> {
  return page
    .locator(`${t('context-menu')} [data-testid^="menu-item-"]`)
    .evaluateAll((els) => els.map((e) => (e.getAttribute('data-testid') ?? '').slice(10)));
}

test('right-click moves the task to another step', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await page.locator(card('T_alerts')).click({ button: 'right', position: { x: 60, y: 30 } });
  await expect(page.locator(t('ade-panel'))).toBeVisible();
  await page.locator(item('ade-task-step')).click();
  await expect(page.locator(item('ade-task-step-spec'))).toBeVisible();
  await page.locator(item('ade-task-step-review')).click();
  await page.locator(t('ade-stage-move')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSetTaskStep)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSetTaskStep)[0]?.args).toMatchObject({
    taskId: 'T_alerts',
    stepId: 'review',
  });
});

test('Mark as not merging, Copy task id and Archive call their backends', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await page.evaluate(() => {
    const w = window as unknown as { __clipboard: string[] };
    w.__clipboard = [];
    navigator.clipboard.writeText = async (text: string) => {
      w.__clipboard.push(text);
    };
  });
  const open = () =>
    page.locator(card('T_alerts')).click({ button: 'right', position: { x: 60, y: 30 } });

  await open();
  await page.locator(item('ade-task-park')).click();
  await expect.poll(() => calls(control, IPC.adeTaskUpdateTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskUpdateTask)[0]?.args).toMatchObject({
    taskId: 'T_alerts',
    patch: { kind: 'parked' },
  });

  await open();
  await page.locator(item('ade-task-copy')).click();
  await page.locator(item('ade-task-copy-id')).click();
  await expect
    .poll(() => page.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard))
    .toEqual(['T_alerts']);

  await open();
  await page.locator(item('ade-task-archive')).click();
  await expect.poll(() => calls(control, IPC.adeTaskArchiveRisk)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskArchiveRisk)[0]?.args).toEqual({ taskId: 'T_alerts' });
});

test('a review item gets the reduced set', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(t('ade-load-all')).click();
  await page.locator(card('R_sara')).scrollIntoViewIfNeeded();
  await page
    .locator(`[data-testid="ade-task"][data-task-id="R_sara"] ${t('ade-tag')}`)
    .first()
    .click({ button: 'right' });
  await expect(page.locator(t('context-menu'))).toBeVisible();
  const ids = await menuIds(page);
  expect(ids).toContain('ade-task-sessions');
  expect(ids).toContain('ade-task-archive');
  expect(ids).not.toContain('ade-task-step');
  expect(ids).not.toContain('ade-task-park');
  expect(ids).toContain('ade-task-plan');
  expect(ids).toContain('ade-task-make-task');
});

interface Converted {
  kind: string;
  owner: string;
}

// Contract ade-review-convert, backend half: adeflow TestReviewConvert.
test('contract: Convert to work item sends the backend patch', async ({ relaunch }) => {
  const args = contract<{ patch: unknown }>('ade-review-convert', 'args:AdeTaskService.UpdateTask');
  const { window: page, control } = await openPlan(relaunch);
  await page.locator(t('ade-load-all')).click();
  await page.locator(card('R_sara')).scrollIntoViewIfNeeded();
  await page
    .locator(`${card('R_sara')} [data-testid="ade-branch-row"][data-branch-id="b_sara"]`)
    .click({ button: 'right' });
  await expect(page.locator(item('ade-task-review'))).toBeVisible();
  await page.locator(item('ade-task-make-task')).click();
  await expect.poll(() => calls(control, IPC.adeTaskUpdateTask)).toHaveLength(1);
  const sent = calls(control, IPC.adeTaskUpdateTask)[0]?.args;
  expect(sent).toMatchObject({ taskId: 'R_sara' });
  expect((sent as { patch: unknown }).patch).toEqual(args.patch);
});

test('contract: a converted review renders as a normal task', async ({ relaunch }) => {
  const task = contract<Converted>('ade-review-convert', 'AdeTaskService.UpdateTask#converted');
  const branch = contract<{ kind: string }>(
    'ade-review-convert',
    'AdeTaskService.Branch#converted',
  );
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        const tk = b.tasks.find((x) => x.id === 'R_sara');
        const br = b.branches.find((x) => x.id === 'b_sara');
        if (!tk || !br) throw new Error('fixture review missing');
        Object.assign(tk, { kind: task.kind, owner: task.owner });
        Object.assign(br, { kind: branch.kind });
        // An undated task sits on the Later tab, so date the converted one to keep it on the Plan.
        (b.plan as { day: Record<string, string> }).day.R_sara = '2026-09-25';
      }),
    },
  ]);
  await page.locator(t('ade-load-all')).click();
  await page.locator(card('R_sara')).scrollIntoViewIfNeeded();
  const head = page.locator(`${card('R_sara')} ${t('ade-card-head')}`);
  await expect(head).toBeVisible();
  await head.click({ button: 'right' });
  await expect(page.locator(item('ade-task-park'))).toBeVisible();
  await expect(page.locator(item('ade-task-make-task'))).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(page.locator(t('ade-panel-review-note'))).toHaveCount(0);
});

test('Shift+F10 on the card head and the panel More button open it', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(`${card('T_bill')} ${t('ade-card-head')}`).focus();
  await page.keyboard.press('Shift+F10');
  await expect(page.locator(item('ade-task-sessions'))).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(t('context-menu'))).toHaveCount(0);

  await page.keyboard.press('ContextMenu');
  await expect(page.locator(item('ade-task-sessions'))).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(t('context-menu'))).toHaveCount(0);

  await page.locator(t('ade-panel-more')).click();
  await expect(page.locator(item('ade-task-sessions'))).toBeVisible();
});
