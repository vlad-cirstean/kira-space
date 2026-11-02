import { expect, test } from './fixtures';
import { adeFixture, openPlan } from './support/adeV2';
import { IPC } from './support/ipcChannels';

// Capture box and the Add popover on the committed fixtures.

interface TaskFx {
  id: string;
  [key: string]: unknown;
}

function visibleTask(): TaskFx {
  return { ...adeFixture<TaskFx>('task'), id: 'T_deps' };
}

test('Enter in the capture box adds a task to Later and confirms it', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCaptureTask, response: visibleTask() },
  ]);
  const input = page.locator('[data-testid="ade-capture"]');
  await input.fill('Rate limit exports');
  await input.press('Enter');
  await expect(page.locator('[data-testid="ade-capture-note"]')).toHaveText('Added to later');
  await expect(input).toHaveValue('');
  expect(control.log().find((e) => e.channel === IPC.adeTaskCaptureTask)?.args).toEqual({
    title: 'Rate limit exports',
    day: null,
  });
});

test('New task lands today without repos and selects it', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCreateTask, response: visibleTask() },
  ]);
  await page.locator('[data-testid="ade-add"]').click();
  await page.locator('[data-testid="ade-nw-title"]').fill('Export CSV');
  await page.locator('[data-testid="ade-nw-jira"]').fill('PAY-77');
  await expect(page.locator('[data-testid="ade-nw-repo"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="ade-nw-base"]')).toHaveCount(0);
  await page.locator('[data-testid="ade-nw-add"]').click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.adeTaskCreateTask))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.adeTaskCreateTask)?.args).toEqual({
    title: 'Export CSV',
    jira: { key: 'PAY-77', url: '' },
    hostUrl: '',
    notes: '',
    codeRepoIds: [],
    workflowId: '',
    bases: {},
    day: '2026-09-22',
  });
  await expect(page.locator('[data-testid="ade-add-popover"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="ade-task"][data-task-id="T_deps"]')).toHaveAttribute(
    'data-selected',
    'true',
  );
});

test('Existing branch searches every repo and adds the picked one', async ({ relaunch }) => {
  const added = { ...adeFixture<{ task: TaskFx }>('add-existing-branch') };
  added.task = visibleTask();
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCandidateBranches, response: adeFixture('candidates') },
    { channel: IPC.adeTaskAddExistingBranch, response: added },
  ]);
  await page.locator('[data-testid="ade-add"]').click();
  await page.locator('[data-testid="ade-add-tab-branch"]').click();
  await page.locator('[data-testid="ade-branch-search"]').fill('invoice');
  const rows = page.locator('[data-testid^="ade-branch-option-"]');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('You');
  await expect(rows.first()).toContainText('web-app');
  await rows.first().click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.adeTaskAddExistingBranch))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.adeTaskAddExistingBranch)?.args).toEqual({
    codeRepoId: 'repo-web-app',
    name: 'fix/invoice-rounding',
    taskId: '',
  });
});

test('the branch list shows 50 candidates and Show more reveals the rest', async ({ relaunch }) => {
  const base = adeFixture<{ branches: Record<string, unknown>[] }>('candidates');
  const template = base.branches[0] ?? {};
  const branches = Array.from({ length: 60 }, (_, i) => ({
    ...template,
    name: `feat/bulk-${String(i).padStart(2, '0')}`,
  }));
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCandidateBranches, response: { ...base, branches } },
  ]);
  await page.locator('[data-testid="ade-add"]').click();
  await page.locator('[data-testid="ade-add-tab-branch"]').click();
  const rows = page.locator('[data-testid^="ade-branch-option-"]');
  await expect(rows).toHaveCount(50);
  await page.locator('[data-testid="ade-branch-more"]').click();
  await expect(rows).toHaveCount(60);
});
