import { expect, test } from '../ui/fixtures';
import { openDetails, openPlan } from '../ui/support/adeV2';
import { contract } from '../ui/support/contract';
import { IPC } from '../ui/support/ipcChannels';
import { openCard } from '../ui/support/links';

// Link strip baselines: the Details strip with eight links and the open change request card.

interface ArtifactFx {
  id: string;
  source: string;
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
