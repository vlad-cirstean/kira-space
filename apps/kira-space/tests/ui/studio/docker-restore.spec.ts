import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { openDocker, setupDocker } from './support/dockerFixture';
import { emitWailsEvent } from './support/mockRuntime';

type Json = Record<string, unknown>;

const NONE = { state: 'none', engineId: '', context: '', detectedAt: 0, bootAt: 0, items: [] };
const pendingFixture = () => contract<Json>('docker-restore', 'DockerService.RestoreState#pending');
const argsFixture = () => contract<Json>('docker-restore', 'args:DockerService.RestoreContainers');
const resultFixture = () => contract<Json>('docker-restore', 'DockerService.RestoreContainers');

const banner = (page: import('@playwright/test').Page) =>
  page.locator('[data-testid="docker-restore-banner"]');

async function openDialog(page: import('@playwright/test').Page): Promise<void> {
  await page.locator('[data-testid="docker-restore-review"]').click();
  await expect(page.locator('[data-testid="restore-dialog"]')).toBeVisible();
}

test('a pending restore shows the banner; Review lists projects, running and deleted containers', async ({
  relaunch,
}) => {
  const { page, state } = await setupDocker(relaunch);
  state.restore = pendingFixture();
  await openDocker(page);

  await expect(banner(page)).toContainText('5 containers were running before');
  await openDialog(page);

  const project = page.locator('[data-testid="restore-row"][data-id="project:shop-run"]');
  await expect(project).toHaveCount(1);
  await expect(project.locator('[data-testid="restore-member"]')).toHaveCount(2);
  await expect(
    page.locator('[data-testid="restore-row"][data-id="id-solo"] [data-slot="checkbox"]'),
  ).toHaveAttribute('data-state', 'checked');
  await expect(page.locator('[data-testid="restore-selection"]')).toHaveText('2 of 2 selected');

  await page.locator('[data-testid="restore-unavailable-toggle"]').click();
  await expect(
    page.locator('[data-testid="restore-row"][data-id="id-keeper"][data-status="back"]'),
  ).toContainText('Already running');
  await expect(
    page.locator('[data-testid="restore-row"][data-id="id-doomed"][data-status="gone"]'),
  ).toContainText('Not recreated');
});

test('Restore sends the contract args and clears the banner', async ({ relaunch }) => {
  const { page, docker, state } = await setupDocker(relaunch, {
    handlers: {
      RestoreContainers: () => {
        state.restore = NONE;
        return resultFixture();
      },
    },
  });
  state.restore = pendingFixture();
  await openDocker(page);
  await openDialog(page);

  await page.locator('[data-testid="restore-submit"]').click();

  await expect(banner(page)).toHaveCount(0);
  expect(docker.calls('RestoreContainers')[0]?.args).toEqual(argsFixture());
});

test('a failed container shows its error inline and keeps the dialog open', async ({
  relaunch,
}) => {
  const { page, state } = await setupDocker(relaunch, {
    handlers: {
      RestoreContainers: () => ({
        items: [
          { id: 'id-solo', name: 'kira-flow-solo', ok: false, error: 'port is already allocated' },
          { id: 'id-web', name: 'kira-flow-shop-a-web', ok: true, error: '' },
          { id: 'id-db', name: 'kira-flow-shop-b-db', ok: true, error: '' },
        ],
      }),
    },
  });
  state.restore = pendingFixture();
  await openDocker(page);
  await openDialog(page);

  await page.locator('[data-testid="restore-submit"]').click();

  await expect(page.locator('[data-testid="restore-row-error"]')).toContainText(
    'port is already allocated',
  );
  await expect(page.locator('[data-testid="restore-dialog"]')).toBeVisible();
});

test('Dismiss calls RestoreDismiss with the engine id and clears the banner', async ({
  relaunch,
}) => {
  const { page, docker, state } = await setupDocker(relaunch, {
    handlers: {
      RestoreDismiss: () => {
        state.restore = NONE;
      },
    },
  });
  state.restore = pendingFixture();
  await openDocker(page);
  await openDialog(page);

  await page.locator('[data-testid="restore-dismiss"]').click();

  await expect(banner(page)).toHaveCount(0);
  expect(docker.calls('RestoreDismiss')[0]?.args).toEqual({
    engineId: (pendingFixture() as { engineId: string }).engineId,
  });
});

test('a pushed pending state shows the banner without a reload', async ({ relaunch }) => {
  const { page } = await setupDocker(relaunch);
  await openDocker(page);
  await expect(banner(page)).toHaveCount(0);

  // The push can land before the panel subscribes under load; re-emit until it shows.
  await expect(async () => {
    await emitWailsEvent(page, 'kira:docker:restore', pendingFixture());
    await expect(banner(page)).toBeVisible({ timeout: 1000 });
  }).toPass();

  await emitWailsEvent(page, 'kira:docker:restore', NONE);
  await expect(banner(page)).toHaveCount(0);
});

test('unchecking the project row drops both members from the call', async ({ relaunch }) => {
  const { page, docker, state } = await setupDocker(relaunch);
  state.restore = pendingFixture();
  await openDocker(page);
  await openDialog(page);

  await page
    .locator('[data-testid="restore-row"][data-id="project:shop-run"] [data-slot="checkbox"]')
    .first()
    .click();
  await page.locator('[data-testid="restore-submit"]').click();

  await expect.poll(() => docker.calls('RestoreContainers').length).toBe(1);
  const sent = docker.calls('RestoreContainers')[0]?.args as { ids: string[] };
  expect(sent.ids).toEqual(['id-solo']);
});

test('a rejected Restore call shows its error in the dialog', async ({ relaunch }) => {
  const { page, state } = await setupDocker(relaunch, {
    handlers: {
      RestoreContainers: () => ({
        error: { code: 'E_CONFLICT', message: 'No restore is pending for this engine.' },
      }),
    },
  });
  state.restore = pendingFixture();
  await openDocker(page);
  await openDialog(page);

  await page.locator('[data-testid="restore-submit"]').click();

  await expect(page.locator('[data-testid="restore-call-error"]')).toContainText(
    'No restore is pending for this engine.',
  );
  await expect(page.locator('[data-testid="restore-dialog"]')).toBeVisible();
});

test('the banner text and Review button never overlap, wide or narrow', async ({ relaunch }) => {
  const { page, state } = await setupDocker(relaunch);
  state.restore = pendingFixture();
  await openDocker(page);

  const overlap = async (): Promise<boolean> => {
    const text = await banner(page).locator('[data-slot="alert-description"]').boundingBox();
    const btn = await page.locator('[data-testid="docker-restore-review"]').boundingBox();
    if (!text || !btn) throw new Error('banner not laid out');
    return (
      text.x < btn.x + btn.width &&
      btn.x < text.x + text.width &&
      text.y < btn.y + btn.height &&
      btn.y < text.y + text.height
    );
  };

  await expect(banner(page)).toBeVisible();
  expect(await overlap()).toBe(false);
  await page.setViewportSize({ width: 900, height: 700 });
  expect(await overlap()).toBe(false);
});
