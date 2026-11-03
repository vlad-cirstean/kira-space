import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, adeFixture, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// The Plan's right-hand panel: task fields, notes, branch details, force push, resize.

interface BoardFx {
  tasks: { id: string; est: string }[];
  branches: {
    id: string;
    deployments: { env: string; status: string; error: string }[];
  }[];
}

const card = (page: Page, id: string) =>
  page.locator(`[data-testid="ade-card"][data-task-id="${id}"] [data-testid="ade-card-head"]`);

async function openTask(page: Page, id: string): Promise<void> {
  await card(page, id).click();
  await expect(page.locator('[data-testid="ade-panel"]')).toBeVisible();
}

function updates(control: { log(): { channel: string; args?: unknown }[] }) {
  return control.log().filter((e) => e.channel === IPC.adeTaskUpdateTask);
}

test('clicking a review card opens the task panel', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator('[data-testid="ade-load-all"]').click();
  const row = page.locator(
    '[data-testid="ade-card"][data-task-id="R_sara"] [data-testid="ade-branch-row"][data-branch-id="b_sara"]',
  );
  await row.scrollIntoViewIfNeeded();
  await row.click();
  await expect(page.locator('[data-testid="ade-panel-review-note"]')).toBeVisible();
  await expect(page.locator('[data-testid="ade-panel-back"]')).toHaveCount(0);
  await page.locator('[data-testid="ade-task-branch"][data-branch-id="b_sara"]').click();
  await expect(page.locator('[data-testid="ade-panel-back"]')).toBeVisible();

  await page.locator('[data-testid="ade-panel-back"]').click();
  await row.focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('[data-testid="ade-panel-review-note"]')).toBeVisible();
  await expect(page.locator('[data-testid="ade-panel-back"]')).toHaveCount(0);
});

test('Space on a focused card head selects the task', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await card(page, 'T_bill').focus();
  await page.keyboard.press('Space');
  await expect(page.locator('[data-testid="ade-panel"]')).toBeVisible();
  await expect(page.locator('[data-testid="ade-panel-title"]')).toHaveAttribute(
    'placeholder',
    /PAY-102/,
  );
});

test('selecting a task shows its facts; status is read-only', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_bill');
  await expect(page.locator('[data-testid="ade-panel-title"]')).toHaveAttribute(
    'placeholder',
    /PAY-102/,
  );
  await expect(page.locator('[data-testid="ade-panel-facts"]')).toContainText(
    '3 branches in api, web-app',
  );
  await expect(page.locator('[data-testid="ade-task-status-why"]')).not.toBeEmpty();
  await expect(page.locator('[data-testid="ade-task-branch"]')).toHaveCount(3);
});

test('Escape in the panel title reverts the text and writes nothing', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_notif');
  const name = page.locator('[data-testid="ade-panel-title"]');
  const original = await name.inputValue();
  await name.fill('Discard me');
  await name.press('Escape');
  await expect(name).toHaveValue(original);
  expect(updates(control)).toHaveLength(0);
});

test('name, Issue, PR and Not merging write UpdateTask patches', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_notif');
  const name = page.locator('[data-testid="ade-panel-title"]');
  await name.fill('Notify on mention');
  await name.press('Enter');
  await expect.poll(() => updates(control)).toHaveLength(1);
  expect(updates(control)[0]?.args).toMatchObject({
    taskId: 'T_notif',
    patch: { title: 'Notify on mention', jira: null, hostUrl: null, est: null, kind: null },
  });

  const paste = page.locator('[data-testid="ade-details-paste"]');
  await page.locator('[data-testid="ade-panel-tab-details"]').click();
  await paste.fill('PAY-9');
  await paste.press('Enter');
  await expect.poll(() => updates(control)).toHaveLength(2);
  expect(updates(control)[1]?.args).toMatchObject({
    patch: { jira: { key: 'PAY-9', url: '' } },
  });

  await paste.fill('https://github.com/acme/web-app/issues/882');
  await paste.press('Enter');
  await expect.poll(() => updates(control)).toHaveLength(3);
  expect(updates(control)[2]?.args).toMatchObject({
    patch: { hostUrl: 'https://github.com/acme/web-app/issues/882' },
  });

  await page.locator('[data-testid="ade-panel-tab-main"]').click();
  await page.locator('[data-testid="ade-task-parked"]').click();
  await expect.poll(() => updates(control)).toHaveLength(4);
  expect(updates(control)[3]?.args).toMatchObject({ patch: { kind: 'parked' } });
});

