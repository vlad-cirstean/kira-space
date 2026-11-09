import type { Locator, Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest, test } from './fixtures';
import { FIXED_NOW, openPlan } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// Every kind of the approval dialog lines up on the same grid (P589 A1-A11): one left edge, one
// content indent, one baseline per line 1, details on a fixed column. Each check is within 1px.

const TOL = 1;
const t = (id: string) => `[data-testid="${id}"]`;

async function box(l: Locator) {
  const b = await l.boundingBox();
  if (!b) throw new Error(`no box for ${l}`);
  return b;
}
const left = async (l: Locator) => (await box(l)).x;
const right = async (l: Locator) => {
  const b = await box(l);
  return b.x + b.width;
};
const midY = async (l: Locator) => {
  const b = await box(l);
  return b.y + b.height / 2;
};
// The alphabetic baseline of an element's first text line: the first visible character is wrapped
// with a zero-size inline-block probe that sits on the baseline, measured, then put back.
async function baseline(l: Locator): Promise<number> {
  return l.evaluate((el) => {
    const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
    let text: Text | null = null;
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      if ((n.textContent ?? '').trim() !== '') {
        text = n as Text;
        break;
      }
    }
    if (!text) throw new Error('no text to measure');
    const parent = text.parentNode as Node;
    const start = (text.textContent ?? '').search(/\S/);
    const first = start > 0 ? text.splitText(start) : text;
    const rest = first.splitText(1);
    const wrap = document.createElement('span');
    parent.insertBefore(wrap, first);
    wrap.appendChild(first);
    const probe = document.createElement('span');
    probe.style.cssText = 'display:inline-block;width:0;height:0;vertical-align:baseline';
    wrap.appendChild(probe);
    const y = probe.getBoundingClientRect().top;
    parent.insertBefore(first, wrap);
    wrap.remove();
    first.textContent = (first.textContent ?? '') + (rest.textContent ?? '');
    rest.remove();
    return y;
  });
}
function near(actual: number, expected: number, what: string) {
  expect(Math.abs(actual - expected), `${what}: ${actual} vs ${expected}`).toBeLessThanOrEqual(TOL);
}

const ORIGIN = { client: 'Claude Code 2.1.318', cwd: '~/shop', task: '', step: '', source: '' };

function mk(
  id: string,
  kind: string,
  target: Partial<ApprovalItem['target']>,
  operation: ApprovalItem['operation'],
  payload: unknown,
  seconds = 600,
): ApprovalItem {
  return liveItem(
    {
      id,
      kind,
      createdAt: 0,
      expiresAt: 0,
      origin: ORIGIN,
      target: { name: '', kind: '', color: '', readOnly: false, location: '', ...target },
      operation,
      reason: 'permission',
      payload,
    },
    seconds,
  );
}

const PLAN = {
  estimatedRowsRead: 48210,
  thresholdRows: 10000,
  overThreshold: true,
  issues: [],
  issuesOmitted: 0,
};
const DB = { connectionId: 'c1', enforcement: 'session', reviewResult: false };
const SHOP = {
  name: 'shop db',
  kind: 'postgres',
  color: 'green',
  location: 'db.internal · shop / public',
};

const RESULT = JSON.stringify({
  kind: 'tabular',
  rowCount: 50,
  returned: 6,
  columns: ['country', 'sessions'].map((name) => ({ name, dataType: 'String', typeClass: 'text' })),
  rows: [
    ['DE', '18204'],
    ['NL', '9771'],
    ['FR', '7430'],
    ['ES', '6110'],
    ['IT', '5990'],
    ['PL', '4021'],
  ],
  truncatedCells: 0,
});

