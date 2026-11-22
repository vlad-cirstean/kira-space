import { expect, test } from '../ui/fixtures';
import { openDetails, openPlan } from '../ui/support/adeV2';
import { contract } from '../ui/support/contract';
import { IPC } from '../ui/support/ipcChannels';
import { openCard } from '../ui/support/links';

// Link strip baselines: the Details strip with eight links and the open change request card.

interface ArtifactFx {
  id: string;
  source: string;
  foundIn?: { state: string }[];
}

const artifacts = () =>
  contract<{ artifacts: ArtifactFx[] }>(
    'ade-details-links',
    'AdeTaskService.Artifacts',
  ).artifacts.map((a, i) => ({ ...a, id: a.source === 'user' ? `a_${i}` : '' }));

test('the link strip with eight links', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArtifacts, response: { artifacts: artifacts(), gaps: [] } },
  ]);
  await openDetails(page);
  const strip = page.locator('[data-testid="ade-links"]');
  await expect(strip).toBeVisible();
  await expect(strip).toHaveScreenshot('ade-links.png');
});

test('the open change request card', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArtifacts, response: { artifacts: artifacts(), gaps: [] } },
  ]);
  await openDetails(page);
  const card = await openCard(page, 'api!9');
  await expect(card).toBeVisible();
  await expect(card).toHaveScreenshot('ade-links-card.png');
});

test('the related links line', async ({ relaunch }) => {
  const found = contract<{ artifacts: ArtifactFx[] }>(
    'ade-found-links',
    'AdeTaskService.Artifacts#promoted',
  ).artifacts.map((a, i) => ({
    ...a,
    id: a.source === 'user' || a.source === 'agent' ? `a_${i}` : '',
    // The handover entry only: a note entry prints a time that follows the machine's time zone.
    foundIn: a.foundIn?.filter((f) => f.state === 'handed'),
  }));
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArtifacts, response: { artifacts: found, gaps: [] } },
  ]);
  await openDetails(page);
  const strip = page.locator('[data-testid="ade-links"]');
  await expect(strip).toBeVisible();
  await strip.locator('[data-testid="ade-links-pill-found"]').last().hover();
  const tip = page.locator('[data-slot="tooltip-content"]');
  await expect(tip).toBeVisible();
  const [a, b] = [await strip.boundingBox(), await tip.boundingBox()];
  if (!a || !b) throw new Error('no box');
  // Whole pixels: a fractional clip rounds differently from run to run.
  const x = Math.floor(Math.min(a.x, b.x));
  const y = Math.floor(Math.min(a.y, b.y));
  const right = x + 560; // the pills and the tooltip, not the full-width strip
  const bottom = Math.ceil(Math.max(a.y + a.height, b.y + b.height));
  await expect(page).toHaveScreenshot('ade-links-related.png', {
    clip: { x, y, width: right - x, height: bottom - y },
  });
});