test('a refused git host link shows the backend error', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskUpdateTask,
      error: { code: 'invalid', message: 'Not a GitHub PR or issue link.' },
    },
  ]);
  await openTask(page, 'T_notif');
  await page.locator('[data-testid="ade-panel-tab-details"]').click();
  const paste = page.locator('[data-testid="ade-details-paste"]');
  await paste.fill('https://example.com/not-a-forge-link');
  await paste.press('Enter');
  await expect(page.locator('[data-testid="ade-details-paste-error"]')).toContainText(
    'Not a GitHub PR or issue link.',
  );
});

test('a refused estimate shows the backend error', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskUpdateTask,
      error: { code: 'invalid', message: 'Enter an estimate like 2h or 3d.' },
    },
  ]);
  await openTask(page, 'T_bill');
  await page.locator('[data-testid="ade-estimate-num"]').fill('1');
  await page.locator('[data-testid="ade-estimate-num"]').press('Tab');
  await expect(page.locator('[data-testid="ade-estimate-error"]')).toContainText(
    'Enter an estimate like 2h or 3d.',
  );
});

test('+ Add repo adds a draft branch in the chosen repo', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_bill');
  await page.locator('[data-testid="ade-add-repo"]').click();
  await page.locator('[data-testid="ade-add-repo-option-repo-mobile"]').click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.adeTaskAddTaskRepo))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.adeTaskAddTaskRepo)?.args).toEqual({
    taskId: 'T_bill',
    codeRepoId: 'repo-mobile',
    base: null,
  });
});

test('+ Add branch attaches an existing branch to this task', async ({ relaunch }) => {
  const added = adeFixture<{ task: unknown }>('add-existing-branch');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCandidateBranches, response: adeFixture('candidates') },
    { channel: IPC.adeTaskAddExistingBranch, response: added },
  ]);
  await openTask(page, 'T_bill');
  await page.locator('[data-testid="ade-add-branch"]').click();
  await expect(page.locator('[data-testid="ade-add-attach"]')).toContainText('Adding to');
  await expect(page.locator('[data-testid="ade-add-tab-new"]')).toHaveCount(0);
  await page.locator('[data-testid="ade-branch-search"]').fill('invoice');
  await page.locator('[data-testid^="ade-branch-option-"]').first().click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.adeTaskAddExistingBranch))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.adeTaskAddExistingBranch)?.args).toEqual({
    codeRepoId: 'repo-web-app',
    name: 'fix/invoice-rounding',
    taskId: 'T_bill',
  });
});

test('Main tab has no link or notes rows; notes live in the Details tab', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_bill');
  const labels = (await page.locator('[data-testid="ade-main-tab"] dt').allTextContents()).map(
    (s) => s.trim(),
  );
  for (const label of ['Issue', 'PR', 'Notes']) expect(labels).not.toContain(label);
  await expect(page.locator('[data-testid="ade-task-notes-row"]')).toHaveCount(0);
  await page.locator('[data-testid="ade-panel-tab-details"]').click();
  await expect(page.locator('[data-testid="ade-details-tab"] .ProseMirror')).toContainText(
    'Q4 pricing launch',
  );
});

test('typing in Notes saves once, debounced', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_notif');
  await page.locator('[data-testid="ade-panel-tab-details"]').click();
  const editor = page.locator('[data-testid="ade-details-tab"] .ProseMirror');
  await editor.click();
  await page.keyboard.type('check the digest', { delay: 20 });
  await expect.poll(() => updates(control)).toHaveLength(1);
  await page.clock.runFor(900);
  expect(updates(control)).toHaveLength(1);
  const args = updates(control)[0]?.args as { patch: { notes: string } };
  expect(args.patch.notes).toContain('check the digest');
});

test('a branch opens Details with merged and deployed rows; back to the task', async ({
  relaunch,
}) => {
  const board = adeFixture<BoardFx>('board');
  const auth = board.branches.find((b) => b.id === 'b_auth');
  const preview = auth?.deployments[0];
  if (preview) {
    preview.status = 'unknown';
    preview.error = 'script exited 1';
  }
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await openTask(page, 'T_auth');
  await page.locator('[data-testid="ade-task-branch"][data-branch-id="b_auth"]').click();
  const details = page.locator('[data-testid="ade-branch-details"]');
  await expect(details).toBeVisible();
  await expect(
    details.locator('[data-testid="ade-branch-into"][data-target="develop"]'),
  ).toContainText('Stale');
  await expect(
    details.locator('[data-testid="ade-branch-deploy"][data-env="preview"]'),
  ).toContainText('Unknown');
  await page.locator('[data-testid="ade-panel-back"]').click();
  await expect(page.locator('[data-testid="ade-main-tab"]')).toBeVisible();
});

