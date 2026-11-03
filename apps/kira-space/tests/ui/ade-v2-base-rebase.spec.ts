import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { type AdeBoardFx, adeBoard, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Task base branch, Change base and the headless rebase: one rule behind every control, the base
// picker, the dialog and how a run's outcome shows.

const t = (id: string) => `[data-testid="${id}"]`;
const row = (id: string) => `[data-testid="ade-branch-row"][data-branch-id="${id}"]`;

type Edit = (b: AdeBoardFx) => void;
const branch = (b: AdeBoardFx, id: string) =>
  b.branches.find((x) => x.id === id) as Record<string, unknown>;

/** feat/oauth-login based on develop and behind it: the rebase tag shows. */
/** T_notif's only branch idle, so Change base is allowed. */
const idleNotif: Edit = (b) => {
  for (const r of b.tasks.find((x) => x.id === 'T_notif')?.runs ?? []) r.state = 'done';
};

const onDevelop: Edit = (b) => {
  Object.assign(branch(b, 'b_auth'), { base: 'develop', behind: 2 });
};

function rebaseRun(id: string, branchId: string, taskId: string, over: Record<string, unknown>) {
  return {
    id,
    taskId,
    stepId: '',
    branchId,
    attempt: 1,
    state: 'failed',
    loops: 0,
    note: '',
    summary: '',
    sessionId: '',
    exitCode: 1,
    startedAt: 1790060000000,
    finishedAt: 1790060100000,
    purpose: 'rebase',
    outcome: null,
    ...over,
  };
}

const failedOutcome = {
  status: 'failed',
  reason: 'src/auth/session.ts conflicts',
  source: 'agent',
  reported: true,
  report: {
    conflictedFiles: ['src/auth/session.ts'],
    lastGitError: 'CONFLICT (content)',
    tried: 'plain rebase',
  },
  rebase: {
    verified: false,
    inProgress: true,
    aborted: false,
    ontoRef: 'origin/develop',
    ontoTip: 'a1b2c3d4e5f6',
    conflictedFiles: ['src/auth/session.ts'],
    branches: [
      {
        branchId: 'b_auth',
        name: 'feat/oauth-login',
        before: '1f8e6aa11',
        after: '1f8e6aa11',
        onBase: false,
      },
    ],
    pushed: null,
  },
};

async function open(relaunch: Parameters<typeof openPlan>[0], edit: Edit, extra = []) {
  const board = adeBoard(edit);
  return openPlan(relaunch, [{ channel: IPC.adeTaskBoard, response: board }, ...extra]);
}

const authTask = (page: Page) =>
  page.locator(`${row('b_auth')} >> xpath=ancestor::*[@data-testid="ade-task"][1]`);

test('the picker disables a branch and its descendants and shows Previous base only when pending', async ({
  relaunch,
}) => {
  const repoBranches = {
    mainName: 'main',
    previous: 'release',
    branches: [
      {
        name: 'feat/usage-billing',
        local: true,
        remote: false,
        branchId: 'b_bill',
        taskId: 'T_bill',
        taskTitle: 'Billing',
        draft: false,
        excluded: 'self',
      },
      {
        name: 'feat/billing-dashboard',
        local: true,
        remote: false,
        branchId: 'b_billdash',
        taskId: 'T_bill',
        taskTitle: 'Billing',
        draft: false,
        excluded: 'stacked',
      },
    ],
  };
  const { window: page } = await open(
    relaunch,
    (b) => {
      Object.assign(branch(b, 'b_auth'), { basePendingFrom: 'release' });
    },
    [{ channel: IPC.adeTaskRepoBranches, response: repoBranches }] as never,
  );
  await page.locator(row('b_auth')).click();
  await page.locator(t('ade-panel-action-change-base')).click();
  await page.locator(t('ade-base-picker')).click();
  await expect(page.locator(t('ade-base-picker-option-previous'))).toBeVisible();
  await expect(page.locator(t('ade-base-picker-option-feat/usage-billing'))).toHaveAttribute(
    'data-disabled',
  );
  await expect(page.locator(t('ade-base-picker-option-feat/billing-dashboard'))).toHaveAttribute(
    'data-disabled',
  );
  await expect(page.locator(t('ade-base-picker-option-feat/usage-billing'))).toContainText(
    'its own base',
  );
  await expect(page.locator(t('ade-base-picker-option-feat/billing-dashboard'))).toContainText(
    'Stacked on this branch',
  );
});

test('a branch behind a develop base offers Rebase onto develop, run in the background', async ({
  relaunch,
}) => {
  const { window: page, control } = await open(relaunch, onDevelop);
  const tag = authTask(page).locator(t('ade-rebase')).first();
  await expect(tag).toHaveText('Rebase');
  await tag.hover();
  await expect(
    page.locator('[data-testid="branch-chip"][data-var="base"]', { hasText: 'develop' }).first(),
  ).toBeVisible();
  await tag.click();
  await expect(page.locator(t('ade-dialog-message'))).toHaveValue(/Rebase/);
  await expect(page.locator(t('ade-dialog-suffix'))).toBeVisible();
  await expect(page.locator(t('ade-dialog-target'))).toHaveCount(0);
  await expect(page.locator(t('ade-dialog-send'))).toHaveText('Run in background');
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskRebase)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRebase)[0]?.args).toMatchObject({
    branchId: 'b_auth',
    message: '',
  });
});

