import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import {
  dbItem,
  editorOverflows,
  exemptionItem,
  LONG_SQL,
  PLAN_OVER,
  READ_SQL,
  recorded,
  scrollBodyToEnd,
  WRITE_SQL,
  withPayload,
} from './support/approvalItems';
import { editorText } from './support/editorText';

// Contract dbmcp. Backend half: dbmcpflow TestDbMcpApprovalFlow. A write from an AI client parks in
// the approvals dialog; Approve answers the item id.

test('contract: a pending write shows connection, operation and statement; Approve answers by item id', async ({
  relaunch,
}) => {
  const [item] = contract<ApprovalItem[]>('dbmcp', 'ApprovalsService.List');
  const answer = contract<{ ids: string[] }>('dbmcp', 'args:ApprovalsService.Answer');
  if (!item) throw new Error('contract has no item');
  const payload = item.payload as { statement: string };
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });

  await pushApprovals(page, [liveItem(item)]);
  const card = page.locator(`[data-testid="approval-item-${item.id}"]`);
  const statement = page.locator('[data-testid="db-mcp-approval-statement"]');
  const name = card.locator('[data-testid="approval-target-name"]');
  await expect(name).toHaveText(item.target.name);
  await expect(card.locator('[data-testid="approval-target-icon"]')).toBeVisible();
  // The engine icon names the kind: no engine word beside the location.
  await expect(card.locator('[data-testid="approval-target-kind"]')).toHaveText(
    item.target.location,
  );
  await expect(card.locator('[data-testid="approval-operation"]')).toHaveText('Write');
  await expect(card.locator('[data-testid="approval-fact"]')).toHaveCount(0);
  await expect(card.locator('[data-testid="approval-countdown"]')).toContainText('left');
  // Connection row first, then the badge row, then the statement box.
  const nameBox = await name.boundingBox();
  const lineBox = await card.locator('[data-testid="approval-line"]').boundingBox();
  const statementBox = await statement.boundingBox();
  expect(nameBox?.y).toBeLessThan(lineBox?.y ?? 0);
  expect(lineBox?.y).toBeLessThan(statementBox?.y ?? 0);
  // Details start closed and hold who asked.
  const origin = page.locator('[data-testid="approval-origin"]');
  await expect(origin).toBeHidden();
  await page.click('[data-testid="approval-details-toggle"]');
  await expect(origin).toContainText(item.origin.client);
  const formatted = await editorText(statement);
  expect(formatted).toContain('\n');
  expect(formatted.replace(/\s+/g, ' ').trim()).toBe(payload.statement);
  await expect(page.locator('[data-testid="db-mcp-approval-view-as-sent"]')).toHaveText('Raw');
  await page.click('[data-testid="db-mcp-approval-view-as-sent"]');
  await expect.poll(() => editorText(statement)).toBe(payload.statement);
  const box = await page.locator('[data-testid="approvals-dialog"]').boundingBox();
  expect(box?.width).toBe(900);
  await page.click(`[data-testid="approval-approve-${item.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(answer);
});

test('contract: the footer buttons are the stock variants, chords shown, no resting fill', async ({
  relaunch,
}) => {
  const item = recorded('dbmcp');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [item]);
  const approve = page.locator(`[data-testid="approval-approve-${item.id}"]`);
  const deny = page.locator(`[data-testid="approval-deny-${item.id}"]`);
  await expect(approve).toHaveAttribute('data-variant', 'dialog-approve');
  await expect(deny).toHaveAttribute('data-variant', 'dialog-deny');
  await expect(approve.locator('[data-shortcut="form.submit"]')).toBeVisible();
  await expect(deny.locator('[data-shortcut="prompt.deny"]')).toBeVisible();
  const background = (el: import('@playwright/test').Locator) =>
    el.evaluate((e) => getComputedStyle(e).backgroundColor);
  expect(await background(approve)).toBe('rgba(0, 0, 0, 0)');
  expect(await background(deny)).toBe('rgba(0, 0, 0, 0)');
  await approve.hover();
  await expect.poll(() => background(approve)).not.toBe('rgba(0, 0, 0, 0)');
  // Review result sits in the right button group, before Deny.
  const group = page.locator('[data-testid="approval-actions"]');
  await expect(group.locator('[data-testid="db-mcp-approval-review-result"]')).toBeVisible();
  const reviewBox = await group
    .locator('[data-testid="db-mcp-approval-review-result"]')
    .boundingBox();
  const denyBox = await deny.boundingBox();
  expect(reviewBox?.x).toBeLessThan(denyBox?.x ?? 0);
});

test('contract: the colour line sits under the dialog title, as wide as the content', async ({
  relaunch,
}) => {
  const item = dbItem({ cls: 'read', sql: READ_SQL, enforcement: 'session', color: 'red' });
  const { window: page } = await relaunch({});
  await page.setViewportSize({ width: 1280, height: 900 });
  await pushApprovals(page, [item]);
  const line = page.locator('[data-testid="approvals-dialog"] [data-testid="approvals-conn-line"]');
  await expect(line).toHaveAttribute('style', /var\(--kira-conn-red\)/);
  expect(await line.evaluate((el) => el.closest('[data-slot="dialog-header"]') !== null)).toBe(
    true,
  );
  const edges = async () => {
    const l = await line.boundingBox();
    const target = await page.locator('[data-testid="approval-target"]').boundingBox();
    const box = await page.locator('[data-testid="approval-body"]').boundingBox();
    return { l, target, box };
  };
  let { l, target, box } = await edges();
  expect(Math.abs((l?.x ?? 0) - (target?.x ?? 99))).toBeLessThanOrEqual(1);
  expect(
    Math.abs((l?.x ?? 0) + (l?.width ?? 0) - ((box?.x ?? 0) + (box?.width ?? 0))),
  ).toBeLessThanOrEqual(1);
  // A body that scrolls keeps the line over the content, not over the scrollbar.
  await pushApprovals(page, [
    dbItem({ cls: 'read', sql: LONG_SQL, enforcement: 'session', color: 'red' }),
  ]);
  await expect(page.locator('[data-testid="approvals-footer"]')).toHaveAttribute(
    'data-more-below',
    'true',
  );
  ({ l, target, box } = await edges());
  expect(Math.abs((l?.x ?? 0) - (target?.x ?? 99))).toBeLessThanOrEqual(1);
  expect(
    Math.abs((l?.x ?? 0) + (l?.width ?? 0) - ((box?.x ?? 0) + (box?.width ?? 0))),
  ).toBeLessThanOrEqual(1);
  // Colour "none" draws no line.
  await pushApprovals(page, [dbItem({ cls: 'read', sql: READ_SQL, color: 'none' })]);
  await expect(line).toHaveCount(0);
});

const tooltipText = async (
  page: import('@playwright/test').Page,
  target: import('@playwright/test').Locator,
) => {
  await target.hover();
  const tip = page.locator('[data-slot="tooltip-content"]').first();
  await expect(tip).toBeVisible();
  return (await tip.textContent()) ?? '';
};

test('contract: a read shows one badge with a lock, the enforcement in its tooltip', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({});
  await pushApprovals(page, [dbItem({ cls: 'read', sql: READ_SQL, enforcement: 'session' })]);
  const op = page.locator('[data-testid="approval-operation"]');
  await expect(op).toHaveText('Read');
  await expect(op).toHaveClass(/text-status-green/);
  await expect(op.locator('[data-testid="approval-operation-lock"]')).toBeVisible();
  await expect(page.locator('[data-testid="approval-fact"]')).toHaveCount(0);
  expect(await tooltipText(page, op)).toContain('database');

  // A connection without a write path (Kafka): same lock, its own wording.
  await pushApprovals(page, [
    dbItem({ cls: 'read', sql: READ_SQL, enforcement: 'none', kind: 'kafka', name: 'events' }),
  ]);
  await expect(op.locator('[data-testid="approval-operation-lock"]')).toBeVisible();
  await expect(page.locator('[data-testid="approval-fact"]')).toHaveCount(0);
  await page.mouse.move(0, 0);
  expect(await tooltipText(page, op)).toContain('no write path');
});

test('contract: Kira-checked reads keep their own badge, no lock', async ({ relaunch }) => {
  const { window: page } = await relaunch({});
  await pushApprovals(page, [
    dbItem({
      cls: 'read',
      sql: '{ "find": "orders" }',
      enforcement: 'app',
      kind: 'mongodb',
      name: 'orders',
    }),
  ]);
  const op = page.locator('[data-testid="approval-operation"]');
  await expect(op).toHaveText('Read');
  await expect(op.locator('[data-testid="approval-operation-lock"]')).toHaveCount(0);
  const fact = page.locator('[data-testid="approval-fact"]');
  await expect(fact).toHaveText('Checked by Kira');
  await expect(fact).toHaveClass(/text-status-orange/);
  expect(await tooltipText(page, fact)).toContain('not the database');
});

test('contract: a read-only connection adds its line to the tooltip and a Details row', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({});
  await pushApprovals(page, [
    dbItem({ cls: 'read', sql: READ_SQL, enforcement: 'session', readOnly: true }),
  ]);
  await expect(page.locator('[data-testid="approval-fact"]')).toHaveCount(0);
  expect(await tooltipText(page, page.locator('[data-testid="approval-operation"]'))).toContain(
    'The connection is also set read-only',
  );
  await page.click('[data-testid="approval-details-toggle"]');
  await expect(page.locator('[data-testid="approval-details-section"]')).toContainText(
    'Set read-only',
  );
});

test('contract: kinds have their own hue; Delete and Schema change are not Write', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({});
  const op = page.locator('[data-testid="approval-operation"]');
  await pushApprovals(page, [dbItem({ cls: 'write', sql: WRITE_SQL })]);
  await expect(op).toHaveClass(/text-status-amber/);
  await pushApprovals(page, [dbItem({ cls: 'ddl', sql: 'drop table customers' })]);
  await expect(op).toHaveText('Schema change');
  await expect(op).toHaveClass(/text-status-purple/);
  await pushApprovals(page, [withPayload(recorded('dbmcp-explain-approval'), {})]);
  await expect(op).toHaveText('Explain');
  await expect(op).toHaveClass(/text-status-blue/);
});

test('contract: explain asks show the Explain badge, no extra fact and no Review result', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({});
  await pushApprovals(page, [recorded('dbmcp-explain-approval')]);
  await expect(page.locator('[data-testid="approval-operation"]')).toHaveText('Explain');
  await expect(page.locator('[data-testid="approval-fact"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="db-mcp-approval-review-result"]')).toHaveCount(0);
});

test('contract: a plan frames the warnings; a clean plan adds nothing', async ({ relaunch }) => {
  const { window: page } = await relaunch({});
  const rows = page.locator('[data-testid="db-mcp-approval-plan-rows"]');
  const issues = page.locator('[data-testid="db-mcp-approval-plan-issue"]');
  const clean = { ...PLAN_OVER, estimatedRowsRead: 4200, overThreshold: false, issues: [] };
  await pushApprovals(page, [
    dbItem({ cls: 'read', sql: READ_SQL, enforcement: 'session', plan: clean }),
  ]);
  await expect(page.locator('[data-testid="approval-operation"]')).toBeVisible();
  await expect(rows).toHaveCount(0);
  await expect(issues).toHaveCount(0);

  const warn = {
    ...PLAN_OVER,
    estimatedRowsRead: 48_000,
    overThreshold: false,
    issues: [{ severity: 'warn', code: 'seq-scan', message: 'Seq Scan on customers' }],
  };
  await pushApprovals(page, [
    dbItem({ cls: 'read', sql: READ_SQL, enforcement: 'session', plan: warn }),
  ]);
  await expect(rows).toHaveText('Estimated ~48,000 rows, under the 100,000 row limit');
  await expect(issues).toHaveText('Seq Scan on customers');

  await pushApprovals(page, [
    dbItem({ cls: 'read', sql: READ_SQL, enforcement: 'session', plan: PLAN_OVER }),
  ]);
  await expect(rows).toHaveText('Estimated ~1,240,000 rows, over the 100,000 row limit');
  // Info issues read neutral, warn issues amber.
  await expect(issues.nth(0)).toHaveClass(/text-warn/);
  await expect(issues.nth(1)).not.toHaveClass(/text-warn/);
});

test('contract: a lifted limit leads with the reason and keeps a Read-only badge', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({});
  await pushApprovals(page, [exemptionItem({ sql: READ_SQL, plan: PLAN_OVER })]);
  await expect(page.locator('[data-testid="approval-operation"]')).toHaveText('Lift limit');
  await expect(page.locator('[data-testid="approval-fact"]')).toHaveText('Read-only');
  const signals = page.locator('[data-testid="approval-signals"] li');
  await expect(signals.first()).toContainText('The monthly churn report needs every order');
  await expect(page.locator('[data-testid="db-mcp-exemption-limit"]')).toHaveText(
    'Estimated 1,240,000 rows, over the 100,000 row limit (explain ran before asking)',
  );
});

test('contract: a long statement scrolls the body, wraps, and shows the divider only while more is below', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({});
  await page.setViewportSize({ width: 1280, height: 900 });
  await pushApprovals(page, [
    dbItem({ cls: 'read', sql: LONG_SQL, enforcement: 'session', plan: PLAN_OVER }),
  ]);
  const footer = page.locator('[data-testid="approvals-footer"]');
  await expect(footer).toHaveAttribute('data-more-below', 'true');
  await page.click('[data-testid="approval-details-toggle"]');
  const end = await scrollBodyToEnd(page);
  expect(end.scrollable).toBe(true);
  expect(end.scrollTop).toBeGreaterThan(0);
  await expect(footer).toHaveAttribute('data-more-below', 'false');
  for (const overflow of await editorOverflows(page)) expect(overflow).toBeLessThanOrEqual(1);
  await expect(page.locator('[data-testid="approval-details-toggle"]')).toBeInViewport();
  await expect(page.locator('[data-testid="approval-origin"]')).toBeInViewport();
});

test('contract: the statement is syntax highlighted', async ({ relaunch }) => {
  const { window: page } = await relaunch({});
  await pushApprovals(page, [dbItem({ cls: 'read', sql: READ_SQL, enforcement: 'session' })]);
  const tokens = page.locator(
    '[data-testid="db-mcp-approval-statement"] .monaco-editor .view-line span[class*="mtk"]',
  );
  await expect
    .poll(async () => new Set(await tokens.evaluateAll((els) => els.map((e) => e.className))).size)
    .toBeGreaterThan(1);
});

test('contract: single keys do nothing; Mod+Backspace denies by item id', async ({ relaunch }) => {
  const [item] = contract<ApprovalItem[]>('dbmcp', 'ApprovalsService.List');
  if (!item) throw new Error('contract has no item');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([item.id]) }],
  });

  await pushApprovals(page, [liveItem(item)]);
  await expect(page.locator(`[data-testid="approval-deny-${item.id}"]`)).not.toBeFocused();
  await page.keyboard.press('Enter');
  await page.keyboard.press('Space');
  expect(control.log().some((e) => e.channel === IPC.approvalsAnswer)).toBe(false);
  await expect(page.locator('[data-testid="approvals-dialog"]')).toHaveAttribute(
    'data-chords-armed',
    'true',
  );
  await page.keyboard.press('Meta+Backspace');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [item.id], approve: false, options: null });
});
