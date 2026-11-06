import { expect, test } from './fixtures';
import { adeBoard, archivedTaskFx, FIXED_NOW, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Details list: notes, handovers and proof, plus task events behind the Timeline toggle, one list
// newest first. Contract ade-details-list; backend half: flows/adeflow TestDetailsList. The
// contract masks times, so the specs inject them.

const t = (id: string) => `[data-testid="${id}"]`;
const HOUR = 3_600_000;
const STORAGE_KEY = 'kira.ade.details.timeline';
const fx = <R>(call: string) => contract<R>('ade-details-list', call);

interface Row {
  state?: string;
  [key: string]: unknown;
}

// Oldest to newest: task, branches, stored artifact, note, draft, proof.
function snapshots(): ControlSnapshot[] {
  const board = adeBoard((b) => {
    const task = b.tasks.find((x) => x.id === 'T_alerts');
    if (task) task.createdAt = FIXED_NOW - 10 * HOUR;
    b.branches
      .filter((x) => x.taskId === 'T_alerts')
      .forEach((br, i) => {
        br.addedAt = FIXED_NOW - (9 - i) * HOUR;
      });
  });
  const handovers = fx<{ handovers: Row[] }>('AdeTaskService.Handovers#list').handovers.map(
    (h, i) => {
      const at = FIXED_NOW - (h.state === 'draft' ? 4 : 6) * HOUR;
      return { ...h, id: `h_${i}`, taskId: 'T_alerts', createdAt: at, updatedAt: at };
    },
  );
  const artifacts = fx<{ artifacts: Row[] }>('AdeTaskService.Artifacts#list').artifacts.map(
    (a, i) => ({
      ...a,
      id: `a_${i}`,
      sessionId: 'sp22',
      createdAt: FIXED_NOW - 8 * HOUR,
    }),
  );
  const proofs = fx<{ proofs: Row[] }>('AdeTaskService.TaskProofs#list').proofs.map((p) => ({
    ...p,
    proofId: 'P_login',
    sessionId: '',
    closedAt: FIXED_NOW - 2 * HOUR,
    linkedAt: FIXED_NOW - 2 * HOUR,
  }));
  return [
    { channel: IPC.adeTaskBoard, response: board },
    { channel: IPC.adeTaskHandovers, response: { handovers } },
    { channel: IPC.adeTaskArtifacts, response: { artifacts, gaps: [] } },
    { channel: IPC.adeTaskProofs, response: { proofs } },
    { channel: IPC.proofStep, response: contract('ade-proof-chain', 'ProofService.Step#diagram') },
  ];
}

const toggle = (page: import('@playwright/test').Page) =>
  page.locator(t('ade-details-timeline-toggle'));

async function openDetails(page: import('@playwright/test').Page): Promise<void> {
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-task-chain'))).toBeVisible();
}

const sequence = (page: import('@playwright/test').Page) =>
  page
    .locator(`${t('ade-task-chain')} [data-kind]`)
    .evaluateAll((els) =>
      els.map((e) => e.getAttribute('data-event') ?? e.getAttribute('data-kind')),
    );

test('the Timeline toggle is off by default and the list shows no events', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  await expect(toggle(page)).toHaveAttribute('aria-pressed', 'false');
  await expect(page.locator(t('ade-details-timeline-entry'))).toHaveCount(0);
  await expect.poll(() => sequence(page)).toEqual(['proof', 'handover', 'handover']);
});

test('contract: the toggle merges task events into one list newest first', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  const before = await page.locator('[data-kind="handover"]').count();
  await toggle(page).click();
  await expect(toggle(page)).toHaveAttribute('aria-pressed', 'true');
  await expect
    .poll(() => sequence(page))
    .toEqual(['proof', 'handover', 'handover', 'artifact', 'branch', 'branch', 'created']);
  await expect(page.locator('[data-kind="handover"]')).toHaveCount(before);
  await toggle(page).click();
  await expect(page.locator(t('ade-details-timeline-entry'))).toHaveCount(0);
});

test('the choice persists across a reload and a relaunch', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  await toggle(page).click();
  await page.reload();
  await page.locator(t('ade-plan')).waitFor();
  await openDetails(page);
  await expect(toggle(page)).toHaveAttribute('aria-pressed', 'true');

  const seeded = await openPlan(
    async (o) => relaunch({ ...o, storage: { [STORAGE_KEY]: 'true' } }),
    snapshots(),
  );
  await openDetails(seeded.window);
  await expect(seeded.window.locator(t('ade-details-timeline-entry')).first()).toBeVisible();
  await expect(toggle(seeded.window)).toHaveAttribute('aria-pressed', 'true');
});

test('an archived task toggles the events and stays read-only', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    ...snapshots(),
    { channel: IPC.adeTaskArchivedTask, response: archivedTaskFx() },
  ]);
  await page.locator(t('ade-archived-find')).click();
  await page.locator(`${t('ade-archived-find-item')}[data-task-id="H_1"]`).click();
  await page.locator(t('ade-panel-tab-details')).click();
  await toggle(page).click();
  await expect(
    page.locator(`${t('ade-details-timeline-entry')}[data-event="created"]`),
  ).toHaveCount(1);
  await expect(page.locator(t('ade-note-add'))).toHaveCount(0);
  await expect(page.locator(t('ade-proof-attach'))).toHaveCount(0);
});

test('an event row with a known session opens the Sessions tab on it', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  await toggle(page).click();
  await page
    .locator(`${t('ade-details-timeline-entry')}[data-event="artifact"]`)
    .locator(t('ade-origin-session'))
    .click();
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();
  await expect(page.locator(t('ade-session-tab-sp22'))).toHaveAttribute('data-selected', 'true');
});
