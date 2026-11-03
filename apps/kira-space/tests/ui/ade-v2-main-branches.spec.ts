import { expect, test } from './fixtures';
import { openPlan, openTask } from './support/adeV2';

// Main tab of a multi-branch task: branches first, in the Plan's row look, one place per branch.
// Frontend only: IPC is unchanged, so no backend half.

const t = (id: string) => `[data-testid="${id}"]`;
const mine = (id: string) => `${t('ade-task-branch')}[data-branch-id="${id}"]`;
const BRANCHES = ['b_meter', 'b_bill', 'b_billdash'];

test('branches come first, stacks together, and stacked rows indent', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_bill');
  const first = await page.locator(t('ade-task-branch')).first().boundingBox();
  const flow = await page.locator(t('ade-workflow-block')).boundingBox();
  expect(first && flow && first.y < flow.y).toBe(true);
  await expect(page.locator(t('ade-task-branch'))).toHaveCount(3);
  const ids = await page
    .locator(t('ade-task-branch'))
    .evaluateAll((els) => els.map((e) => (e as HTMLElement).dataset.branchId));
  expect(ids).toEqual(BRANCHES);
  const depth = async (id: string) =>
    Number(await page.locator(mine(id)).getAttribute('data-depth'));
  expect(await depth('b_billdash')).toBeGreaterThan(await depth('b_bill'));
});

test('a panel row reads like its Plan row', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  const plan = (id: string) =>
    page.locator(`${t('ade-branch-row')}[data-branch-id="${id}"] ${t('ade-tag')}`);
  const planTags: Record<string, { kind: string | null; icon: string | null }> = {};
  for (const id of BRANCHES) {
    planTags[id] = {
      kind: await plan(id).getAttribute('data-kind'),
      icon: await plan(id).getAttribute('data-icon'),
    };
  }
  await openTask(page, 'T_bill');
  for (const id of BRANCHES) {
    const tag = page.locator(`${mine(id)} ${t('ade-tag')}`);
    await expect(tag).toHaveAttribute('data-kind', planTags[id]?.kind ?? '');
    await expect(tag).toHaveAttribute('data-icon', planTags[id]?.icon ?? '');
    await expect(page.locator(`${mine(id)} ${t('ade-branch-repo')}`)).toBeVisible();
  }
  await expect(page.locator(`${mine('b_billdash')} ${t('ade-base-marker')}`)).toBeVisible();
});

test('the PR line shows state, base and checks under its branch only', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_bill');
  const line = page.locator(`${t('ade-task-branch-pr')}[data-branch-id="b_bill"]`);
  await expect(line.locator(t('ade-task-branch-pr-state'))).toHaveAttribute('data-state', /.+/);
  await expect(line.locator(t('ade-task-branch-pr-base'))).toHaveText('→ main');
  await expect(line.locator(t('ade-task-branch-pr-checks'))).toBeVisible();
  await expect(page.locator(`${t('ade-task-branch-pr')}[data-branch-id="b_billdash"]`)).toHaveCount(
    0,
  );
});

test('run lines name their branch only where a repo repeats', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_bill');
  const lines = page.locator(`${t('ade-step-block')}[data-step-id="impl"] ${t('ade-run-line')}`);
  const ids = await lines.evaluateAll((els) => els.map((e) => (e as HTMLElement).dataset.branchId));
  expect(ids).toEqual(BRANCHES);
  const named = (id: string) =>
    lines.and(page.locator(`[data-branch-id="${id}"]`)).locator(t('ade-run-branch'));
  await expect(named('b_bill')).toHaveText('feat/usage-billing');
  await expect(named('b_billdash')).toHaveText('feat/billing-dashboard');
  await expect(named('b_meter')).toHaveCount(0);
});

test('step blocks hold no branch rows', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_bill');
  await expect(page.locator(`${t('ade-step-block')} ${t('ade-task-branch')}`)).toHaveCount(0);
  await expect(page.locator(t('ade-release-row'))).toHaveCount(0);
  await expect(page.locator(t('ade-review-row'))).toHaveCount(0);
});

test('the edit menu opens and a click selects the branch', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_bill');
  await page.locator(`${t('ade-task-branch-menu')}[data-branch-id="b_bill"]`).click();
  await expect(page.locator(t('ade-task-branch-menu-content'))).toBeVisible();
  await page.keyboard.press('Escape');
  await page.locator(mine('b_meter')).click();
  await expect(page.locator(`${t('ade-branch-row')}[data-branch-id="b_meter"]`)).toHaveAttribute(
    'data-selected',
    'true',
  );
});
