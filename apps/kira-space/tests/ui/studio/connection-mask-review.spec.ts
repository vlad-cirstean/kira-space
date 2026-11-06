import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import type { MaskReviewListing } from '@shared/domain/maskReview';
import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectionRow } from './support/tree';

// Column review in the connection Privacy tab and the masking prompt. Backend half: dbmcpflow
// TestDbMcpColumnReview, whose dbmcp-mask-review contract pins the listing below.

const FILE = 'local.yaml';
const dlg = '[data-testid="connection-file-dialog"]';
const rows = '[data-testid="connection-privacy-review-row"]';

const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');
const listing = () =>
  contract<MaskReviewListing>('dbmcp-mask-review', 'MaskRulesService.ReviewColumns');

type Relaunch = Parameters<Parameters<typeof test>[2]>[0]['relaunch'];

interface Draft {
  kind?: string;
  reviewed?: string[];
  masks?: { table?: string; column: string; kind: string }[];
  offline?: boolean;
}

function control(d: Draft) {
  const kind = d.kind ?? 'sqlite';
  const lines = ['name: Local', `kind: ${kind}`, 'mcp:', '  enabled: true'];
  const value: Record<string, unknown> = { name: 'Local', kind, mcp: { enabled: true } };
  if (d.reviewed) {
    lines.push('mask_review:', '  columns:', ...d.reviewed.map((x) => `    - ${x}`));
    value.mask_review = { columns: d.reviewed };
  }
  if (d.masks) {
    lines.push('masks:', ...d.masks.map((m) => `  - column: ${m.column}\n    kind: ${m.kind}`));
    value.masks = d.masks;
  }
  const text = `${lines.join('\n')}\n`;
  const base = contract<ConnectionFileDetail>('connection-form', 'file#read');
  const detail = { ...base, file: FILE, id: list()[0]?.id ?? '', text };
  return [
    { channel: IPC.connectionsList, response: list() },
    { channel: IPC.connectionConfigView, response: view() },
    { channel: IPC.connectionConfigFile, response: detail },
    { channel: IPC.connectionConfigForm, response: { problem: null, text, value } },
    {
      channel: IPC.maskReviewColumns,
      response: d.offline ? { ...listing(), applicable: false, columns: [] } : listing(),
    },
    { channel: IPC.maskReviewPending, response: { count: 2 } },
  ];
}

async function open(relaunch: Relaunch, d: Draft = {}) {
  const { window: page } = await relaunch({ control: control(d) });
  await (await connectionRow(page, 'Local')).click({ button: 'right' });
  await page.click('[data-testid="menu-item-edit"]');
  await expect(page.locator(dlg)).toBeVisible();
  await page.locator('[data-testid="connection-tab-privacy"]').click();
  return page;
}

const box = (page: Awaited<ReturnType<typeof open>>, ref: string) =>
  page.locator(`${rows}[data-ref="${ref}"] [data-testid="connection-privacy-review-reviewed"]`);

test('contract: rows show masked, reviewed and needs-review states from the draft', async ({
  relaunch,
}) => {
  const page = await open(relaunch, {
    reviewed: ['notes.id'],
    masks: [{ column: 'body', kind: 'redact' }],
  });
  const all = listing().columns;
  await page.locator('[data-testid="connection-privacy-review-filter"]').selectOption('all');
  await expect(page.locator(rows)).toHaveCount(all.length);
  await expect(page.locator(`${rows}[data-ref="notes.id"]`)).toHaveAttribute(
    'data-state',
    'reviewed',
  );
  await expect(page.locator(`${rows}[data-ref="notes.body"]`)).toHaveAttribute(
    'data-state',
    'masked',
  );
  await expect(page.locator(`${rows}[data-ref="notes.secret"]`)).toHaveAttribute(
    'data-state',
    'pending',
  );
  await expect(box(page, 'notes.body')).toBeDisabled();
  await expect(page.locator('[data-testid="connection-privacy-review-status"]')).toContainText(
    'need review',
  );
  const enforce = page.locator('[data-testid="connection-privacy-review-enforce"]');
  await expect(enforce).toContainText('every 30 seconds');
  await expect(enforce).toContainText('dropped from results');
  await expect(enforce).toContainText('No exemption applies');
});