function studioItems(): ApprovalItem[] {
  return [
    mk(
      'db.query:1',
      'db.query',
      SHOP,
      { class: 'write', tone: 'caution' },
      {
        ...DB,
        statement: "update orders set status = 'void' where created_at < '2025-01-01'",
        mode: 'write',
        detected: 'write',
        plan: PLAN,
        reviewOffered: true,
      },
      108,
    ),
    mk(
      'db.query:2',
      'db.query',
      { name: 'notes db', kind: 'sqlite', location: '/data/mcp.db' },
      { class: 'read', tone: 'neutral' },
      {
        ...DB,
        statement: 'select id, body from notes limit 50',
        mode: 'read',
        detected: 'read',
        plan: null,
        reviewOffered: false,
      },
      0,
    ),
    mk(
      'db.exemption:3',
      'db.exemption',
      SHOP,
      { class: 'read', tone: 'danger' },
      {
        ...DB,
        statement: 'select customer_id, sum(total) from orders group by 1',
        mode: 'read',
        detected: 'read',
        reason: 'Monthly revenue report needs the full scan',
        limits: [{ limit: 'statement_timeout', measured: 31000, max: 30000, unit: 'ms' }],
        plan: null,
        reviewOffered: true,
      },
    ),
    mk(
      'db.response:4',
      'db.response',
      { name: 'analytics', kind: 'clickhouse', color: 'violet', location: 'ch.internal · events' },
      { class: 'read', tone: 'neutral' },
      {
        connectionId: 'c2',
        statement:
          'select country, count() as sessions from events group by country order by 2 desc',
        isError: false,
        bytes: 100,
        alreadyRan: false,
        requested: false,
      },
    ),
    mk(
      'api.request:5',
      'api.request',
      { name: 'api.staging.acme.dev', kind: 'http', color: 'amber', location: 'Staging' },
      { class: 'request', tone: 'caution' },
      {
        method: 'POST',
        url: 'https://api.staging.acme.dev/v1/orders?dryRun=true',
        host: 'api.staging.acme.dev',
        environment: { id: 'e1', name: 'Staging', color: 'amber' },
        query: [{ name: 'dryRun', value: 'true' }],
        headers: [
          { name: 'Authorization', value: 'Bearer {{stagingToken}}', masked: true },
          { name: 'Content-Type', value: 'application/json', masked: false },
          { name: 'Idempotency-Key', value: '7f3c…e21', masked: false },
          { name: 'X-Trace', value: 'abc', masked: false },
          { name: 'X-Client', value: 'kira', masked: false },
        ],
        auth: null,
        body: {
          contentType: 'application/json',
          text: '{"orderId":4711,"items":[{"sku":"KS-22","qty":2}]}',
          truncated: false,
          form: null,
        },
        variables: [{ name: 'orderId', value: '4711', secret: false }],
        credentials: [{ name: 'stagingToken', hosts: ['api.staging.acme.dev'] }],
        followRedirects: false,
        timeoutSeconds: 30,
      },
      250,
    ),
    mk(
      'kube.read:6',
      'kube.read',
      { name: 'prod-eu', kind: 'kubernetes', location: 'k8s.prod.acme:6443 · payments' },
      { class: 'read', tone: 'neutral' },
      {
        context: 'prod-eu',
        server: 'k8s.prod.acme:6443',
        namespace: 'payments',
        operation: 'list',
        statement: 'pods -l app=checkout',
      },
      0,
    ),
    mk(
      'script.run:7',
      'script.run',
      { name: 'reindex orders', kind: 'script', location: '~/shop/ops' },
      { class: 'run', tone: 'caution' },
      {
        scriptId: 's1',
        description: '',
        command: './scripts/reindex.sh --table orders --concurrently',
        cwd: '~/shop/ops',
        params: [{ name: 'table', value: 'orders', secret: false, saved: false, fromVar: '' }],
        env: [{ name: 'PGPASSWORD', value: '', secret: true, saved: true, fromVar: '' }],
      },
      0,
    ),
    mk(
      'proof.review:8',
      'proof.review',
      { name: 'Q4 runbook' },
      { class: 'edit', tone: 'caution' },
      {
        sessionId: 'p1',
        name: 'Q4 runbook',
        revision: 3,
        reason: 'Step 2 limit too low for the batch job',
        steps: [
          { ordinal: 2, description: 'Replace retry count', diff: '- retries: 3\n+ retries: 5' },
        ],
      },
      0,
    ),
  ];
}

