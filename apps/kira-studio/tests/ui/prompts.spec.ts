import type { ApprovalItem } from '@shared/domain/approvals';
import type { RoutedPrompt } from '@shared/domain/prompts';
import { liveItem } from '@workbench/testing/ui/approvals';
import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// P246: the window shows the popup the Go router targeted at it, one at a time. The Go half is
// flows/promptflow; here the router's pushes are mocked.

const approvalItem = (): ApprovalItem => {
  const [item] = contract<ApprovalItem[]>('dbmcp', 'ApprovalsService.List');
  if (!item) throw new Error('contract has no item');
  return liveItem(item);
};

// The router's approval entry (flows/dbmcpflow TestDbMcpApprovalRoutes, contract dbmcp-route), with
// this spec's item id and the window key of the UI suite.
function approvalEntry(over: Partial<RoutedPrompt> = {}): RoutedPrompt {
  const entry = contract<RoutedPrompt>('dbmcp-route', 'PromptsService.List#approval');
  const { id } = approvalItem();
  return { ...entry, id: `approval:${id}`, ref: id, target: 'main', ...over };
}

const UPDATE = {
  channel: IPC.updateStatus,
  response: {
    updateAvailable: true,
    currentVersion: '1.2.0',
    latestVersion: '1.3.0',
    installLogPath: '/tmp/kira-studio-update.log',
  },
} as const;

test('contract: an MCP approval shows only in the window the router targets', async ({ kira }) => {
  const { window } = kira;
  const dialog = window.locator('[data-testid="approvals-dialog"]');
  await emitWailsEvent(window, IPC.approvalsChanged, [approvalItem()]);

  await emitPrompts(window, [approvalEntry({ target: 'other-window' })]);
  await expect(dialog).toHaveCount(0);
  await emitPrompts(window, [approvalEntry()]);
  await expect(dialog).toBeVisible();
  await emitPrompts(window, []);
  await expect(dialog).toHaveCount(0);
});

test('contract: a prompt on screen turns the approvals dialog into the toast', async ({
  relaunch,
}) => {
  const { window } = await relaunch({ control: [UPDATE] });
  const approvals = window.locator('[data-testid="approvals-dialog"]');
  const toast = window.locator('[data-testid="approvals-toast"]');
  const updateDialog = window.locator('[data-testid="update-dialog"]');
  await emitWailsEvent(window, IPC.approvalsChanged, [approvalItem()]);
  await emitPrompts(window, [approvalEntry({ createdAt: 1 })]);
  await expect(approvals).toBeVisible({ timeout: 15_000 });

  await emitPrompts(window, [approvalEntry({ createdAt: 1 }), routed('update', '1.3.0')]);
  await expect(updateDialog).toBeVisible({ timeout: 15_000 });
  await expect(approvals).toHaveCount(0);
  await expect(toast).toBeVisible();

  await emitPrompts(window, [approvalEntry({ createdAt: 1 })]);
  await expect(approvals).toBeVisible();
  await expect(toast).toHaveCount(0);
});

test('Send test notification calls the bound method', async ({ kira }) => {
  const { window, control } = kira;
  await emitWailsEvent(window, IPC.openSettings, undefined);
  await window.locator('[data-testid="settings-section-Advanced"]').click();
  await expect(window.locator('[data-testid="settings-notify-prompts"]')).toHaveAttribute(
    'aria-checked',
    'true',
  );
  await window.locator('[data-testid="settings-notify-prompts-test"]').click();
  await expect.poll(() => control.log().some((e) => e.channel === IPC.promptsSendTest)).toBe(true);
});
