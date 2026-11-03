import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// Review code: one step from the Plan to a branch's review window.

const t = (id: string) => `[data-testid="${id}"]`;
const card = (id: string) => `[data-testid="ade-card"][data-task-id="${id}"]`;
const row = (id: string) => `[data-testid="ade-branch-row"][data-branch-id="${id}"]`;
const item = (id: string) => t(`menu-item-${id}`);
const tip = '[data-slot="tooltip-content"]';
const OPEN: ControlSnapshot = { channel: IPC.adeTaskOpenReviewWindow, response: true };

function calls(control: { log(): { channel: string; args?: unknown }[] }) {
  return control.log().filter((e) => e.channel === IPC.adeTaskOpenReviewWindow);
}

// The Go menu accelerator pushes this event; no window keydown remains.
async function pressReview(page: Page): Promise<void> {
  await emitWailsEvent(page, IPC.reviewCode, null);
}

async function pickerIds(page: Page): Promise<string[]> {
  return page
    .locator(`${t('context-menu')} [data-testid^="menu-item-ade-review-pick-"]`)
    .evaluateAll((els) => els.map((e) => (e.getAttribute('data-testid') ?? '').slice(25)));
}

test('the card button opens the only branch and previews branch and base', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [OPEN]);
  const btn = page.locator(`${card('T_deps')} ${t('ade-card-review')}`);
  await btn.hover();
  const tooltip = page.locator(tip);
  await expect(tooltip.locator('[data-var="branch"]')).toHaveText('chore/deps-bump');
  await expect(tooltip.locator('[data-var="base"]')).toHaveText('main');
  await expect(tooltip).toContainText('An agent is still working');
  await btn.click();
  await expect.poll(() => calls(control)).toHaveLength(1);
  expect(calls(control)[0]?.args).toEqual({ branchId: 'b_deps' });
});

test('a task with several branches shows a picker', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [OPEN]);
  const btn = page.locator(`${card('T_bill')} ${t('ade-card-review')}`);
  await btn.click();
  expect(await pickerIds(page)).toHaveLength(3);
  await page.locator(item('ade-review-pick-b_billdash')).click();
  await expect.poll(() => calls(control)).toHaveLength(1);
  expect(calls(control)[0]?.args).toEqual({ branchId: 'b_billdash' });

  await btn.click();
  await expect(page.locator(t('context-menu'))).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(t('context-menu'))).toHaveCount(0);
  await expect(btn).toBeFocused();
  expect(calls(control)).toHaveLength(1);
});

test('the picker lists a branch with no commits as disabled with the reason', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [OPEN]);
  await page.locator(`${card('T_search')} ${t('ade-card-review')}`).click();
  const none = page.locator(item('ade-review-pick-b_searchui'));
  await expect(none).toHaveAttribute('data-disabled', '');
  await none.hover();
  const tooltip = page.locator(tip);
  await expect(tooltip).toContainText('No commits on top of');
  await expect(tooltip.locator('[data-var="base"]')).toHaveText('feat/search-index');
  await page.mouse.move(0, 0);
  await expect(tooltip).toHaveCount(0);
  await page.locator(item('ade-review-pick-b_search')).click();
  await expect.poll(() => calls(control)).toHaveLength(1);
  expect(calls(control)[0]?.args).toEqual({ branchId: 'b_search' });
});

test('an uncreated branch is disabled in the button and the menu', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [OPEN]);
  const btn = page.locator(`${card('T_alerts')} ${t('ade-card-review')}`);
  await expect(btn).toBeDisabled();
  await page.locator(card('T_alerts')).click({ button: 'right', position: { x: 60, y: 30 } });
  await page.locator(item('ade-task-review')).hover();
  await page.locator(item('ade-task-review-d_alerts_api')).hover();
  await expect(page.locator(item('ade-task-review-d_alerts_api'))).toHaveAttribute(
    'data-disabled',
    '',
  );
  await expect(page.locator(tip)).toContainText('Create the branch first');
  expect(calls(control)).toHaveLength(0);
});