test("the panel has no toolbar; actions sit at the tab row's right end", async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_auth');
  const panel = page.locator('[data-testid="ade-panel"]');
  await expect(panel.locator('[data-slot="panel-header"]')).toHaveCount(0);
  const actions = panel.locator('[data-testid="ade-panel-actions"]');
  await expect(actions.locator('[data-testid="ade-panel-more"]')).toBeVisible();
  await expect(actions.locator('[data-testid^="ade-task-action-"]').first()).toBeVisible();
  const box = await actions.boundingBox();
  const panelBox = await panel.boundingBox();
  const tab = await panel.locator('[data-testid="ade-panel-tab-main"]').boundingBox();
  if (!box || !panelBox || !tab) {
    throw new Error('missing bounding box');
  }
  expect(panelBox.x + panelBox.width - (box.x + box.width)).toBeLessThanOrEqual(8);
  expect(box.y + box.height).toBeGreaterThan(tab.y);
  expect(box.y).toBeLessThan(tab.y + tab.height);
  await page.locator('[data-testid="ade-branch-row"][data-branch-id="b_cart"]').first().click();
  await expect(page.locator('[data-testid="ade-branch-details"]')).toBeVisible();
  await expect(panel.locator('[data-slot="panel-header"]')).toHaveCount(0);
});

test('a branch row on the Plan selects the branch', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator('[data-testid="ade-branch-row"][data-branch-id="b_cart"]').first().click();
  await expect(page.locator('[data-testid="ade-branch-details"]')).toBeVisible();
  await expect(page.locator('[data-testid="ade-branch-deploy"][data-env="prod"]')).toContainText(
    'Deployed',
  );
});

test('Force push in the branch header asks first and sends the branch', async ({ relaunch }) => {
  const board = adeFixture<{ plan: { unpushed: Record<string, boolean> } }>('board');
  board.plan.unpushed = { b_deps: true };
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
    { channel: IPC.adeTaskForcePush, response: adeFixture('force-push') },
  ]);
  await openTask(page, 'T_deps');
  await page.locator('[data-testid="ade-task-branch"]').first().click();
  await page.locator('[data-testid="ade-panel-force-push"]').click();
  await page.locator('[data-testid="ade-confirm-yes"]').click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.adeTaskForcePush))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.adeTaskForcePush)?.args).toEqual({
    branchId: 'b_deps',
  });
});

test('a pushed branch offers no Force push', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_deps');
  await page.locator('[data-testid="ade-task-branch"]').first().click();
  await expect(page.locator('[data-testid="ade-branch-details"]')).toBeVisible();
  await expect(page.locator('[data-testid="ade-panel-force-push"]')).toHaveCount(0);
});

test('dragging the handle persists the dragged width; a click persists nothing', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_bill');
  const handle = page.locator('[data-testid="ade-panel-resize-handle"]');
  const startWidth = Number(await handle.getAttribute('aria-valuenow'));
  const box = await handle.boundingBox();
  if (!box) throw new Error('no handle');
  const x = box.x + box.width / 2;
  const y = box.y + 200;
  const widthWrites = () =>
    control
      .log()
      .filter((e) => e.channel === IPC.layoutSet && JSON.stringify(e.args).includes('"ade"'));
  await page.mouse.click(x, y);
  expect(widthWrites()).toHaveLength(0);
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x - 120, y, { steps: 6 });
  await page.mouse.up();
  await expect.poll(() => widthWrites()).toHaveLength(1);
  expect(widthWrites()[0]?.args).toEqual({
    patch: { panel: { ade: { width: startWidth + 120 } } },
  });
});

test('arrow keys on the handle resize and persist the panel width', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_bill');
  const handle = page.locator('[data-testid="ade-panel-resize-handle"]');
  const startWidth = Number(await handle.getAttribute('aria-valuenow'));
  await handle.focus();
  await page.keyboard.press('ArrowLeft');
  const writes = () =>
    control
      .log()
      .filter((e) => e.channel === IPC.layoutSet && JSON.stringify(e.args).includes('"ade"'));
  await expect.poll(() => writes().length).toBe(1);
  const patch = writes()[0]?.args as { patch: { panel: { ade: { width: number } } } };
  expect(patch.patch.panel.ade.width).toBeGreaterThan(startWidth);
});