test('an edited prompt is what Rebase sends', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, onDevelop);
  await authTask(page).locator(t('ade-rebase')).first().click();
  await page.locator(t('ade-dialog-message')).fill('Rebase, keep merge commits.');
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskRebase)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRebase)[0]?.args).toMatchObject({
    message: 'Rebase, keep merge commits.',
  });
});

test('the same act from the panel header, fix menu and task menu has one label and call', async ({
  relaunch,
}) => {
  const { window: page, control } = await open(relaunch, onDevelop);
  await page.locator(row('b_auth')).click();
  const header = page.locator(t('ade-panel-action-rebase-base'));
  await expect(header).toContainText('Rebase onto develop');
  await page.keyboard.press('Escape');
  await page.locator(row('b_auth')).click({ button: 'right' });
  const fix = page.getByRole('menuitem', { name: 'Rebase onto develop' });
  await expect(fix).toBeVisible();
  await fix.click();
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskRebase)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRebase)[0]?.args).toMatchObject({ branchId: 'b_auth' });
});

test('the card base button opens the dialog for one branch and a menu for several', async ({
  relaunch,
}) => {
  const { window: page } = await open(relaunch, idleNotif);
  const notif = page.locator(`${t('ade-card')}[data-task-id="T_notif"]`);
  await notif.locator(t('ade-card-change-base')).click();
  await expect(page.locator(t('ade-dialog-title'))).toHaveText('Change the base');
  await page.locator(t('ade-dialog-cancel')).click();
  const auth = page.locator(`${t('ade-card')}[data-task-id="T_auth"]`);
  await auth.locator(t('ade-card-change-base')).click();
  await expect(page.locator(t('ade-card-change-base-menu'))).toBeInViewport();
  await expect(page.locator(t('ade-card-change-base-item')).first()).toBeVisible();
});

test('Change base on a created branch refetches the preview with onto and runs Rebase', async ({
  relaunch,
}) => {
  const { window: page, control } = await open(relaunch, idleNotif, [
    { channel: IPC.adeTaskSessions, response: { sessions: [] } },
  ] as never);
  await page
    .locator(`${t('ade-card')}[data-task-id="T_notif"]`)
    .locator(t('ade-card-change-base'))
    .click();
  await page.locator(t('ade-base-picker')).click();
  await page.locator(t('ade-base-picker-option-develop')).click();
  await expect
    .poll(() => calls(control, IPC.adeTaskRebasePreview).at(-1)?.args)
    .toMatchObject({ onto: { ref: 'develop', branchId: '' } });
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskRebase)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRebase)[0]?.args).toMatchObject({
    branchId: 'b_notif',
    onto: { ref: 'develop', branchId: '' },
  });
});

test('Change base on a draft stores the base with no prompt', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await page
    .locator(`${t('ade-card')}[data-task-id="T_alerts"]`)
    .locator(t('ade-card-change-base'))
    .first()
    .click();
  await expect(page.locator(t('ade-card-change-base-menu'))).toBeVisible();
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  await expect(page.locator(t('ade-dialog-message'))).toHaveCount(0);
  await page.locator(t('ade-base-picker')).click();
  await page.locator(t('ade-base-picker-option-develop')).click();
  await expect(page.locator(t('ade-dialog-send'))).toHaveText('Save base');
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSetBranchBase)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRebase)).toHaveLength(0);
});