test('the default filter shows what needs review; search narrows the list', async ({
  relaunch,
}) => {
  const page = await open(relaunch, { reviewed: ['notes.id'] });
  await expect(page.locator(rows)).toHaveCount(2);
  await page.locator('[data-testid="connection-privacy-review-filter"]').selectOption('all');
  await page.locator('[data-testid="connection-privacy-review-search"]').fill('secret');
  await expect(page.locator(rows)).toHaveCount(1);
});

test('ticking a row reviews one column; confirm all reviews the rest', async ({ relaunch }) => {
  const page = await open(relaunch, { reviewed: ['notes.id'] });
  await box(page, 'notes.body').click();
  await expect(page.locator(rows)).toHaveCount(1);
  await page.locator('[data-testid="connection-privacy-review-confirm-all"]').click();
  await expect(page.locator('[data-testid="connection-privacy-review-status"]')).toContainText(
    'All columns reviewed',
  );
  await expect(box(page, 'notes.secret')).toHaveAttribute('aria-checked', 'true');
});

test('turning the switch on records every live column as reviewed', async ({ relaunch }) => {
  const page = await open(relaunch);
  const on = page.locator('[data-testid="connection-privacy-review-on"]');
  await expect(on).toHaveAttribute('aria-checked', 'false');
  await expect(page.locator('[data-testid="connection-privacy-review-status"]')).toContainText(
    'is off',
  );
  await on.click();
  await expect(on).toHaveAttribute('aria-checked', 'true');
  await expect(page.locator('[data-testid="connection-privacy-review-status"]')).toContainText(
    'All columns reviewed',
  );
});

test('turning the switch off asks first', async ({ relaunch }) => {
  const page = await open(relaunch, { reviewed: ['notes.id'] });
  const on = page.locator('[data-testid="connection-privacy-review-on"]');
  await on.click();
  await expect(page.locator('[data-testid="confirm-dialog"]')).toContainText('unmasked');
  await page.click('[data-testid="confirm-dialog-confirm"]');
  await expect(on).toHaveAttribute('aria-checked', 'false');
});

test('Mask prefills the add-rule form', async ({ relaunch }) => {
  const page = await open(relaunch, { reviewed: ['notes.id'] });
  await page
    .locator(`${rows}[data-ref="notes.secret"] [data-testid="connection-privacy-review-mask"]`)
    .click();
  await expect(page.locator('[data-testid="mask-rule-add-table"]')).toHaveValue('notes');
  await expect(page.locator('[data-testid="mask-rule-add-column"]')).toHaveValue('secret');
});

test('an unreachable server says to connect and keeps the saved entries', async ({ relaunch }) => {
  const page = await open(relaunch, { reviewed: ['notes.id'], offline: true });
  await expect(page.locator('[data-testid="connection-privacy-review-offline"]')).toBeVisible();
  await expect(page.locator(rows)).toHaveCount(0);
});

test('a kind without columns says review does not apply', async ({ relaunch }) => {
  const page = await open(relaunch, { kind: 'redis' });
  await expect(page.locator('[data-testid="connection-privacy-review-na"]')).toBeVisible();
  await expect(page.locator('[data-testid="connection-privacy-review-on"]')).toHaveCount(0);
});

test('the masking prompt shows the count; Review masking opens the Privacy tab', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: control({ reviewed: ['notes.id'] }) });
  const id = list()[0]?.id ?? '';
  await emitPrompts(page, [
    routed('masking', id, { title: 'New columns on Local need a masking review' }),
  ]);
  const prompt = page.locator('[data-testid="masking-review-dialog"]');
  await expect(prompt).toContainText('2 new columns are withheld');
  await page.locator('[data-testid="masking-review-open"]').click();
  await expect(page.locator(dlg)).toBeVisible();
  await expect(page.locator('[data-testid="connection-section-privacy"]')).toBeVisible();
  await expect(page.locator('[data-testid="connection-privacy-review"]')).toBeVisible();
});