test('the Advanced switch saves ade.headlessSettingSources with the dialog', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await page.locator('[data-testid="settings-section-Advanced"]').click();
  const sw = page.locator('[data-testid="settings-ade-headless-sources"]');
  await expect(sw).toHaveAttribute('aria-checked', 'false');
  await sw.click();
  expect(
    control
      .log()
      .some(
        (e) =>
          e.channel === IPC.settingsSet &&
          JSON.stringify(e.args).includes('headlessSettingSources'),
      ),
  ).toBe(false);
  await page.locator('[data-testid="settings-save"]').click();
  await expect
    .poll(() =>
      control
        .log()
        .some(
          (e) =>
            e.channel === IPC.settingsSet &&
            JSON.stringify(e.args).includes('"headlessSettingSources":"user"'),
        ),
    )
    .toBe(true);
});

test('Details lists every environment of the repo, not deployed where the branch has none', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator('[data-testid="ade-branch-row"][data-branch-id="b_bill"]').first().click();
  const rows = page.locator('[data-testid="ade-branch-deploy"]');
  await expect(rows).toHaveCount(3);
  await expect(rows.first()).toContainText('Not deployed');
  await expect(page.locator('[data-testid="ade-branch-deploy"][data-env="prod"]')).toBeVisible();
});

test('contract: a deployed environment and one whose script failed show as the backend reports them', async ({
  relaunch,
}) => {
  const facts = contract<{ env: string; status: string; error: string }[]>(
    'repo-env',
    'AdeTaskService.Branch#deployments',
  );
  const board = adeFixture<BoardFx>('board');
  const target = board.branches.find((b) => b.id === 'b_bill');
  if (target)
    target.deployments = facts.map((d) => ({
      ...d,
      deployedSha: d.status === 'deployed' ? 'abc1234' : '',
    }));
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator('[data-testid="ade-branch-row"][data-branch-id="b_bill"]').first().click();
  const row = (env: string) => page.locator(`[data-testid="ade-branch-deploy"][data-env="${env}"]`);
  const ok = facts.find((d) => d.status === 'deployed');
  const broken = facts.find((d) => d.error);
  await expect(row(ok?.env ?? '')).toContainText('Runs abc1234, contains this branch');
  await expect(row(broken?.env ?? '')).toContainText(broken?.error ?? '');
});

// ---- worktree setup

test('a running setup shows a ticking preparing tag', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  const tag = page
    .locator('[data-testid="ade-task"][data-task-id="T_push"] [data-testid="ade-tag"]')
    .and(page.locator('[aria-label*="preparing" i]'));
  await expect(tag).toHaveCount(1);
  const first = await tag.getAttribute('aria-label');
  await expect.poll(async () => tag.getAttribute('aria-label')).not.toBe(first);
});

test('See error opens the branch with the failed setup log, Retry setup sends the branch', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await page
    .locator('[data-testid="ade-task"][data-task-id="T_search"] [data-testid="ade-see-error"]')
    .click();
  await expect(page.locator('[data-testid="ade-branch-details"]')).toBeVisible();
  const setup = page.locator('[data-testid="ade-worktree-setup"]');
  await expect(setup).toBeVisible();
  await expect(setup).toContainText('failed');
  await expect(setup.locator('[data-testid="script-progress-reason"]')).toHaveText(
    'exited with status 1',
  );
  await expect(setup.locator('[data-testid="ade-run-log"]')).toContainText('ERR_PNPM_FETCH_404');

  await page.locator('[data-testid="ade-setup-retry"]').click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.adeTaskRetrySetup))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.adeTaskRetrySetup)?.args).toEqual({
    branchId: 'b_searchui',
  });
});

test('a ready setup shows its status without a log', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator('[data-testid="ade-branch-row"][data-branch-id="b_search"]').click();
  const setup = page.locator('[data-testid="ade-worktree-setup"]');
  await expect(setup).toContainText('ready');
  await expect(setup.locator('[data-testid="ade-run-log"]')).toHaveCount(0);
});

test('the step mover goes back, picks any step and is off while a run is live', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  const moves = () => control.log().filter((e) => e.channel === IPC.adeTaskSetTaskStep);

  await openTask(page, 'T_auth');
  await page.locator('[data-testid="ade-step-back"]').click();
  await page.locator('[data-testid="ade-stage-move"]').click();
  await expect.poll(moves).toHaveLength(1);
  expect(moves()[0]?.args).toMatchObject({ taskId: 'T_auth', stepId: 'pr' });

  await page.locator('[data-testid="ade-step-pick"]').click();
  await page.locator('[data-testid="ade-step-option-done"]').click();
  await page.locator('[data-testid="ade-stage-move"]').click();
  await expect.poll(moves).toHaveLength(2);
  expect(moves()[1]?.args).toMatchObject({ taskId: 'T_auth', stepId: 'done' });

  await openTask(page, 'T_alerts');
  await expect(page.locator('[data-testid="ade-step-back"]')).toBeDisabled();
  await expect(page.locator('[data-testid="ade-step-next"]')).toBeEnabled();

  await openTask(page, 'T_bill');
  await expect(page.locator('[data-testid="ade-step-pick"]')).toBeDisabled();
  await expect(page.locator('[data-testid="ade-step-next"]')).toBeDisabled();
});

