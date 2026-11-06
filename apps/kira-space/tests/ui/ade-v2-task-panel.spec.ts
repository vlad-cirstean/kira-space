import { expect, test } from './fixtures';
import { adeFixture, archivedTaskFx, FIXED_NOW, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Task panel: Main, Details and Sessions tabs, link groups, timeline and session links. Link adding
// and editing: ade-v2-details-links.spec.ts. Contract ade-task-panel; backend half: flows/adeflow
// TestTaskPanelOrigins.

const t = (id: string) => `[data-testid="${id}"]`;

interface HandoverFx {
  id: string;
  taskId: string;
  sessionId: string;
  createdAt: number;
  updatedAt: number;
}
interface ArtifactFx {
  id: string;
  kind: string;
  title: string;
  url: string;
  source: string;
  sessionId: string;
  createdAt: number;
}

// The contract's session ids hydrate to <id:N> uuids; map them to sessions the fixtures know.
const SESSION = {
  '00000000-0000-4000-8000-000000000001': 'sp22',
  '00000000-0000-4000-8000-000000000002': 't2',
} as const;
const sessionOf = (id: string): string => SESSION[id as keyof typeof SESSION] ?? '';

function handovers(): HandoverFx[] {
  const res = contract<{ handovers: HandoverFx[] }>('ade-task-panel', 'AdeTaskService.Handovers');
  return res.handovers.map((h, i) => ({
    ...h,
    id: `h_${i}`,
    taskId: 'T_alerts',
    sessionId: sessionOf(h.sessionId),
    createdAt: FIXED_NOW - 5000 + i,
    updatedAt: FIXED_NOW - 5000 + i,
  }));
}

function artifacts(): { artifacts: ArtifactFx[]; gaps: never[] } {
  const res = contract<{ artifacts: ArtifactFx[] }>('ade-task-panel', 'AdeTaskService.Artifacts');
  return {
    gaps: [],
    artifacts: res.artifacts.map((a, i) => ({
      ...a,
      id: a.source === 'agent' || a.source === 'user' ? `a_${i}` : '',
      sessionId: sessionOf(a.sessionId),
      createdAt: FIXED_NOW - 4000 + i,
    })),
  };
}

function sessions(): { sessions: Record<string, unknown>[] } {
  const all = adeFixture<{ sessions: Record<string, unknown>[] }>('sessions').sessions;
  const base = all.find((s) => s.id === 'sp22');
  if (!base) throw new Error('fixture session sp22 missing');
  return { sessions: [...all, { ...base, id: 't2', state: 'stopped' }] };
}

const panel = (): ControlSnapshot[] => [
  { channel: IPC.adeTaskHandovers, response: { handovers: handovers() } },
  { channel: IPC.adeTaskArtifacts, response: artifacts() },
  { channel: IPC.adeTaskSessions, response: sessions() },
];

async function openDetails(page: import('@playwright/test').Page): Promise<void> {
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-details-tab'))).toBeVisible();
}

const row = (page: import('@playwright/test').Page, text: string) =>
  page.locator(t('ade-details-link'), { hasText: text });

test('the tabs are Main, Details and Sessions, Main first', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, panel());
  await openTask(page, 'T_alerts');
  await expect(page.locator(t('ade-main-tab'))).toBeVisible();
  await expect(page.locator(t('ade-panel-tab-main'))).toHaveText(/Main/);
  await expect(page.locator(t('ade-panel-tab-details'))).toBeVisible();
  await expect(page.locator(t('ade-panel-tab-sessions'))).toBeVisible();
  await expect(page.locator('[data-testid^="ade-panel-tab-"]')).toHaveCount(3);
  await expect(page.locator(t('ade-panel-title'))).toBeVisible();
});

test('Main shows branches with their PR line and never handover text', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, panel());
  await openTask(page, 'T_bill');
  await expect(page.locator(t('ade-task-branch-pr'))).toHaveCount(2);
  await expect(page.locator(t('ade-task-branch-pr-base')).first()).toHaveText('→ main');
  const failed = page.locator(`${t('ade-task-branch-pr-checks')}[data-state="FAILURE"]`);
  await expect(failed).toHaveCount(1);
  await expect(page.locator(`${t('ade-task-branch')}[data-branch-id="b_billdash"]`)).toBeVisible();
  await expect(page.locator(`${t('ade-task-branch-pr')}[data-branch-id="b_billdash"]`)).toHaveCount(
    0,
  );
  await expect(page.locator(t('ade-notes-chain'))).toHaveCount(0);
});

