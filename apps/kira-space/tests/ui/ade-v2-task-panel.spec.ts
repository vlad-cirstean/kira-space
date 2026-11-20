import { expect, test } from './fixtures';
import { adeFixture, archivedTaskFx, FIXED_NOW, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { line, openCard } from './support/links';
import type { ControlSnapshot } from './support/types';

// Task panel: Main, Details and Sessions tabs, link groups and session links. Link adding
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

test('the tabs are Main, Details and Sessions, Main first, and Files is an icon', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, panel());
  await openTask(page, 'T_alerts');
  await expect(page.locator(t('ade-main-tab'))).toBeVisible();
  await expect(page.locator(t('ade-panel-tab-main'))).toHaveText(/Main/);
  await expect(page.locator(t('ade-panel-tab-details'))).toBeVisible();
  await expect(page.locator(t('ade-panel-tab-sessions'))).toBeVisible();
  await expect(page.locator('[data-testid^="ade-panel-tab-"]')).toHaveCount(3);
  await expect(page.locator(t('ade-panel-tab-files'))).toHaveCount(0);
  await expect(page.locator(t('ade-panel-title'))).toBeVisible();
});

test('Files is a header icon: no tab, no right-click or More entry', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, panel());
  await openTask(page, 'T_cart');
  await expect(page.locator(t('ade-panel-files'))).toBeVisible();
  const files = page.locator('[data-testid^="menu-item-"][data-testid*="files"]');
  await page.locator(t('ade-panel-dot')).click({ button: 'right' });
  await expect(page.locator('[data-testid^="menu-item-"]').first()).toBeVisible();
  await expect(files).toHaveCount(0);
  await page.keyboard.press('Escape');
  await page.locator(t('ade-panel-more')).click();
  await expect(page.locator('[data-testid^="menu-item-"]').first()).toBeVisible();
  await expect(files).toHaveCount(0);
});

test('Main shows branches with their PR line and never handover text', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, panel());
  await openTask(page, 'T_bill');
  await expect(page.locator(t('ade-task-branch-pr'))).toHaveCount(2);
  await expect(page.locator(t('ade-task-branch-pr-base')).first()).toHaveText('→ main');
  const failed = page.locator(`${t('ade-task-branch-pr-state')}[data-checks="failure"]`);
  await expect(failed).toHaveCount(1);
  await expect(page.locator(`${t('ade-task-branch')}[data-branch-id="b_billdash"]`)).toBeVisible();
  await expect(page.locator(`${t('ade-task-branch-pr')}[data-branch-id="b_billdash"]`)).toHaveCount(
    0,
  );
  await expect(page.locator(t('ade-task-chain'))).toHaveCount(0);
});

test('Details groups links under Issues, Change requests and Misc', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, panel());
  await openDetails(page);
  await expect(
    line(page, 'issue').locator(t('ade-links-pill'), { hasText: 'PAY-130' }),
  ).toHaveCount(1);
  await expect(
    line(page, 'changeRequest').locator(t('ade-links-pill'), { hasText: 'Fix login' }),
  ).toHaveCount(1);
  await expect(line(page, 'misc').locator(t('ade-links-pill'), { hasText: 'Runbook' })).toHaveCount(
    1,
  );
  // Pills show a title or ref, never a bare number; the card title is the link.
  const labels = page.locator(t('ade-links-pill-label'));
  const count = await labels.count();
  for (let i = 0; i < count; i++) {
    expect((await labels.nth(i).textContent())?.trim()).not.toMatch(/^[#!]?\d+$/);
  }
  const card = await openCard(page, 'Runbook');
  expect(await card.locator(t('ade-links-card-title')).getAttribute('href')).toBeTruthy();
});

test('session links open the Sessions tab on that session', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, panel());
  await openDetails(page);
  await page.locator(t('ade-handover-draft')).locator(t('ade-origin-session')).click();
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();
  await expect(page.locator(t('ade-session-tab-sp22'))).toHaveAttribute('data-selected', 'true');

  await page.locator(t('ade-panel-tab-details')).click();
  const runbook = await openCard(page, 'Runbook');
  await runbook.locator(t('ade-origin-session')).click();
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();

  await page.locator(t('ade-panel-tab-details')).click();
  const other = await openCard(page, 'Other docs');
  await expect(other.locator(t('ade-origin-session'))).toHaveCount(0);
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
  await expect(page.locator(t('ade-links-add'))).toHaveCount(0);
  const card = await openCard(page, 'Runbook');
  await expect(card.locator(t('ade-links-card-edit'))).toHaveCount(0);
  await expect(card.locator(t('ade-links-card-remove'))).toHaveCount(0);
});

// Layout only, no IPC: the backend half has nothing to assert.
test('Main tab rows share one grid', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, panel());
  // Field responsive: label column beside the control once the panel is past 24rem, so widen.
  await page.setViewportSize({ width: 2200, height: 960 });
  await openTask(page, 'T_bill');
  await expect(page.locator(t('ade-main-tab'))).toBeVisible();
  const fields = page.locator(
    `${t('ade-main-fields')} [data-slot="field"][data-orientation="responsive"]`,
  );
  expect(await fields.count()).toBeGreaterThanOrEqual(3);
  const boxes = await fields.evaluateAll((els) =>
    els.map((el) => {
      const label = el.querySelector(':scope > [data-slot="field-label"]');
      const control = Array.from(el.children).find((c) => c !== label);
      const lb = label?.getBoundingClientRect();
      const cb = control?.getBoundingClientRect();
      return {
        labelX: lb?.x ?? null,
        controlX: cb?.x ?? null,
        labelY: lb?.y ?? 0,
        controlY: cb?.y ?? 99,
      };
    }),
  );
  const uniq = (xs: (number | null)[]) => [...new Set(xs.map((v) => Math.round(v ?? -1)))];
  expect(uniq(boxes.map((b) => b.labelX))).toHaveLength(1);
  expect(uniq(boxes.map((b) => b.controlX))).toHaveLength(1);
  for (const b of boxes) {
    expect(b.labelX ?? 99).toBeLessThan(b.controlX ?? 0);
    expect(Math.abs(b.labelY - b.controlY)).toBeLessThan(8);
  }
  const rows = await page.locator(`${t('ade-main-tab')} [data-slot="ade-row"]`).evaluateAll((els) =>
    els.map((el) => ({
      depth: Number((el as HTMLElement).dataset.depth),
      lead:
        el.querySelector(':scope > [data-slot="ade-row-lead"]')?.getBoundingClientRect().x ?? null,
      height: el.getBoundingClientRect().height,
      minHeight: Number.parseFloat(getComputedStyle(el).minHeight),
    })),
  );
  const leadAt = (d: number) => uniq(rows.filter((r) => r.depth === d).map((r) => r.lead));
  const depths = [...new Set(rows.map((r) => r.depth))].sort();
  for (const d of depths) expect(leadAt(d)).toHaveLength(1);
  for (const d of depths.slice(1)) {
    expect((leadAt(d)[0] ?? 0) - (leadAt(d - 1)[0] ?? 0)).toBe(14);
  }
  for (const r of rows) expect(Math.round(r.height)).toBe(Math.round(r.minHeight));
});