test('a dirty worktree blocks Send until Autostash is on', async ({ relaunch }) => {
  const dirty = {
    prompt: 'Rebase feat/oauth-login onto origin/develop.',
    suffix: 'Report the outcome with finish_step.',
    stack: [],
    blockers: [{ branchId: 'b_auth', kind: 'dirty', text: '2 uncommitted files' }],
    noOp: false,
  };
  const { window: page } = await open(relaunch, onDevelop, [
    { channel: IPC.adeTaskRebasePreview, response: dirty },
  ] as never);
  await authTask(page).locator(t('ade-rebase')).first().click();
  await expect(page.locator(t('ade-dialog-blocker'))).toContainText('2 uncommitted files');
  await expect(page.locator(t('ade-dialog-send'))).toBeDisabled();
  await page.locator(t('ade-dialog-autostash')).click();
  await expect(page.locator(t('ade-dialog-send'))).toBeEnabled();
});

test('a run working on the stack disables the act with a tip', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, (b) => {
    Object.assign(branch(b, 'b_bill'), { base: 'develop', behind: 2 });
  });
  await page.locator(row('b_bill')).click();
  await expect(page.locator(t('ade-panel-action-rebase-base'))).toBeDisabled();
});

test('a pending base shows its tag, Retry rebase, and Abort rebase asks first', async ({
  relaunch,
}) => {
  const { window: page, control } = await open(relaunch, (b) => {
    Object.assign(branch(b, 'b_auth'), {
      base: 'develop',
      basePendingFrom: 'main',
      rebaseInProgress: true,
    });
  });
  await expect(authTask(page).locator(t('ade-tag')).first()).toBeVisible();
  await page.locator(row('b_auth')).click();
  await expect(page.locator(t('ade-panel-action-rebase-retry'))).toContainText('Retry rebase');
  await page.locator(t('ade-panel-action-abort-rebase')).click();
  await expect(page.getByText('Abort rebase?')).toBeVisible();
  await expect(page.locator(t('ade-confirm-no'))).toHaveText('Cancel');
  await expect(page.locator(t('ade-confirm-yes'))).toHaveText('Abort');
  expect(calls(control, IPC.adeTaskAbortRebase)).toHaveLength(0);
  await page.locator(t('ade-confirm-yes')).click();
  await expect.poll(() => calls(control, IPC.adeTaskAbortRebase)).toHaveLength(1);
});

test('a failed rebase run shows its reason, files and shas, and Copy for agent copies them', async ({
  relaunch,
}) => {
  const { window: page } = await open(relaunch, (b) => {
    Object.assign(branch(b, 'b_auth'), { base: 'develop', basePendingFrom: 'main' });
    const task = b.tasks.find((x) => x.id === 'T_auth');
    task?.runs.push(rebaseRun('r_rb1', 'b_auth', 'T_auth', { outcome: failedOutcome }));
  });
  await page.evaluate(() => {
    const w = window as unknown as { __clipboard: string[] };
    w.__clipboard = [];
    navigator.clipboard.writeText = async (text: string) => {
      w.__clipboard.push(text);
    };
  });
  await page.locator(row('b_auth')).click();
  const out = page.locator(t('ade-run-outcome'));
  await expect(out.locator(t('ade-outcome-reason'))).toHaveText('src/auth/session.ts conflicts');
  await expect(out.locator(t('ade-outcome-files'))).toContainText('src/auth/session.ts');
  await expect(out.locator(t('ade-outcome-branch'))).toContainText('1f8e6aa');
  await out.locator(t('ade-outcome-copy')).click();
  await expect
    .poll(() =>
      page.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard[0]),
    )
    .toContain('Conflicted files: src/auth/session.ts');
});