test('Details groups the Jira and host links in Main and stored links in Related', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, panel());
  await openDetails(page);
  const main = page.locator(t('ade-details-main'));
  await expect(main.locator(t('ade-details-link'), { hasText: 'PAY-130' })).toHaveCount(1);
  await expect(main.locator(t('ade-details-link'), { hasText: 'Fix login' })).toHaveCount(1);
  const related = page.locator(t('ade-details-related'));
  await expect(related.locator(t('ade-details-link'), { hasText: 'Runbook' })).toHaveCount(1);
  const titles = page.locator(t('ade-details-link-title'));
  const count = await titles.count();
  for (let i = 0; i < count; i++) {
    const title = titles.nth(i);
    expect(await title.getAttribute('href')).toBeTruthy();
    expect((await title.textContent())?.trim()).not.toMatch(/^[#!]?\d+$/);
  }
});

test('the timeline starts closed and lists entries oldest first', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, panel());
  await openDetails(page);
  await expect(page.locator(t('ade-details-timeline-entry'))).toHaveCount(0);
  await page.locator(t('ade-details-timeline-toggle')).click();
  const kinds = await page
    .locator(t('ade-details-timeline-entry'))
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-kind')));
  expect(kinds[0]).toBe('created');
  for (const kind of ['branch', 'artifact', 'handover']) expect(kinds).toContain(kind);
});

test('session links open the Sessions tab on that session', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, panel());
  await openDetails(page);
  await page.locator(t('ade-handover-draft')).locator(t('ade-origin-session')).click();
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();
  await expect(page.locator(t('ade-session-tab-sp22'))).toHaveAttribute('data-selected', 'true');

  await page.locator(t('ade-panel-tab-details')).click();
  await row(page, 'Runbook').locator(t('ade-origin-session')).click();
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();

  await page.locator(t('ade-panel-tab-details')).click();
  await expect(row(page, 'Other docs').locator(t('ade-origin-session'))).toHaveCount(0);
});

test('an archived task opens Details read-only', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    ...panel(),
    { channel: IPC.adeTaskArchivedTask, response: archivedTaskFx() },
  ]);
  await page.locator(t('ade-archived-find')).click();
  await page.locator(`${t('ade-archived-find-item')}[data-task-id="H_1"]`).click();
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-details-tab'))).toBeVisible();
  await expect(page.locator(t('ade-details-paste'))).toHaveCount(0);
  await expect(page.locator(t('ade-details-link-edit'))).toHaveCount(0);
  await expect(page.locator(t('ade-details-link-remove'))).toHaveCount(0);
});

// Layout only, no IPC: the backend half has nothing to assert.
test('Main tab rows share one grid', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, panel());
  await openTask(page, 'T_bill');
  await expect(page.locator(t('ade-main-tab'))).toBeVisible();
  const rows = await page.locator(`${t('ade-main-tab')} [data-slot="ade-row"]`).evaluateAll((els) =>
    els.map((el) => {
      const x = (sel: string): number | null => {
        const n = el.querySelector(`:scope > ${sel}`);
        return n ? n.getBoundingClientRect().x : null;
      };
      const label = el.querySelector(':scope > [data-slot="ade-row-label"]');
      return {
        depth: Number((el as HTMLElement).dataset.depth),
        lead: x('[data-slot="ade-row-lead"]'),
        labelX: label ? label.getBoundingClientRect().x : null,
        labelW: label ? label.getBoundingClientRect().width : null,
        body: x('[data-slot="ade-row-body"]'),
        height: el.getBoundingClientRect().height,
        minHeight: Number.parseFloat(getComputedStyle(el).minHeight),
      };
    }),
  );
  expect(rows.length).toBeGreaterThanOrEqual(12);
  const uniq = (xs: (number | null)[]) => [...new Set(xs.map((v) => Math.round(v ?? -1)))];
  const leadAt = (d: number) => uniq(rows.filter((r) => r.depth === d).map((r) => r.lead));
  const depths = [...new Set(rows.map((r) => r.depth))].sort();
  for (const d of depths) expect(leadAt(d)).toHaveLength(1);
  for (const d of depths.slice(1)) {
    expect((leadAt(d)[0] ?? 0) - (leadAt(d - 1)[0] ?? 0)).toBe(14);
  }
  const labelled = rows.filter((r) => r.labelX !== null);
  expect(labelled.length).toBeGreaterThan(0);
  expect(uniq(labelled.map((r) => r.labelX))).toHaveLength(1);
  expect(uniq(labelled.map((r) => r.labelW))).toHaveLength(1);
  expect(uniq(labelled.map((r) => r.body))).toHaveLength(1);
  const plain = rows.filter((r) => r.depth === 0 && r.labelX === null);
  expect(plain.length).toBeGreaterThan(0);
  expect(uniq(plain.map((r) => r.body))).toEqual(uniq(labelled.map((r) => r.labelX)));
  for (const r of rows) expect(Math.round(r.height)).toBe(Math.round(r.minHeight));
});