test('a started task on an older workflow version says so', async ({ relaunch }) => {
  const workflow = adeFixture<{ workflows: { workflow: unknown }[] }>('workflows').workflows[0]
    ?.workflow;
  const board = adeBoard((b) => {
    const task = b.tasks.find((x) => x.id === 'T_bill');
    if (task) Object.assign(task, { workflow, workflowOutdated: true });
  });
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await openTask(page, 'T_bill');
  await expect(page.locator('[data-testid="ade-workflow-outdated"]')).toHaveText(
    'Updated, applies to new work only',
  );
  await openTask(page, 'T_auth');
  await expect(page.locator('[data-testid="ade-workflow-outdated"]')).toHaveCount(0);
});

test('a skipped step is dimmed, cannot be entered, and Skip this step jumps past it', async ({
  relaunch,
}) => {
  const workflows = adeFixture<{
    workflows: { workflow: { steps: { id: string; skip: boolean }[] } | null }[];
  }>('workflows');
  for (const s of workflows.workflows[0]?.workflow?.steps ?? []) s.skip = s.id === 'plan';
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskWorkflows, response: workflows },
  ]);
  const moves = () => control.log().filter((e) => e.channel === IPC.adeTaskSetTaskStep);

  await openTask(page, 'T_alerts');
  const skipped = page.locator('[data-testid="ade-step-block"][data-step-id="plan"]');
  await expect(skipped).toHaveAttribute('data-state', 'skipped');
  await expect(skipped).toContainText('Skipped');

  await page.locator('[data-testid="ade-step-pick"]').click();
  await expect(page.locator('[data-testid="ade-step-option-plan"]')).toHaveAttribute(
    'data-disabled',
    '',
  );
  await page.keyboard.press('Escape');

  await page
    .locator(
      '[data-testid="ade-step-block"][data-step-id="spec"] [data-testid="ade-step-block-head"]',
    )
    .click({ button: 'right', position: { x: 200, y: 8 } });
  await page.locator('[data-testid="menu-item-ade-step-skip-this"]').click();
  await page.locator('[data-testid="ade-stage-move"]').click();
  // Skipping past the skipped step is the natural next stage of a manual step: StepDone.
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.adeTaskStepDone))
    .toHaveLength(1);
  expect(moves()).toHaveLength(0);
});

test('agent-made branches carry a sparkle', async ({ relaunch }) => {
  const board = adeBoard((b) => {
    const task = b.tasks.find((x) => x.id === 'T_bill');
    const branch = b.branches.find((x) => x.taskId === task?.id && x.name !== '');
    if (branch) branch.origin = 'agent';
  });
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await openTask(page, 'T_bill');
  await expect(page.locator('[data-testid="ade-workflow-space"]')).toHaveCount(0);
  const sparkle = page.locator('[data-testid="ade-task-branch"] [data-testid="ade-branch-agent"]');
  await expect(sparkle).toHaveCount(1);
  await sparkle.hover();
  await expect(
    page.locator('[data-testid="ade-branch-tip"]').getByText('Added or named by the agent').first(),
  ).toBeVisible();
  await page
    .locator('[data-testid="ade-task-branch"]:has([data-testid="ade-branch-agent"])')
    .click();
  await expect(page.locator('[data-testid="ade-panel-agent"]')).toBeVisible();
});

test('a change request link opens the in-app PR view', async ({ relaunch }) => {
  const url = 'https://github.com/acme/web-app/pull/7';
  const board = adeBoard((b) => {
    const task = b.tasks.find((x) => x.id === 'T_bill');
    Object.assign(task ?? {}, {
      hostUrl: url,
      hostLink: { provider: 'github', kind: 'changeRequest', ref: 'acme/web-app#7' },
    });
  });
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
    { channel: IPC.hostViewOpen, response: { key: 'hostview-1', opened: true } },
  ]);
  await openTask(page, 'T_bill');
  await page.locator('[data-testid="ade-panel-tab-details"]').click();
  await page
    .locator('[data-testid="ade-details-link-title"]', { hasText: 'acme/web-app#7' })
    .click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.hostViewOpen))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.hostViewOpen)?.args).toEqual({ url });
});
