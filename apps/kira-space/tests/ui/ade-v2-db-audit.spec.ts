import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, archivedTaskFx, FIXED_NOW, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// Kira Studio audit trail in the Details list. Contract ade-db-audit; backend half: flows/adeflow
// TestDbAuditTrail. The contract masks ids and times, so the specs fill them in.

const t = (id: string) => `[data-testid="${id}"]`;
const MINUTE = 60_000;
const STORAGE_KEY = 'kira.ade.details.dbAudit';

interface Approval {
  id: string;
  outcome: string;
  subject: string;
  raisedAt: number;
  resolvedAt: number;
  [key: string]: unknown;
}
interface Entry {
  id: string;
  sessionId: string;
  startedAt: number;
  durationMs: number;
  statement: string;
  statementWithheld: boolean;
  statementLen: number;
  approvals: Approval[];
  [key: string]: unknown;
}

// Newest first, as the backend lists them; each row a minute older than the last.
function entries(mutate: (e: Entry[]) => void = () => undefined): Entry[] {
  const fx = contract<{ entries: Entry[] }>('ade-db-audit', 'AdeTaskService.DbAudit#list');
  const out = fx.entries.map((e, i) => {
    const at = FIXED_NOW - (i + 1) * MINUTE;
    return {
      ...e,
      id: `c_${i}`,
      sessionId: 'sp22',
      startedAt: at,
      durationMs: 12,
      approvals: e.approvals.map((a, j) => ({
        ...a,
        id: `a_${i}_${j}`,
        raisedAt: at,
        resolvedAt: at + 5,
      })),
    };
  });
  mutate(out);
  return out;
}

function snapshots(list: Entry[] = entries()): ControlSnapshot[] {
  const board = adeBoard((b) => {
    const task = b.tasks.find((x) => x.id === 'T_alerts');
    if (task) task.createdAt = FIXED_NOW - 10 * 60 * MINUTE;
  });
  return [
    { channel: IPC.adeTaskBoard, response: board },
    { channel: IPC.adeTaskDbAudit, response: { entries: list } },
  ];
}

const toggle = (page: Page) => page.locator(t('ade-details-dbaudit-toggle'));
const rows = (page: Page) => page.locator(t('ade-details-dbaudit-entry'));

async function openDetails(page: Page): Promise<void> {
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-task-chain'))).toBeVisible();
}

test('the toggle is on by default and counts the actions', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  await expect(toggle(page)).toHaveAttribute('aria-pressed', 'true');
  await expect(toggle(page)).toContainText('Kira Studio (7)');
  await expect(rows(page)).toHaveCount(7);
  const times = await rows(page).evaluateAll((els) =>
    els.map((e) => e.getAttribute('data-status')),
  );
  expect(times).toEqual(['refused', 'refused', 'refused', 'ok', 'error', 'ok', 'ok']);
});

test('contract: statuses and approval outcomes match the backend', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  const approvals = page.locator(t('ade-details-dbaudit-approval'));
  await expect
    .poll(() => approvals.evaluateAll((els) => els.map((e) => e.getAttribute('data-outcome'))))
    .toEqual(['denied', 'denied', 'approved']);
  const exemption = approvals.filter({ hasText: 'Exemption' }).first();
  await expect(exemption).toHaveAttribute('data-subject', 'db.exemption');
  await expect(exemption).toContainText('expensive_query');
  await expect(exemption).toContainText('Reason:');
});

test('statements show masked, can expand, and a withheld one says so', async ({ relaunch }) => {
  const list = entries((e) => {
    e[2].statement = '';
    e[2].statementWithheld = true;
    e[2].statementLen = 42;
  });
  const { window: page } = await openPlan(relaunch, snapshots(list));
  await openDetails(page);
  const first = page.locator(t('ade-details-dbaudit-statement')).first();
  await expect(first).toContainText("'…'");
  await expect(first).not.toContainText('@');
  const expand = page.locator(t('ade-details-dbaudit-expand')).first();
  await expand.click();
  await expect(expand).toHaveText('Show less');
  await expect(page.locator(t('ade-details-dbaudit-withheld'))).toContainText(
    'Statement not recorded (42 characters).',
  );
});

test('the choice persists across a reload and a relaunch', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  await toggle(page).click();
  await expect(rows(page)).toHaveCount(0);
  await page.reload();
  await page.locator(t('ade-plan')).waitFor();
  await openDetails(page);
  await expect(toggle(page)).toHaveAttribute('aria-pressed', 'false');
  await expect(rows(page)).toHaveCount(0);

  const seeded = await openPlan(
    async (o) => relaunch({ ...o, storage: { [STORAGE_KEY]: 'false' } }),
    snapshots(),
  );
  await openDetails(seeded.window);
  await expect(toggle(seeded.window)).toHaveAttribute('aria-pressed', 'false');
  await expect(rows(seeded.window)).toHaveCount(0);
});

test('without audit entries the toggle is absent', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, snapshots([]));
  await openDetails(page);
  await expect(toggle(page)).toHaveCount(0);
  await expect(rows(page)).toHaveCount(0);
});

test('a dbAudit push refetches the trail', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  await expect.poll(() => calls(control, IPC.adeTaskDbAudit).length).toBeGreaterThan(0);
  const before = calls(control, IPC.adeTaskDbAudit).length;
  await emitWailsEvent(page, IPC.adeTaskDbAuditChanged, { taskId: 'T_alerts' });
  await expect
    .poll(() => calls(control, IPC.adeTaskDbAudit).length, { timeout: 15_000 })
    .toBeGreaterThan(before);
});

test('an archived task shows the trail read-only', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    ...snapshots(),
    { channel: IPC.adeTaskArchivedTask, response: archivedTaskFx() },
  ]);
  await page.locator(t('ade-archived-find')).click();
  await page.locator(`${t('ade-archived-find-item')}[data-task-id="H_1"]`).click();
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(rows(page).first()).toBeVisible();
  await expect(page.locator(t('ade-note-add'))).toHaveCount(0);
});

test('an audit row opens the Sessions tab on its session', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  await rows(page).first().locator(t('ade-origin-session')).click();
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();
  await expect(page.locator(t('ade-session-tab-sp22'))).toHaveAttribute('data-selected', 'true');
});
