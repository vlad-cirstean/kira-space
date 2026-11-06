import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeFixture, archivedTaskFx, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Unified notes: user notes and agent handovers in one chain. Contract ade-notes; backend half:
// flows/adeflow TestUnifiedNotes (notes_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface ChainFx {
  id: string;
  taskId: string;
  body: string;
  source: string;
  state: string;
}

function chain(taskId: string): ChainFx[] {
  const fx = contract<{ handovers: ChainFx[] }>('ade-notes', 'AdeTaskService.Handovers#chain');
  return fx.handovers.map((h, i) => ({ ...h, id: `h_${i}`, taskId }));
}

function chainSnapshot(taskId: string): ControlSnapshot {
  return { channel: IPC.adeTaskHandovers, response: { handovers: chain(taskId) } };
}

async function openDetails(page: Page, taskId = 'T_alerts'): Promise<void> {
  await openTask(page, taskId);
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-task-chain'))).toBeVisible();
}

test('contract: the chain lists notes and the agent handover newest first, marked by author', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, [chainSnapshot('T_alerts')]);
  await openDetails(page);
  const entries = page.locator(`${t('ade-task-chain')} [data-author]`);
  await expect(entries).toHaveCount(4);
  expect(await entries.evaluateAll((els) => els.map((e) => e.getAttribute('data-author')))).toEqual(
    ['user', 'agent', 'user', 'user'],
  );
  await expect(entries.first()).toContainText('Ship on Friday.');
  await expect(entries.nth(1)).toContainText('Form wired.');
  await expect(entries.nth(2).locator('strong')).toHaveText('new');
  await expect(entries.nth(2).locator(t('ade-note-stage'))).toContainText('During Design');
  await expect(page.locator(t('ade-note-composer'))).toHaveCount(0);
  await expect(page.locator(t('ade-notes-toolbar-btn'))).toHaveCount(0);
});

test('Add note reveals the composer; Mod+Enter sends AddNote with Markdown and hides it', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    chainSnapshot('T_alerts'),
    { channel: IPC.adeTaskAddNote, response: chain('T_alerts')[0] },
  ]);
  await openDetails(page);
  await page.locator(t('ade-note-add')).click();
  await expect(page.locator(t('ade-note-composer'))).toBeVisible();
  await expect(page.locator(t('ade-notes-toolbar-btn')).first()).toBeVisible();
  await page.locator(`${t('ade-note-composer')} .ProseMirror`).click();
  await page.keyboard.type('Check **digest**');
  await page.keyboard.press('Meta+Enter');
  await expect.poll(() => calls(control, IPC.adeTaskAddNote)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddNote)[0]?.args).toEqual({
    taskId: 'T_alerts',
    body: 'Check **digest**',
  });
  await expect(page.locator(t('ade-note-composer'))).toHaveCount(0);
});

test('the composer draft survives a switch to the Sessions tab and back', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [chainSnapshot('T_alerts')]);
  await openDetails(page);
  await page.locator(t('ade-note-add')).click();
  await page.locator(`${t('ade-note-composer')} .ProseMirror`).click();
  await page.keyboard.type('half a thought');
  await page.locator(t('ade-panel-tab-sessions')).click();
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(`${t('ade-note-composer')} .ProseMirror`)).toHaveText('half a thought');
});

test('editing the handed agent row uses the same editor and sends UpdateHandover', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [chainSnapshot('T_alerts')]);
  await openDetails(page);
  const row = page.locator(`${t('ade-task-chain')} ${t('ade-handover-row')}`);
  await row.locator(t('ade-handover-edit')).click();
  await expect(page.locator(t('ade-notes-toolbar-btn')).first()).toBeVisible();
  await row.locator('.ProseMirror').fill('Corrected handover.');
  await row.locator(t('ade-handover-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskUpdateHandover)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskUpdateHandover)[0]?.args).toEqual({
    id: 'h_1',
    body: 'Corrected handover.',
  });
});

test('an archived task shows the chain read-only: no Add, no editor controls', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArchivedTask, response: archivedTaskFx('H_1') },
    chainSnapshot('H_1'),
  ]);
  await page.locator(t('ade-archived-find')).click();
  await page.locator(`${t('ade-archived-find-item')}[data-task-id="H_1"]`).click();
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(`${t('ade-task-chain')} [data-author]`)).toHaveCount(4);
  await expect(page.locator(t('ade-note-add'))).toHaveCount(0);
  await expect(page.locator(t('ade-handover-edit'))).toHaveCount(0);
  await expect(page.locator(t('ade-notes-toolbar-btn'))).toHaveCount(0);
  await expect(page.locator(`${t('ade-task-chain')} .ProseMirror`).first()).toHaveAttribute(
    'contenteditable',
    'false',
  );
});

test('the Add popover notes field is the rich editor and CreateTask carries its Markdown', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCreateTask, response: { ...adeFixture<object>('task'), id: 'T_deps' } },
  ]);
  await page.locator(t('ade-add')).click();
  await page.locator(t('ade-nw-title')).fill('Export CSV');
  await expect(
    page.locator(`${t('ade-nw-notes')} ${t('ade-notes-toolbar-btn')}`).first(),
  ).toBeVisible();
  await page.locator(`${t('ade-nw-notes')} .ProseMirror`).click();
  await page.keyboard.type('Mind the **header** row');
  await page.locator(t('ade-nw-add')).click();
  await expect.poll(() => calls(control, IPC.adeTaskCreateTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskCreateTask)[0]?.args).toMatchObject({
    title: 'Export CSV',
    notes: 'Mind the **header** row',
  });
});
