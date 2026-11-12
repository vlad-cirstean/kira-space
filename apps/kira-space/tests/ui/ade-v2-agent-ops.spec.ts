import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// Agent ops the backend records (branch requests, approval wait, remote tools) show on the task card
// as a chip while running, and toast when they finish past 2 s. Backend half: adeflow
// TestAgentBranchRequest and the agentmcp op records.

const t = (id: string) => `[data-testid="${id}"]`;
const card = (page: Page) => page.locator('[data-testid="ade-card"][data-task-id="T_cart"]');
const chip = (page: Page) => card(page).locator(t('ade-op-status'));
const toast = (page: Page) => page.locator('[data-sonner-toast]');

function opRecord(overrides: Record<string, unknown>): Record<string, unknown> {
  return {
    id: 'op-1',
    startedAt: new Date().toISOString(),
    durationMs: null,
    kind: 'agentBranches',
    status: 'running',
    command: 'api, web',
    error: null,
    category: 'agent',
    module: 'ade',
    trigger: 'agent',
    subject: 'Cart checkout',
    repoRoot: '',
    repoName: '',
    source: 'Kira Space',
    taskId: 'T_cart',
    parentId: '',
    background: false,
    repeat: 1,
    steps: [],
    stepsDropped: 0,
    cancellable: false,
    ...overrides,
  };
}

const push = (page: Page, record: Record<string, unknown>) =>
  emitWailsEvent(page, IPC.opUpdate, record);

async function open(relaunch: Parameters<typeof openPlan>[0]) {
  const { window: page } = await openPlan(relaunch, [{ channel: IPC.opsRecent, response: [] }]);
  return page;
}

test('a running branch request shows a chip on the card until it finishes', async ({
  relaunch,
}) => {
  const page = await open(relaunch);
  await push(page, opRecord({}));
  await expect(chip(page)).toHaveAttribute('data-op', 'agentBranches');
  await push(page, opRecord({ status: 'ok', durationMs: 400 }));
  await expect(chip(page)).toHaveCount(0);
  await expect(toast(page)).toHaveCount(0);
});

test('the approval wait is its own chip and never toasts', async ({ relaunch }) => {
  const page = await open(relaunch);
  await push(page, opRecord({ id: 'op-ap', kind: 'agentApproval' }));
  await expect(chip(page)).toHaveAttribute('data-op', 'agentApproval');
  await push(
    page,
    opRecord({
      id: 'op-ap',
      kind: 'agentApproval',
      status: 'cancelled',
      error: 'denied',
      durationMs: 9000,
    }),
  );
  await expect(chip(page)).toHaveCount(0);
  await expect(toast(page)).toHaveCount(0);
});

test('a remote tool call mirrors as a push chip', async ({ relaunch }) => {
  const page = await open(relaunch);
  await push(page, opRecord({ id: 'op-push', category: 'mcp', kind: 'push_branch' }));
  await expect(chip(page)).toHaveAttribute('data-op', 'agentPush');
});

test('a finish past 2 s toasts success; a slow failure toasts the error', async ({ relaunch }) => {
  const page = await open(relaunch);
  await push(page, opRecord({}));
  await push(page, opRecord({ status: 'ok', durationMs: 3000 }));
  await expect(toast(page)).toContainText('Created branches for Cart checkout');
  await push(page, opRecord({ id: 'op-2', kind: 'agentRename' }));
  await push(
    page,
    opRecord({
      id: 'op-2',
      kind: 'agentRename',
      status: 'error',
      error: 'remote refused the rename',
      durationMs: 3000,
    }),
  );
  await expect(toast(page).filter({ hasText: 'remote refused the rename' })).toBeVisible();
  await expect(chip(page)).toHaveCount(0);
});
