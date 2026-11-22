import type { Locator, Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { box, left, near, right, TOL } from '../support/approvalGeometry';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { withTimes } from './support/keptTimes';

// P726: an approval card shows its sections as separate boxes, and typed-value rows share one
// label / field / choice grid. Layout only: the payloads are the recorded contract items, edited here
// where a shape is missing. No backend half (no payload or IPC change).

test.use({ viewport: { width: 1400, height: 900 } });

const t = (id: string) => `[data-testid="${id}"]`;
const BOX = '[data-approval-box]';

async function allNear(ls: Locator, measure: (l: Locator) => Promise<number>, what: string) {
  const n = await ls.count();
  expect(n, `${what}: count`).toBeGreaterThan(0);
  const first = await measure(ls.first());
  for (let i = 1; i < n; i++) near(await measure(ls.nth(i)), first, `${what} ${i}`);
}

const USER_VALUE = 'e2e-user-value';
const KUBE = 'e2e-kube-script';
const userValue = (key: string) =>
  contract<ApprovalItem[]>(USER_VALUE, `ApprovalsService.List#${key}`)[0] as ApprovalItem;

interface Input {
  name: string;
  keepMinutes: number;
  origin: string;
  state: string;
}
const INPUT_SPECS: Pick<Input, 'name' | 'keepMinutes'>[] = [
  { name: 'pin', keepMinutes: 5 },
  { name: 'otp', keepMinutes: 60 },
  { name: 'partner_key_for_the_staging_tenant', keepMinutes: 5 },
  { name: 'key', keepMinutes: 60 },
];
function inputsOf(count: number, state = 'needed'): Input[] {
  return INPUT_SPECS.slice(0, count).map((s) => ({
    ...s,
    origin: 'http://127.0.0.1:1',
    state,
  }));
}
function withInputs(it: ApprovalItem, inputs: Input[]): ApprovalItem {
  const payload = structuredClone(it.payload) as { inputs: Input[] };
  payload.inputs = inputs;
  return liveItem({ ...it, payload });
}
const card = (page: Page, it: ApprovalItem) => page.locator(t(`approval-item-${it.id}`));

async function boxIds(c: Locator): Promise<string[]> {
  return c
    .locator(t('approval-body'))
    .locator(BOX)
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-testid') ?? ''));
}

for (const count of [1, 2, 3, 4]) {
  test(`value rows align: ${count} row(s), mixed name lengths`, async ({ relaunch }) => {
    const it = withInputs(userValue('needed'), inputsOf(count));
    const { window: page } = await relaunch({ control: [] });
    await pushApprovals(page, [it]);
    const panel = card(page, it).locator(t('api-approval-inputs'));
    await expect(panel).toBeVisible();
    const rows = panel.locator('[data-testid^="api-approval-input-"]');
    await expect(rows).toHaveCount(count);
    await allNear(panel.locator('[data-testid^="api-input-"][id]'), left, 'field left');
    await allNear(panel.locator('[data-testid^="api-input-"][id]'), right, 'field right');
    await allNear(panel.locator('label'), left, 'label left');
    const labels = panel.locator('label');
    const fields = panel.locator('[data-testid^="api-input-"][id]');
    for (let i = 0; i < count; i++) {
      expect(await right(labels.nth(i))).toBeLessThanOrEqual((await left(fields.nth(i))) + TOL);
    }
    if (count === 1)
      expect(await left(fields.first())).toBeGreaterThan(await right(labels.first()));
  });
}

test('an opened stored row puts its field in the same column', async ({ relaunch }) => {
  const needed = inputsOf(2);
  const stored: Input = {
    name: 'saved',
    keepMinutes: 5,
    origin: 'http://127.0.0.1:1',
    state: 'stored',
  };
  const it = withInputs(userValue('needed'), [needed[0] as Input, stored]);
  const { window: page } = await relaunch({ control: [] });
  await pushApprovals(page, [it]);
  const c = card(page, it);
  await c.locator(`${t('api-approval-input-replace')} [data-slot="collapsible-trigger"]`).click();
  const content = c.locator(`${t('api-approval-input-replace')} [data-slot="collapsible-content"]`);
  await expect(content).toBeVisible();
  expect(await content.evaluate((e) => getComputedStyle(e).display)).toBe('grid');
  const needField = c.locator(t(`api-input-${needed[0]?.name}`));
  const storedField = c.locator(t('api-input-saved'));
  near(await left(storedField), await left(needField), 'stored field left');
  near(await right(storedField), await right(needField), 'stored field right');
});

test('API card: kept, request, values, prepare, transform boxes in order, 8px apart', async ({
  relaunch,
}) => {
  const base = withTimes(
    contract<ApprovalItem[]>(
      'approvals-api-kept-script',
      'ApprovalsService.List',
    )[0] as ApprovalItem,
  );
  const it = withInputs(base, inputsOf(2));
  const { window: page } = await relaunch({ control: [] });
  await pushApprovals(page, [it]);
  const c = card(page, it);
  await expect(c.locator(t('api-approval-box-transform'))).toBeVisible();
  expect(await boxIds(c)).toEqual([
    'api-approval-box-kept',
    'api-approval-box-request',
    'api-approval-box-inputs',
    'api-approval-box-prepare',
    'api-approval-box-transform',
  ]);
  const dialog = page.locator(t('approvals-dialog'));
  const titleX = await left(dialog.locator('[data-slot="dialog-title"]'));
  const boxes = c.locator(t('approval-body')).locator(BOX);
  const n = await boxes.count();
  const firstRight = await right(boxes.first());
  for (let i = 0; i < n; i++) {
    near(await left(boxes.nth(i)), titleX, `box ${i} left`);
    near(await right(boxes.nth(i)), firstRight, `box ${i} right`);
    if (i > 0) {
      const prev = await box(boxes.nth(i - 1));
      near((await box(boxes.nth(i))).y - (prev.y + prev.height), 8, `gap before box ${i}`);
    }
  }
  for (const out of ['approval-signals', 'approval-details-section', 'approval-operation']) {
    const l = c.locator(t(out));
    if (await l.count())
      expect(await l.locator(`xpath=ancestor::*[@data-approval-box]`).count()).toBe(0);
  }
});

test('API card with no kept, values or script has the request box only', async ({ relaunch }) => {
  const it = liveItem(userValue('needed'));
  const payload = structuredClone(it.payload) as { kept: unknown[]; inputs: unknown[] };
  payload.kept = [];
  payload.inputs = [];
  const plain = liveItem({ ...it, payload });
  const { window: page } = await relaunch({ control: [] });
  await pushApprovals(page, [plain]);
  await expect(card(page, plain).locator(t('api-approval-box-request'))).toBeVisible();
  expect(await boxIds(card(page, plain))).toEqual(['api-approval-box-request']);
});

test('DB query: kept, statement with parameters, result script boxes', async ({ relaunch }) => {
  const base = contract<ApprovalItem[]>(
    'dbmcp-query-script-kept',
    'ApprovalsService.List',
  )[0] as ApprovalItem;
  const payload = structuredClone(base.payload) as { params: unknown[]; placeholders: string[] };
  payload.params = ['42'];
  payload.placeholders = ['$1'];
  const it = withTimes({ ...base, payload } as ApprovalItem);
  const { window: page } = await relaunch({ control: [] });
  await pushApprovals(page, [it]);
  const c = card(page, it);
  await expect(c.locator(t('db-approval-box-transform'))).toBeVisible();
  expect(await boxIds(c)).toEqual([
    'db-approval-box-kept',
    'db-approval-box-statement',
    'db-approval-box-transform',
  ]);
  await expect(
    c.locator(`${t('db-approval-box-statement')} ${t('db-approval-params')}`),
  ).toBeVisible();
});

test('kube read: kept, request, prepare, transform boxes', async ({ relaunch }) => {
  const it = withTimes(
    contract<ApprovalItem[]>(KUBE, 'ApprovalsService.List#logs')[0] as ApprovalItem,
  );
  const { window: page } = await relaunch({ control: [] });
  await pushApprovals(page, [it]);
  const c = card(page, it);
  await expect(c.locator(t('kube-approval-box-transform'))).toBeVisible();
  expect(await boxIds(c)).toEqual([
    'kube-approval-box-kept',
    'kube-approval-box-request',
    'kube-approval-box-prepare',
    'kube-approval-box-transform',
  ]);
});

test('scripted kube review: request, result, script and Will save boxes', async ({ relaunch }) => {
  const it = liveItem(
    contract<ApprovalItem[]>(KUBE, 'ApprovalsService.List#review')[0] as ApprovalItem,
  );
  const held = contract<Record<string, unknown>>(KUBE, 'DbMcpService.PendingResponse#review');
  const { window: page } = await relaunch({
    control: [{ channel: IPC.dbMcpPendingResponse, response: held }],
  });
  await pushApprovals(page, [it]);
  const c = card(page, it);
  await expect(c.locator(t('kube-review-box-keep'))).toBeVisible();
  expect(await boxIds(c)).toEqual([
    'kube-review-box-result',
    'kube-review-box-script',
    'kube-review-box-keep',
    'kube-review-box-request',
  ]);
  await expect(
    c.locator(`${t('kube-review-box-result')} [data-slot="secondary-tabs"]`),
  ).toBeVisible();
});

test('script runs and proof reviews keep the single approval-body box', async ({ relaunch }) => {
  const origin = {
    client: 'Claude Code 2.1.318',
    cwd: '~/shop',
    task: '',
    step: '',
    source: '',
    session: '',
  };
  const target = { name: 'x', kind: '', color: '', readOnly: false, location: '' };
  const mk = (id: string, kind: string, operation: ApprovalItem['operation'], payload: unknown) =>
    liveItem({
      id,
      kind,
      createdAt: 0,
      expiresAt: 0,
      origin,
      target,
      operation,
      reason: 'permission',
      payload,
    });
  const items = [
    mk(
      'script.run:7',
      'script.run',
      { class: 'run', tone: 'caution' },
      {
        scriptId: 's1',
        description: '',
        command: './scripts/reindex.sh --table orders',
        cwd: '~/shop/ops',
        params: [{ name: 'table', value: 'orders', secret: false, saved: false, fromVar: '' }],
        env: [],
      },
    ),
    mk(
      'proof.review:8',
      'proof.review',
      { class: 'edit', tone: 'caution' },
      {
        sessionId: 'p1',
        taskId: 't1',
        slug: 'q4-runbook',
        name: 'Q4 runbook',
        revision: 3,
        reason: 'Step 2 limit too low',
        steps: [
          { ordinal: 2, description: 'Replace retry count', diff: '- retries: 3\n+ retries: 5' },
        ],
      },
    ),
  ];
  const { window: page } = await relaunch({ control: [] });
  for (const it of items) {
    await pushApprovals(page, [it]);
    const c = card(page, it);
    await expect(c.locator(t('approval-body'))).toBeVisible();
    await expect(c.locator(BOX)).toHaveCount(1);
    await expect(c.locator(t('approval-body'))).toHaveAttribute('data-approval-box', '');
  }
});