// The checks every item of either app shares.
async function checkItems(page: Page) {
  const dialog = page.locator(t('approvals-dialog'));
  const titleX = await left(dialog.locator('[data-slot="dialog-title"]'));
  const approveRights: number[] = [];
  const dtX: number[] = [];
  const ddX: number[] = [];
  const sections = dialog.locator('section[data-kind]');
  for (let n = 0; n < (await sections.count()); n++) {
    const item = sections.nth(n);
    const itemBox = await box(item);
    const kind = (await item.getAttribute('data-kind')) ?? String(n);
    const op = item.locator(t('approval-operation'));
    const approve = item.locator(`[data-testid^="approval-approve-"]`);
    // A1
    near(itemBox.x, titleX, `${kind} item left vs dialog title`);
    near(await left(op), titleX, `${kind} op left vs dialog title`);
    near(await left(item.locator(t('approval-origin'))), titleX, `${kind} requester left`);
    near(await right(approve), itemBox.x + itemBox.width, `${kind} approve right vs item right`);
    approveRights.push(await right(approve));
    const countdown = item.locator(t('approval-countdown'));
    if (await countdown.count())
      near(await right(countdown), await right(approve), `${kind} countdown right`);
    // A4: line 1 shares one baseline
    const base = await baseline(op);
    for (const sel of ['approval-target-name', 'approval-target-kind', 'approval-countdown']) {
      const l = item.locator(t(sel));
      if (await l.count()) near(await baseline(l), base, `${kind} ${sel} baseline`);
    }
    const risks = item.locator(`${t('approval-line')} span.inline-flex`);
    for (let i = 0; i < (await risks.count()); i++) {
      near(await baseline(risks.nth(i)), base, `${kind} risk ${i} baseline`);
    }
    // A5: band spans the item with the item gap above and below
    const band = item.locator(t('approval-band'));
    if (await band.count()) {
      const bb = await box(band);
      near(bb.x, itemBox.x, `${kind} band x`);
      near(bb.width, itemBox.width, `${kind} band width`);
      const line = await box(item.locator(t('approval-line')));
      near(bb.y - (line.y + line.height), 8, `${kind} gap above band`);
      const body = item.locator(t('approval-body'));
      if (await body.count())
        near((await box(body)).y - (bb.y + bb.height), 8, `${kind} gap below band`);
    }
    // A6: footer centres and gaps
    const deny = item.locator(`[data-testid^="approval-deny-"]`);
    const centre = await midY(approve);
    near(await midY(deny), centre, `${kind} deny centre`);
    near(await midY(item.locator(t('approval-origin'))), centre, `${kind} requester centre`);
    near((await left(approve)) - (await right(deny)), 6, `${kind} deny/approve gap`);
    for (const sel of ['db-mcp-approval-review-result', 'proof-review-open']) {
      const l = item.locator(t(sel));
      if (await l.count()) near(await midY(l), centre, `${kind} ${sel} centre`);
    }
    // A2/A3: content indent and details columns
    const body = item.locator(t('approval-body'));
    if (await body.count()) {
      near(await left(body.locator(':scope > *').first()), titleX + 12, `${kind} body indent`);
    }
    const dts = item.locator(`${t('approval-details')} > dt`);
    for (let i = 0; i < (await dts.count()); i++) {
      const dt = dts.nth(i);
      near(await left(dt), titleX + 12, `${kind} dt ${i} x`);
      dtX.push(await left(dt));
      const dd = dt.locator('xpath=following-sibling::dd[1]');
      ddX.push(await left(dd));
      near(await left(dd), titleX + 12 + 84, `${kind} dd ${i} x`);
      near(await baseline(dt), await baseline(dd), `${kind} dt ${i} baseline`);
    }
  }
  expect(new Set(approveRights.map(Math.round)).size).toBeLessThanOrEqual(1);
  for (const x of [...dtX, ...ddX]) expect(x).toBeGreaterThan(0);
}

studioTest.describe('approval dialog alignment (Studio)', () => {
  studioTest.use({ viewport: { width: 1400, height: 900 } });

  studioTest('every Studio kind lines up', async ({ relaunch }) => {
    const items = studioItems();
    const { window: page } = await relaunch({
      control: [
        {
          channel: IPC.dbMcpPendingResponse,
          response: { requestId: '4', text: RESULT, isError: false },
        },
      ],
    });
    await pushApprovals(page, items);
    const dialog = page.locator(t('approvals-dialog'));
    await expect(dialog).toBeVisible();
    await expect(page.locator(`${t('db-mcp-preview-row')}`).first()).toBeVisible();
    await expect(dialog.locator('.view-line').first()).toBeVisible();
    await page.waitForTimeout(500);
    await checkItems(page);

    const titleX = await left(dialog.locator('[data-slot="dialog-title"]'));
    const indent = titleX + 12;

    // A2: first glyph of editors, code blocks, table, ordinal
    const q1 = dialog.locator(t('approval-item-db.query:1'));
    near(await left(q1.locator('.view-line').first()), indent, 'query code x');
    const kube = dialog.locator(t('approval-item-kube.read:6'));
    near(await left(kube.locator(t('db-mcp-approval-kube'))), indent, 'kube code x');
    const script = dialog.locator(t('approval-item-script.run:7'));
    near(await left(script.locator(t('db-mcp-approval-script-command'))), indent, 'script code x');
    const resp = dialog.locator(t('approval-item-db.response:4'));
    near(
      await left(resp.locator(t('db-mcp-preview-column')).first()),
      indent,
      'table first column x',
    );
    const proof = dialog.locator(t('approval-item-proof.review:8'));
    near(await left(proof.locator('ol li > span').first()), indent, 'proof ordinal x');
    const api = dialog.locator(t('approval-item-api.request:5'));
    near(await left(api.locator(t('api-approval-url'))), indent, 'api url x');
    near(await left(api.locator('.view-line').first()), indent, 'api body x');

    // A3/A9: "+N more" starts at the pairs column it extends
    const more = api.locator(t('approval-more'));
    near(await left(more), await left(api.locator(t('api-approval-headers'))), 'headers +N more x');
    const rows = resp.locator(t('db-mcp-preview-more'));
    near(await left(rows), indent, 'table +N more rows x');

    // A8: tabs right column
    for (const item of [q1, resp]) {
      const tabs = item.locator('[data-slot="secondary-tabs"]');
      near(
        await right(tabs),
        (await box(item)).x + (await box(item)).width,
        'tabs right vs item right',
      );
      const first = item.locator('.view-line, [data-testid="db-mcp-preview-column"]').first();
      near(await midY(tabs), await midY(first), 'tabs centre vs first line');
      const content = item
        .locator('[data-testid="db-mcp-approval-statement"], [data-testid="db-mcp-preview-table"]')
        .first();
      expect(await left(tabs)).toBeGreaterThanOrEqual((await right(content)) - TOL);
    }

    // A11: flush editor
    for (const item of [q1, api]) {
      const colours = await item
        .locator('.monaco-editor, .monaco-editor-background')
        .evaluateAll((els) =>
          els.map((e) => `${e.className}=${getComputedStyle(e).backgroundColor}`),
        );
      expect(
        colours.filter((c) => !c.endsWith('=rgba(0, 0, 0, 0)')),
        'opaque editor layers',
      ).toEqual([]);
      const margin = item.locator('.monaco-editor .margin').first();
      if (await margin.count()) expect((await box(margin)).width).toBeLessThanOrEqual(TOL);
    }
    const stmt = q1.locator(t('db-mcp-approval-statement'));
    const fontSize = await q1
      .locator('.view-line')
      .first()
      .evaluate((e) => parseFloat(getComputedStyle(e).fontSize));
    const lines = await q1.locator('.view-line').count();
    near(
      (await box(stmt)).height,
      Math.min(lines, 12) * Math.round(1.5 * fontSize),
      'statement height',
    );
    near((await box(q1.locator('.view-line').first())).y, (await box(stmt)).y, 'first line top');

    // A7: countdown width stays put across ticks
    const cd = q1.locator(t('approval-countdown'));
    expect(await cd.evaluate((e) => getComputedStyle(e).fontVariantNumeric)).toContain(
      'tabular-nums',
    );
    const w1 = (await box(cd)).width;
    await page.waitForTimeout(1200);
    near((await box(cd)).width, w1, 'countdown width across ticks');
  });
});

