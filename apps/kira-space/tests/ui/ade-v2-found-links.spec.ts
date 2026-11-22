import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { openDetails, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { line, openCard, pill } from './support/links';

// Links found in notes and handovers: the Related line, provenance, promote and dismiss. UI half of
// contract ade-found-links; backend half: flows/adeflow TestFoundLinks (foundlinks_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface ArtifactFx {
  id: string;
  source: string;
  url: string;
  title: string;
}

/** Stored rows get distinct ids: the contract masks them. Found links stay detected (no id). */
function listed(key: string): ArtifactFx[] {
  return contract<{ artifacts: ArtifactFx[] }>('ade-found-links', key).artifacts.map((a, i) => ({
    ...a,
    id: a.source === 'user' || a.source === 'agent' ? `a_${i}` : '',
  }));
}

const result = (key: string) => ({ artifacts: listed(key), gaps: [] });

const initial = () => ({
  channel: IPC.adeTaskArtifacts,
  response: result('AdeTaskService.Artifacts#initial'),
});

const related = (page: Page) => line(page, 'related').locator(t('ade-links-pill'));

test('contract: found links draw in the Related line with where they were found', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, [initial()]);
  await openDetails(page);
  await expect(related(page)).toHaveCount(4);
  await expect(page.locator(`${t('ade-links-pill')}[data-origin="found"]`)).toHaveCount(4);
  for (const title of ['Design', 'api!21', 'ex.test/a', 'ex.test/b']) {
    await expect(pill(line(page, 'related'), title)).toHaveCount(1);
  }
  // Main links stay in their own lines: the user's Spec doc is Misc, no found pill is there.
  await expect(line(page, 'misc').locator(t('ade-links-pill'))).toHaveCount(1);
  await expect(pill(line(page, 'misc'), 'Spec')).toHaveAttribute('data-origin', 'stored');
  await expect(line(page, 'changeRequest')).toHaveCount(0);

  await pill(page, 'ex.test/a').locator(t('ade-links-pill-found')).hover();
  await expect(page.getByRole('tooltip')).toContainText('Found in: Handover Work to Ship, Note of');
});

test('contract: the card lists the sources; adding to links sends AddLink and main takes over', async ({
  relaunch,
}) => {
  const sent = contract<{ taskId: string; text: string }>(
    'ade-found-links',
    'args:AdeTaskService.AddLink',
  );
  const { window: page, control } = await openPlan(relaunch, [initial()]);
  await openDetails(page);
  const card = await openCard(page, 'ex.test/a');
  await expect(card.locator(t('ade-links-card-found')).locator('li')).toHaveCount(2);
  await expect(card.locator(t('ade-links-card-found'))).toContainText('Handover Work to Ship');
  await expect(card.locator(t('ade-links-card-edit'))).toHaveCount(0);
  await expect(card.locator(t('ade-links-card-remove'))).toHaveCount(0);
  await expect(card.locator(t('ade-links-card-kind'))).toHaveCount(0);
  await page.keyboard.press('Escape');

  const design = await openCard(page, 'Design');
  control.respond(IPC.adeTaskArtifacts, result('AdeTaskService.Artifacts#promoted'));
  await design.locator(t('ade-links-card-promote')).click();
  await expect.poll(() => calls(control, IPC.adeTaskAddLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddLink)[0]?.args).toEqual({ ...sent, taskId: 'T_alerts' });

  await expect(related(page)).toHaveCount(2);
  await expect(pill(line(page, 'related'), 'Design')).toHaveCount(0);
  // Design (the user's) and B (the agent's) are Misc now.
  await expect(pill(line(page, 'misc'), 'Design')).toHaveAttribute('data-origin', 'stored');
  await expect(pill(line(page, 'misc'), 'B')).toHaveAttribute('data-origin', 'stored');
  const b = await openCard(page, 'B');
  await expect(b.locator(t('ade-links-card-source'))).toHaveText('Agent');
});

test('contract: dismiss sends DismissFoundLink and the link leaves Related', async ({
  relaunch,
}) => {
  const sent = Object.keys(
    contract<Record<string, unknown>>('ade-found-links', 'args:AdeTaskService.DismissFoundLink'),
  );
  const url = contract<{ url: string }>(
    'ade-found-links',
    'args:AdeTaskService.DismissFoundLink',
  ).url;
  const { window: page, control } = await openPlan(relaunch, [initial()]);
  await openDetails(page);
  const card = await openCard(page, 'api!21');
  control.respond(IPC.adeTaskArtifacts, result('AdeTaskService.Artifacts#dismissed'));
  await card.locator(t('ade-links-card-dismiss')).click();
  await expect.poll(() => calls(control, IPC.adeTaskDismissFoundLink)).toHaveLength(1);
  const args = calls(control, IPC.adeTaskDismissFoundLink)[0]?.args as Record<string, unknown>;
  expect(args).toEqual({ taskId: 'T_alerts', url });
  expect(Object.keys(args).sort()).toEqual([...sent].sort());
  await expect(pill(page, 'api!21')).toHaveCount(0);
});

test('an archived task shows found links with no promote or dismiss', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [initial()]);
  await page.locator(t('ade-archived-find')).click();
  await page.locator(`${t('ade-archived-find-item')}[data-task-id="H_1"]`).click();
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(related(page)).toHaveCount(4);
  const card = await openCard(page, 'Design');
  await expect(card.locator(t('ade-links-card-found'))).toBeVisible();
  await expect(card.locator(t('ade-links-card-promote'))).toHaveCount(0);
  await expect(card.locator(t('ade-links-card-dismiss'))).toHaveCount(0);
});