test('a parked task has no Review code', async ({ relaunch }) => {
  // A parked task with no day sits on the Later tab, so date it to keep it on the Plan.
  const board = adeBoard((b) => {
    (b.plan as { day: Record<string, string> }).day.T_spike = '2026-09-28';
  });
  const { window: page } = await openPlan(relaunch, [
    OPEN,
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator(t('ade-load-all')).click();
  await page.locator(card('T_spike')).scrollIntoViewIfNeeded();
  await expect(page.locator(`${card('T_spike')} ${t('ade-card-review')}`)).toHaveCount(0);
  await page.locator(card('T_spike')).click({ button: 'right', position: { x: 60, y: 30 } });
  await expect(page.locator(t('context-menu'))).toBeVisible();
  await expect(page.locator(item('ade-task-review'))).toHaveCount(0);
});

test('a review card opens from its menu and its row', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [OPEN]);
  await page.locator(t('ade-load-all')).click();
  await page.locator(card('R_sara')).scrollIntoViewIfNeeded();
  await page
    .locator(`[data-testid="ade-task"][data-task-id="R_sara"] ${t('ade-tag')}`)
    .first()
    .click({ button: 'right' });
  await expect(page.locator(item('ade-task-review-shortcut'))).toBeVisible();
  await page.locator(item('ade-task-review')).click();
  await expect.poll(() => calls(control)).toHaveLength(1);
  expect(calls(control)[0]?.args).toEqual({ branchId: 'b_sara' });

  await page.locator(row('b_sara')).hover();
  await page.locator(`${row('b_sara')} ${t('ade-branch-review')}`).click();
  await expect.poll(() => calls(control)).toHaveLength(2);
  expect(calls(control)[1]?.args).toEqual({ branchId: 'b_sara' });
});

test('Cmd/Ctrl+Shift+R reviews the focused or selected target', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [OPEN]);
  const head = page.locator(`${card('T_deps')} ${t('ade-card-head')}`);
  await head.click();
  await pressReview(page);
  await expect.poll(() => calls(control)).toHaveLength(1);
  expect(calls(control)[0]?.args).toEqual({ branchId: 'b_deps' });
  await expect(head).toBeFocused();

  await page.locator(row('b_billdash')).click();
  await pressReview(page);
  await expect.poll(() => calls(control)).toHaveLength(2);
  expect(calls(control)[1]?.args).toEqual({ branchId: 'b_billdash' });
  await expect(page.locator(t('context-menu'))).toHaveCount(0);

  await page.locator(`${card('T_bill')} ${t('ade-card-head')}`).click();
  await pressReview(page);
  expect(await pickerIds(page)).toHaveLength(3);
});

test('the task panel header opens the review', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [OPEN]);
  await page.locator(`${card('T_deps')} ${t('ade-card-head')}`).click();
  await expect(page.locator(t('ade-panel-review')).locator('.codicon-git-compare')).toBeVisible();
  await page.locator(t('ade-panel-review')).click();
  await expect.poll(() => calls(control)).toHaveLength(1);
  expect(calls(control)[0]?.args).toEqual({ branchId: 'b_deps' });
});

test('a branch with only uncommitted changes is disabled and says why', async ({ relaunch }) => {
  const board = adeBoard((b) => {
    const br = b.branches.find((x) => x.id === 'b_deps');
    if (br) Object.assign(br, { ahead: 0, dirty: [{ code: 'M', path: 'a.ts' }] });
  });
  const { window: page, control } = await openPlan(relaunch, [
    OPEN,
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  const btn = page.locator(`${card('T_deps')} ${t('ade-card-review')}`);
  await expect(btn).toBeDisabled();
  await page.locator(`${card('T_deps')} ${t('ade-card-head')}`).click();
  await page.locator(t('ade-panel-review')).locator('xpath=..').hover();
  const tooltip = page.locator(tip);
  await expect(tooltip).toContainText('Only uncommitted changes on');
  await expect(tooltip.locator('[data-var="branch"]')).toHaveText('chore/deps-bump');
  await expect(tooltip.locator('[data-var="base"]')).toHaveText('main');
  await pressReview(page);
  expect(calls(control)).toHaveLength(0);
});

test('an open failure shows in the panel and the step block', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskOpenReviewWindow, error: { code: 'failed', message: 'no window' } },
  ]);
  await page.locator(`${card('T_deps')} ${t('ade-card-review')}`).click();
  await expect(page.locator(t('ade-panel-action-error'))).toContainText('no window');

  await page.locator(`${card('T_auth')} ${t('ade-card-head')}`).click();
  await page
    .locator(`${t('ade-review-row')}[data-branch-id="b_auth"] ${t('ade-review-code')}`)
    .click();
  await expect(page.locator(t('ade-step-block-error'))).toContainText('no window');
});

