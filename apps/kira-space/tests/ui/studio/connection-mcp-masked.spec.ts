import type { ApprovalItem } from '@shared/domain/approvals';
import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectionRow } from './support/tree';

// Contract dbmcp-browse. Backend half: dbmcpflow TestDbMcpBrowsePrompt and the other TestDbMcpBrowse*
// flows. The masked-read row of the MCP tab and its approval item.

const FILE = 'local.yaml';
const dlg = '[data-testid="connection-file-dialog"]';
const KEY = 'dbmcp-browse';

const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');

async function openTab(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  kind: string,
  mcp: Record<string, unknown>,
) {
  const lines = ['name: Local', `kind: ${kind}`, 'mcp:'];
  for (const [k, v] of Object.entries(mcp)) lines.push(`  ${k}: ${v}`);
  const text = `${lines.join('\n')}\n`;
  const base = contract<ConnectionFileDetail>('connection-form', 'file#read');
  const detail = { ...base, file: FILE, id: list()[0]?.id ?? '', text };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: list() },
      { channel: IPC.connectionConfigView, response: view() },
      { channel: IPC.connectionConfigFile, response: detail },
      {
        channel: IPC.connectionConfigForm,
        response: { problem: null, text, value: { name: 'Local', kind, mcp } },
      },
    ],
  });
  await (await connectionRow(page, 'Local')).click({ button: 'right' });
  await page.click('[data-testid="menu-item-edit"]');
  await expect(page.locator(dlg)).toBeVisible();
  await page.locator('[data-testid="connection-tab-mcp"]').click();
  return { page, control };
}

const masked = (page: import('@playwright/test').Page, value: string) =>
  page.locator(`[data-testid="connection-mcp-masked-${value}"]`);

function lastEdits(control: { log(): { channel: string; args: unknown }[] }) {
  const call = control
    .log()
    .filter((e) => e.channel === IPC.connectionConfigForm)
    .at(-1)?.args as { edits: { path: string[]; value?: unknown; unset: boolean }[] } | undefined;
  return call?.edits.map(
    (e) => `${e.path.join('.')}=${e.unset ? 'unset' : JSON.stringify(e.value)}`,
  );
}

test('the row sits above the mode rows and inherit names the read mode', async ({ relaunch }) => {
  const { page } = await openTab(relaunch, 'sqlite', { enabled: true, read: 'deny' });
  const row = page.locator('[data-testid="connection-mcp-masked"]');
  const readRow = page.locator('[data-testid="connection-mcp-read"]');
  await expect(row).toContainText('Read masked data');
  const [rowBox, readBox] = [await row.boundingBox(), await readRow.boundingBox()];
  expect(rowBox?.y ?? 0).toBeLessThan(readBox?.y ?? 0);
  await expect(masked(page, 'inherit')).toHaveAttribute('aria-pressed', 'true');
  await expect(masked(page, 'inherit')).toContainText('Same as read (Deny)');
});

test('the inherit label follows the read mode of the file', async ({ relaunch }) => {
  const { page } = await openTab(relaunch, 'sqlite', { enabled: true, read: 'prompt' });
  await expect(masked(page, 'inherit')).toContainText('Same as read (Ask me)');
});

test('Allow saves mcp.masked, Same as read removes the key', async ({ relaunch }) => {
  const first = await openTab(relaunch, 'sqlite', { enabled: true });
  await masked(first.page, 'allow').click();
  await expect.poll(() => lastEdits(first.control)).toContain('mcp.masked="allow"');
});

test('Same as read unsets a stored value', async ({ relaunch }) => {
  const { page, control } = await openTab(relaunch, 'sqlite', { enabled: true, masked: 'allow' });
  await expect(masked(page, 'allow')).toHaveAttribute('aria-pressed', 'true');
  await masked(page, 'inherit').click();
  await expect.poll(() => lastEdits(control)).toContain('mcp.masked=unset');
});

test('a kind without structured browse disables the row and says why', async ({ relaunch }) => {
  const { page } = await openTab(relaunch, 'mongodb', { enabled: true });
  for (const v of ['inherit', 'deny', 'allow', 'prompt']) {
    await expect(masked(page, v)).toBeDisabled();
  }
  await expect(page.locator('[data-testid="connection-mcp-masked-na"]')).toContainText(
    'no structured browse',
  );
  await expect(page.locator('[data-testid="connection-mcp-masked-enforce"]')).toHaveCount(0);
});

test('the enforcement lines state who enforces, what a violation does and the limits', async ({
  relaunch,
}) => {
  const { page } = await openTab(relaunch, 'postgres', { enabled: true });
  const enforce = page.locator('[data-testid="connection-mcp-masked-enforce"]');
  await expect(enforce).toContainText('builds every browse statement itself');
  await expect(enforce).toContainText('runs it read-only');
  await expect(enforce).toContainText('is refused with the reason');
  await expect(enforce).toContainText('Masked columns only take is-empty filters');
  await expect(enforce).toContainText('lift that limit once');
  await expect(enforce).toContainText('Masking still matches by column name');
  await expect(enforce).toContainText('engines with no estimate (SQLite) skip it');
  await expect(enforce).toContainText('Engines without tables');
});

test('the review section gets a masked row, disabled on a kind without browse', async ({
  relaunch,
}) => {
  const first = await openTab(relaunch, 'sqlite', { enabled: true });
  await expect(
    first.page.locator('[data-testid="connection-mcp-review-masked-send"]'),
  ).toBeEnabled();
  const second = await openTab(relaunch, 'mongodb', { enabled: true });
  await expect(
    second.page.locator('[data-testid="connection-mcp-review-masked-send"]'),
  ).toBeDisabled();
});

test('contract: a browse item names the masked read and offers the review checkbox', async ({
  relaunch,
}) => {
  const [it] = contract<ApprovalItem[]>(KEY, 'ApprovalsService.List#browse');
  if (!it) throw new Error('contract has no browse item');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([it.id]) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  const card = page.locator(`[data-testid="approval-item-${it.id}"]`);
  await expect(card.locator('[data-testid="db-mcp-approval-mode-masked"]')).toContainText(
    'Read masked data',
  );
  await expect(card).toContainText('SELECT');
  const box = card.locator('[data-testid="db-mcp-approval-review-result"]');
  await expect(box).toBeVisible();
  await box.click();
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: { [it.id]: { reviewResult: true } } });
});
