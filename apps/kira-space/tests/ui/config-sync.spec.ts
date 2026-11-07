import type { Page } from '@playwright/test';
import { configSyncReviewTest, expect, test } from './fixtures';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// UI half of configsyncflow TestConfigSync: the Shared config pane, the toast and the review
// window show what the backend scenario produced. Contract config-sync.

const t = (id: string) => `[data-testid="${id}"]`;
const SHA = 'a1b2c3d4e5f60718293a4b5c6d7e8f9012345678';

type Status = { showToast: boolean; changes: number } & Record<string, unknown>;
type Preview = { rows: { id: string; kind: string }[] } & Record<string, unknown>;

const status = {
  ...contract<Status>('config-sync', 'ConfigSyncService.Status'),
  url: 'https://example.com/team/kira-config.git',
  fetched: SHA,
  fetchedAt: '2026-01-01T10:00:00Z',
  appliedAt: '',
};
const preview = {
  ...contract<Preview>('config-sync', 'ConfigSyncService.Preview'),
  token: 'tok-1',
  commit: SHA,
};
const applied = contract('config-sync', 'ConfigSyncService.Apply');

const withStatus = (over: Partial<Status> = {}): ControlSnapshot[] => [
  { channel: IPC.configSyncStatus, response: { ...status, ...over } },
];

async function openPane(
  relaunch: (o: { control: ControlSnapshot[] }) => Promise<{
    window: Page;
    control: { log(): { channel: string; args?: unknown }[] };
  }>,
  control: ControlSnapshot[],
) {
  const { window: page, control: handle } = await relaunch({ control });
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await page.locator(t('settings-dialog')).waitFor();
  await page.locator(t('settings-section-Shared config')).click();
  return { page, handle };
}

test('contract: the pane shows the repository, the changes and each module', async ({
  relaunch,
}) => {
  const { page, handle } = await openPane(relaunch, withStatus({ showToast: false }));
  await expect(page.locator(t('config-sync-repo-url'))).toHaveText(String(status.url));
  await expect(page.locator(t('config-sync-changes-status'))).toHaveText('2 changes available');
  await expect(page.locator(t('config-sync-module-connections'))).toBeVisible();
  await expect(page.locator(t('config-sync-mode-connections'))).toHaveValue('shared');
  await page.locator(t('config-sync-review')).click();
  await expect.poll(() => calls(handle, IPC.configSyncOpenReview)).toHaveLength(1);
});

test('contract: Pull applies a pull without conflicts in place', async ({ relaunch }) => {
  const { page, handle } = await openPane(relaunch, [
    ...withStatus({ showToast: false }),
    { channel: IPC.configSyncPreview, response: preview },
    { channel: IPC.configSyncApply, response: applied },
  ]);
  await page.locator(t('config-sync-pull')).click();
  await expect.poll(() => calls(handle, IPC.configSyncApply)).toHaveLength(1);
  expect(calls(handle, IPC.configSyncApply)[0]?.args).toEqual({ token: 'tok-1', choices: {} });
  expect(calls(handle, IPC.configSyncOpenReview)).toHaveLength(0);
});

test('contract: a pull with a conflict opens the review window instead', async ({ relaunch }) => {
  const conflicted = {
    ...preview,
    rows: preview.rows.map((r, i) => (i === 0 ? { ...r, kind: 'conflict' } : r)),
  };
  const { page, handle } = await openPane(relaunch, [
    ...withStatus({ showToast: false }),
    { channel: IPC.configSyncPreview, response: conflicted },
  ]);
  await page.locator(t('config-sync-pull')).click();
  await expect.poll(() => calls(handle, IPC.configSyncOpenReview)).toHaveLength(1);
  await expect(page.locator(t('config-sync-conflict-hint'))).toBeVisible();
  expect(calls(handle, IPC.configSyncApply)).toHaveLength(0);
});

test('contract: the toast offers Review and Dismiss for the fetched commit', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: withStatus() });
  const toast = page.locator(t('config-sync-toast'));
  await expect(toast).toBeVisible();
  await expect(page.locator(t('config-sync-toast-count'))).toHaveText('2 changes');
  await page.locator(t('config-sync-toast-review')).click();
  await expect.poll(() => calls(control, IPC.configSyncOpenReview)).toHaveLength(1);
  await page.locator(t('config-sync-toast-dismiss')).click();
  await expect.poll(() => calls(control, IPC.configSyncDismiss)).toHaveLength(1);
  expect(calls(control, IPC.configSyncDismiss)[0]?.args).toEqual({ sha: SHA });
});

configSyncReviewTest(
  'contract: the review window lists the units and pulls with the token',
  async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      control: [
        ...withStatus({ showToast: false }),
        { channel: IPC.configSyncIsReviewWindow, response: true },
        { channel: IPC.configSyncPreview, response: preview },
        { channel: IPC.configSyncApply, response: applied },
      ],
    });
    await expect(page.locator(t('config-sync-window'))).toBeVisible();
    await expect(page.locator(t('config-sync-toast'))).toHaveCount(0);
    await expect(page.locator(t('config-sync-row-connections/team.yaml'))).toBeVisible();
    await expect(page.locator(t('config-sync-row-kind-connections/team.yaml'))).toHaveText('Add');
    await page.locator(t('config-sync-select-connections/team.yaml')).click();
    await expect(page.locator(t('config-sync-diff'))).toBeVisible();
    await page.locator(t('config-sync-pull')).click();
    await expect.poll(() => calls(control, IPC.configSyncApply)).toHaveLength(1);
    expect(calls(control, IPC.configSyncApply)[0]?.args).toEqual({ token: 'tok-1', choices: {} });
    await expect.poll(() => calls(control, IPC.configSyncCloseReview)).toHaveLength(1);
  },
);
