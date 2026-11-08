import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// UI half of appflow's TestAboutGitHubLink: the About pane in Settings. Contract settings-about.

const about = () =>
  contract<{ appVersion: string; repoUrl: string }>('settings-about', 'AppService.Info#about');
const opened = () => contract<{ url: string }>('settings-about', 'args:LinkService.OpenExternal');

const id = (s: string) => `[data-testid="${s}"]`;

const snapshot = () => [
  {
    channel: IPC.appInfo,
    response: { ...about(), go: 'go1.0', wails: 'v3.0.0', kiraHome: '/home/u/.kira-space' },
  },
  { channel: IPC.linkOpenExternal, response: null },
];

async function openPane(page: Page): Promise<void> {
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await page.locator(id('settings-section-About')).click();
}

test('contract: About shows name, version and repo link', async ({ relaunch }) => {
  const fx = about();
  const { window: page } = await relaunch({ control: snapshot() });
  await openPane(page);
  const pane = page.locator(id('settings-about'));
  await expect(pane.getByText('Kira Space', { exact: true })).toBeVisible();
  await expect(pane.locator(id('settings-about-version'))).toHaveText(fx.appVersion);
  await expect(pane.locator(id('settings-about-github'))).toContainText(fx.repoUrl);
  await expect(page.locator('[data-testid^="settings-section-"]').last()).toHaveAttribute(
    'data-testid',
    'settings-section-About',
  );
});

test('contract: GitHub link opens through LinkService.OpenExternal', async ({ relaunch }) => {
  const { url } = opened();
  const { window: page, control } = await relaunch({ control: snapshot() });
  await openPane(page);
  const link = page.locator(id('settings-about-github'));
  await link.click();
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual({ url });
  await link.focus();
  await page.keyboard.press('Enter');
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(2);
  expect(calls(control, IPC.linkOpenExternal)[1]?.args).toEqual({ url });
});
