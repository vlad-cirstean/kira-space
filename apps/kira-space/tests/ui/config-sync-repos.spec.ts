import { configSyncReviewTest, expect, test } from './fixtures';
import { openAdeRepos } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// UI half of configsyncflow TestConfigSyncRepos: the repos module in the pane and review window,
// and the repo settings pane's Copy for shared config. Contract config-sync-repos.

const t = (id: string) => `[data-testid="${id}"]`;
const SHA = 'a1b2c3d4e5f60718293a4b5c6d7e8f9012345678';

interface Row {
  id: string;
  kind: string;
  problem: string;
  runsCommands: boolean;
  repo?: string;
  origin?: string;
  target?: string;
}
type Preview = { rows: Row[] } & Record<string, unknown>;

const status = {
  ...contract<Record<string, unknown>>('config-sync-repos', 'ConfigSyncService.Status#repos'),
  url: 'https://example.com/team/kira-config.git',
  fetched: SHA,
  fetchedAt: '2026-01-01T10:00:00Z',
  appliedAt: '',
  showToast: false,
};
const preview = {
  ...contract<Preview>('config-sync-repos', 'ConfigSyncService.Preview#repos'),
  token: 'tok-r',
  commit: SHA,
};
const applied = contract('config-sync-repos', 'ConfigSyncService.Apply#repos');
const sharedText = contract<string>('config-sync-repos', 'ConfigSyncService.RepoSharedText');

const row = (id: string) => {
  const found = preview.rows.find((r) => r.id === id);
  if (!found) throw new Error(`contract row ${id} missing`);
  return found;
};
const byKind = (kind: string) => {
  const found = preview.rows.find((r) => r.kind === kind);
  if (!found) throw new Error(`contract has no ${kind} row`);
  return found;
};

test('contract: the pane offers Keep local and Combined for repository settings', async ({
  relaunch,
}) => {
  const control: ControlSnapshot[] = [{ channel: IPC.configSyncStatus, response: status }];
  const { window: page } = await relaunch({ control });
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await page.locator(t('settings-dialog')).waitFor();
  await page.locator(t('settings-section-Shared config')).click();
  await expect(page.locator(t('config-sync-module-repos'))).toBeVisible();
  await expect(page.locator(t('config-sync-mode-repos'))).toHaveValue('combined');
  const options = await page
    .locator(t('config-sync-mode-repos'))
    .locator('option')
    .allTextContents();
  expect(options).toEqual(['Keep local', 'Combined']);
});

configSyncReviewTest(
  'contract: the review window groups repository rows, flags commands and pulls with the token',
  async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      control: [
        { channel: IPC.configSyncStatus, response: status },
        { channel: IPC.configSyncIsReviewWindow, response: true },
        { channel: IPC.configSyncPreview, response: preview },
        { channel: IPC.configSyncApply, response: applied },
      ],
    });
    await expect(page.locator(t('config-sync-window'))).toBeVisible();

    const add = preview.rows.find((r) => r.kind === 'add' && r.runsCommands);
    if (!add) throw new Error('contract has no add row that runs commands');
    const addRow = page.locator(t(`config-sync-row-${add.id}`));
    await expect(addRow).toBeVisible();
    await expect(addRow.locator(t('config-sync-row-runs'))).toBeVisible();
    await expect(page.locator(t(`config-sync-row-repo-${add.id}`))).toHaveText(String(add.repo));

    const blocked = byKind('blocked');
    await expect(page.locator(t(`config-sync-row-problem-${blocked.id}`))).toHaveText(
      blocked.problem,
    );

    const unmatched = byKind('unmatched');
    await expect(page.locator(t(`config-sync-row-kind-${unmatched.id}`))).toHaveText(
      'No local repository',
    );
    await expect(page.locator(t(`config-sync-row-repo-${unmatched.id}`))).toHaveText(
      String(unmatched.origin),
    );
    await expect(page.locator(t(`config-sync-deactivate-${unmatched.id}`))).toHaveCount(0);

    const targeted = row(add.id);
    await page.locator(t(`config-sync-select-${targeted.id}`)).click();
    await expect(page.locator(t('config-sync-diff'))).toBeVisible();

    await page.locator(t('config-sync-pull')).click();
    await expect.poll(() => calls(control, IPC.configSyncApply)).toHaveLength(1);
    expect(calls(control, IPC.configSyncApply)[0]?.args).toEqual({ token: 'tok-r', choices: {} });
  },
);

test('contract: Copy for shared config writes the backend text to the clipboard', async ({
  relaunch,
}) => {
  const { window: page, control } = await openAdeRepos(relaunch, [
    { channel: IPC.configSyncRepoSharedText, response: sharedText },
  ]);
  await page.locator(t('repo-file')).waitFor();
  await page.evaluate(() => {
    const w = window as unknown as { __clipboard: string[] };
    w.__clipboard = [];
    navigator.clipboard.writeText = async (text: string) => {
      w.__clipboard.push(text);
    };
  });
  await page.locator(t('repo-file-copy-shared')).click();
  await expect.poll(() => calls(control, IPC.configSyncRepoSharedText)).toHaveLength(1);
  await expect
    .poll(() =>
      page.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard[0]),
    )
    .toBe(sharedText);
});