test.describe('approval dialog alignment (Space)', () => {
  test.use({ viewport: { width: 1400, height: 900 } });

  test('branch and task requests line up', async ({ relaunch }) => {
    const [adopt] = contract<ApprovalItem[]>('ade-branch-adopt', 'ApprovalsService.List#adopt');
    const [create] = contract<ApprovalItem[]>(
      'ade-orchestrator',
      'ApprovalsService.List#taskCreate',
    );
    if (!adopt || !create) throw new Error('contracts have no items');
    const payload = adopt.payload as { items: Record<string, unknown>[] };
    const row = payload.items[0] as Record<string, unknown>;
    const branch: ApprovalItem = {
      ...adopt,
      payload: {
        ...payload,
        items: [
          { ...row, dirty: 3 },
          {
            ...row,
            id: 'c0i1',
            kind: 'checkout',
            repo: 'kira-web',
            branch: 'feat/login-redirect',
            holder: undefined,
            dirty: undefined,
          },
        ],
      },
    };
    const items = [liveItem(branch, 300, FIXED_NOW), liveItem(create, 570, FIXED_NOW)];
    const { window: page } = await openPlan(relaunch, seedApprovals(items));
    const dialog = page.locator(t('approvals-dialog'));
    await expect(dialog).toBeVisible();
    await page.waitForTimeout(500);
    await checkItems(page);

    const titleX = await left(dialog.locator('[data-slot="dialog-title"]'));
    const indent = titleX + 12;
    // A10: one grid for every row
    const boxes = dialog.locator(`${t(`approval-item-${branch.id}`)} [role="checkbox"]`);
    near(await left(boxes.first()), indent, 'branch checkbox x');
    const rows = dialog.locator('[data-testid^="ade-branch-request-item-"]');
    const reposX: number[] = [];
    const branchX: number[] = [];
    const tagX: number[] = [];
    for (let i = 0; i < (await rows.count()); i++) {
      const cells = rows.nth(i).locator(':scope > *');
      reposX.push(await left(cells.nth(1)));
      branchX.push(await left(cells.nth(2)));
      tagX.push(await left(cells.nth(3)));
      near(await midY(cells.nth(0)), await midY(cells.nth(1)), `row ${i} checkbox centre`);
    }
    for (const xs of [reposX, branchX, tagX])
      for (const x of xs) near(x, xs[0] ?? 0, 'branch grid column x');

    // A2: brief text starts at the content indent
    const brief = dialog.locator(`${t('ade-task-create-brief')} p`).first();
    near(await left(brief), indent, 'brief x');
  });
});
