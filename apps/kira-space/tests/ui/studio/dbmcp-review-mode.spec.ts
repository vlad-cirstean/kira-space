import type { ApprovalItem } from '@shared/domain/approvals';
import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectionRow } from './support/tree';

// Contract dbmcp-review-mode. Backend half: dbmcpflow TestDbMcpReviewPerMode, TestDbMcpReviewOnRequest,
// TestDbMcpReviewOnRequestExemption and TestDbMcpReviewOnRequestAlreadyOn. Result review per mode in
// the MCP tab, and the review checkbox on approval prompts.

const FILE = 'local.yaml';
const dlg = '[data-testid="connection-file-dialog"]';
const KEY = 'dbmcp-review-mode';

const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');
const items = (key: string) => contract<ApprovalItem[]>(KEY, `ApprovalsService.List#${key}`);
const card = (page: import('@playwright/test').Page, it: ApprovalItem) =>
  page.locator(`[data-testid="approval-item-${it.id}"]`);
const box = '[data-testid="db-mcp-approval-review-result"]';

async function openTab(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  kind: string,
  mcp: Record<string, unknown>,
) {
  const lines = ['name: Local', `kind: ${kind}`, 'mcp:'];
  for (const [k, v] of Object.entries(mcp)) {
    if (typeof v === 'object' && v) {
      lines.push(`  ${k}:`, ...Object.entries(v).map(([a, b]) => `    ${a}: ${b}`));
    } else lines.push(`  ${k}: ${v}`);
  }
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

const checked = (page: import('@playwright/test').Page, mode: string, value: string) =>
  page.locator(`[data-testid="connection-mcp-review-${mode}-${value}"]`);

test('changing one mode saves the per-mode key', async ({ relaunch }) => {
  const { page, control } = await openTab(relaunch, 'sqlite', {
    enabled: true,
    review: { read: 'review', write: 'review', ddl: 'review' },
  });
  await checked(page, 'read', 'send').click();
  await expect
    .poll(() => {
      const call = control
        .log()
        .filter((e) => e.channel === IPC.connectionConfigForm)
        .at(-1)?.args as { edits: { path: string[]; value?: unknown; unset: boolean }[] };
      return call?.edits.map(
        (e) => `${e.path.join('.')}=${e.unset ? 'unset' : JSON.stringify(e.value)}`,
      );
    })
    .toEqual(['mcp.review.read="send"']);
});

test('write and ddl rows are disabled on a kind with no write console', async ({ relaunch }) => {
  const { page } = await openTab(relaunch, 'kafka', { enabled: true });
  await expect(checked(page, 'read', 'send')).toBeEnabled();
  await expect(checked(page, 'write', 'send')).toBeDisabled();
  await expect(checked(page, 'ddl', 'send')).toBeDisabled();
});

test('the enforcement lines state who enforces, what denial does and the limits', async ({
  relaunch,
}) => {
  const { page } = await openTab(relaunch, 'sqlite', { enabled: true });
  const enforce = page.locator('[data-testid="connection-mcp-review-enforce"]');
  await expect(enforce).toContainText('Kira enforces review in the app');
  await expect(enforce).toContainText('no answer in 10 minutes');
  await expect(enforce).toContainText('only in memory');
  await expect(enforce).toContainText('over 8 MB is withheld unseen');
  await expect(enforce).toContainText('Withhold does not undo a write');
  await expect(enforce).toContainText('Only run_query and browse_table results are reviewed');
  await expect(enforce).toContainText('cover the query, not your review');
});

test('contract: a query item offers the checkbox, a tick rides the approval', async ({
  relaunch,
}) => {
  const [it] = items('query');
  if (!it) throw new Error('contract has no query item');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([it.id]) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  const c = card(page, it);
  await expect(c.locator(box)).toBeVisible();
  await expect(c.locator(box)).toHaveAttribute('aria-checked', 'false');
  await c.locator(box).click();
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: { [it.id]: { reviewResult: true } } });
});

test('contract: a call the settings already review shows the box on and locked', async ({
  relaunch,
}) => {
  const [it] = items('on');
  if (!it) throw new Error('contract has no on item');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const c = card(page, it);
  await expect(c.locator(box)).toHaveAttribute('aria-checked', 'true');
  await expect(c.locator(box)).toBeDisabled();
  await expect(c).toContainText('This connection reviews read results.');
});

test('contract: an exemption item offers the same checkbox', async ({ relaunch }) => {
  const [it] = items('exemption');
  if (!it) throw new Error('contract has no exemption item');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([it.id]) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  const c = card(page, it);
  await expect(c.locator(box)).toBeVisible();
  await c.locator(box).click();
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: { [it.id]: { reviewResult: true } } });
});

test('contract: a requested result says so, and leaves Approve all', async ({ relaunch }) => {
  const [it] = items('requested');
  if (!it) throw new Error('contract has no requested item');
  const [other] = items('query');
  if (!other) throw new Error('contract has no query item');
  const held = contract('dbmcp-review-connection', 'DbMcpService.PendingResponse');
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: held },
      { channel: IPC.approvalsAnswer, response: answeredOk([it.id, other.id]) },
    ],
  });
  await pushApprovals(page, [liveItem(it), liveItem(other)]);
  await expect(card(page, it).locator('[data-testid="db-mcp-preview-body"]')).toContainText(
    'You asked to check its result before the AI gets it.',
  );
  await expect(page.locator('[data-testid="approvals-approve-all"]')).toHaveText('Approve all (1)');
  await page.click('[data-testid="approvals-deny-all"]');
  await expect
    .poll(() => {
      const args = control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args as
        | { ids: string[]; approve: boolean }
        | undefined;
      return args && { ...args, ids: [...args.ids].sort() };
    })
    .toMatchObject({ ids: [it.id, other.id].sort(), approve: false });
});