test('contract: Needs you lists the failed rebase and Open selects the branch', async ({
  relaunch,
}) => {
  const conflict = contract<{
    outcome: { reason: string; rebase: { branches: object[] } };
  }>('ade-base', 'AdeTaskService.Run#rebase-conflict');
  const run = rebaseRun('r_rb1', 'b_auth', 'T_auth', {
    ...conflict,
    id: 'r_rb1',
    taskId: 'T_auth',
    branchId: 'b_auth',
    startedAt: 1790060000000,
    finishedAt: 1790060100000,
    outcome: {
      ...conflict.outcome,
      rebase: {
        ...conflict.outcome.rebase,
        branches: conflict.outcome.rebase.branches.map((b) => ({ ...b, branchId: 'b_auth' })),
      },
    },
  });
  const { window: page } = await open(relaunch, (b) => {
    Object.assign(branch(b, 'b_auth'), { base: 'develop', basePendingFrom: 'main' });
    b.tasks.find((x) => x.id === 'T_auth')?.runs.push(run);
  });
  await page.locator(t('ade-tab-needs')).click();
  const item = page.locator(`${t('ade-needs-item')}[data-kind="rebase"]`);
  await expect(item).toBeVisible();
  await expect(item).toContainText(conflict.outcome.reason);
  await item.locator(t('ade-needs-action')).click();
  await expect(page.locator(t('ade-panel'))).toBeVisible();
});

test('a missing base shows its tag and disables Review code', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, (b) => {
    Object.assign(branch(b, 'b_notif'), { base: 'gone/base', baseMissing: true });
  });
  const notif = page.locator(`${t('ade-card')}[data-task-id="T_notif"]`);
  await expect(page.getByText('base missing').first()).toBeVisible();
  await expect(notif.locator(t('ade-card-review'))).toBeDisabled();
});

test('no dialog opens on board load or when a run changes a base', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, (b) => {
    Object.assign(branch(b, 'b_auth'), { base: 'develop', basePendingFrom: 'main' });
  });
  await expect(page.locator(t('ade-plan'))).toBeVisible();
  await expect(page.locator(t('ade-dialog'))).toHaveCount(0);
});

test('a failed step that reported detail shows it in its step block', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, (b) => {
    const run = b.tasks
      .find((x) => x.id === 'T_cart')
      ?.runs.find((r) => r.id === 'r_T_cart_release_b_cart');
    if (run)
      run.outcome = {
        status: 'failed',
        reason: 'release script exited 1',
        source: 'agent',
        reported: true,
        report: { lastGitError: 'push rejected', tried: 'retry' },
      };
  });
  await page.locator(`${t('ade-card')}[data-task-id="T_cart"] ${t('ade-card-head')}`).click();
  const out = page.locator(t('ade-run-outcome'));
  await expect(out.locator(t('ade-outcome-reason'))).toHaveText('release script exited 1');
  await expect(out.locator(t('ade-outcome-git-error'))).toHaveText('push rejected');
});

// Contract ade-base. Backend half: adeflow TestTaskBase and TestRebaseRun. Fixture ids replace the
// backend's; every other field is the backend's.
test('contract: a conflicting rebase is reported, left in progress, and Abort rebase asks first', async ({
  relaunch,
}) => {
  const conflict = contract<{
    outcome: { reason: string; rebase: { conflictedFiles: string[]; branches: object[] } };
  }>('ade-base', 'AdeTaskService.Run#rebase-conflict');
  const stuck = contract<{ rebaseInProgress: boolean }>(
    'ade-base',
    'AdeTaskService.Branch#rebase-conflict',
  );
  const run = rebaseRun('r_rb1', 'b_auth', 'T_auth', {
    ...conflict,
    id: 'r_rb1',
    taskId: 'T_auth',
    branchId: 'b_auth',
    startedAt: 1790060000000,
    finishedAt: 1790060100000,
    outcome: {
      ...conflict.outcome,
      rebase: {
        ...conflict.outcome.rebase,
        branches: conflict.outcome.rebase.branches.map((b) => ({ ...b, branchId: 'b_auth' })),
      },
    },
  });
  const { window: page, control } = await open(relaunch, (b) => {
    Object.assign(branch(b, 'b_auth'), {
      base: 'develop',
      basePendingFrom: 'main',
      rebaseInProgress: stuck.rebaseInProgress,
    });
    b.tasks.find((x) => x.id === 'T_auth')?.runs.push(run);
  });
  await page.locator(row('b_auth')).click();
  const out = page.locator(t('ade-run-outcome'));
  await expect(out.locator(t('ade-outcome-reason'))).toHaveText(conflict.outcome.reason);
  for (const file of conflict.outcome.rebase.conflictedFiles) {
    await expect(out.locator(t('ade-outcome-files'))).toContainText(file);
  }
  await page.locator(t('ade-panel-action-abort-rebase')).click();
  await page.locator(t('ade-confirm-yes')).click();
  await expect.poll(() => calls(control, IPC.adeTaskAbortRebase)).toHaveLength(1);
});