test('the earlier entry points still open their branch', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [OPEN]);
  await page.locator(row('b_deps')).click({ button: 'right' });
  await page.locator(item('ade-review-code')).click();
  await expect.poll(() => calls(control)).toHaveLength(1);
  expect(calls(control)[0]?.args).toEqual({ branchId: 'b_deps' });

  await page.locator(row('b_deps')).click();
  await page.locator(t('ade-panel-action-review')).click();
  await expect.poll(() => calls(control)).toHaveLength(2);
  expect(calls(control)[1]?.args).toEqual({ branchId: 'b_deps' });

  await page.locator(`${card('T_auth')} ${t('ade-card-head')}`).click();
  await page
    .locator(`${t('ade-review-row')}[data-branch-id="b_auth"] ${t('ade-review-code')}`)
    .click();
  await expect.poll(() => calls(control)).toHaveLength(3);
  expect(calls(control)[2]?.args).toEqual({ branchId: 'b_auth' });
});

// Contract ade-review-open, backend half: adeflow TestReviewWindowAndAgent. A press on an open
// window answers false: the app only focuses it (P354).
test('Review code on an open review window only focuses it', async ({ relaunch }) => {
  const existing = contract<boolean>('ade-review-open', 'AdeTaskService.OpenReviewWindow#existing');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskOpenReviewWindow, response: existing },
  ]);
  const btn = page.locator(`${card('T_deps')} ${t('ade-card-review')}`);
  await btn.click();
  await btn.click();
  await expect.poll(() => calls(control)).toHaveLength(2);
  for (const c of calls(control)) expect(c.args).toEqual({ branchId: 'b_deps' });
  await expect(page.locator(t('ade-panel-action-error'))).toHaveCount(0);
  await expect(page.locator(card('T_deps'))).toBeVisible();
});

// Contract ade-review-open. Backend half: adeflow TestReviewOpenFacts. The fixture branch b_deps
// takes the backend's ahead, dirty and base facts; its identity stays the fixture's.
for (const phase of ['uncommitted', 'committed'] as const) {
  test(`contract: Review code with ${phase} changes`, async ({ relaunch }) => {
    const facts = contract<{
      name: string;
      ahead: number;
      base: string;
      dirty: unknown[];
    }>('ade-review-open', `AdeTaskService.Branch#${phase}`);
    const { window: page, control } = await openPlan(relaunch, [
      OPEN,
      {
        channel: IPC.adeTaskBoard,
        response: adeBoard((b) => {
          for (const r of b.tasks.find((x) => x.id === 'T_deps')?.runs ?? []) r.state = 'done';
          const br = b.branches.find((x) => x.id === 'b_deps');
          if (!br) throw new Error('fixture branch missing');
          const { name, ahead, base, dirty } = facts;
          Object.assign(br, { name, ahead, base, dirty });
        }),
      },
    ]);
    const btn = page.locator(`${card('T_deps')} ${t('ade-card-review')}`);
    await btn.locator('xpath=..').hover();
    const tooltip = page.locator(tip);
    await expect(tooltip.locator('[data-var="base"]')).toHaveText(facts.base);
    if (phase === 'uncommitted') {
      await expect(btn).toBeDisabled();
      await expect(tooltip).toContainText('Only uncommitted changes on');
    } else {
      await expect(btn).toBeEnabled();
      await expect(tooltip.locator('[data-var="branch"]')).toHaveText(facts.name);
      await btn.click();
      await expect.poll(() => calls(control)).toHaveLength(1);
      expect(calls(control)[0]?.args).toEqual({ branchId: 'b_deps' });
    }
  });
}
