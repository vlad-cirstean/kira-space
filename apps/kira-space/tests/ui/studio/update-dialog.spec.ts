import { CHANNEL } from '@shared/protocol/events';
import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, test } from './fixtures';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// The status-bar update-available item and the modal update dialog it opens, replacing
// the update-banner (release-page-link) behavior. Default boot answers "no update"
// (mockRuntime.ts's own WILDCARD_DEFAULTS[IPC.updateStatus]), so case 1 proves the
// hidden-by-default shape every other existing spec/snapshot silently relies on; the rest seed an
// available update explicitly.

test('no update available: no status-bar item, no dialog', async ({ kira }) => {
  const { window } = kira;

  await expect(window.locator('[data-testid="update-available"]')).toHaveCount(0);
  await expect(window.locator('[data-testid="update-dialog"]')).toHaveCount(0);
  await expect(window.locator('[data-testid="status-bar"]')).toBeVisible();
});

test('update available: the routed dialog shows, Later dismisses it, the item reopens it', async ({
  relaunch,
}) => {
  const { window, control } = await relaunch({
    control: [
      {
        channel: IPC.updateStatus,
        response: {
          updateAvailable: true,
          currentVersion: '1.2.0',
          latestVersion: '1.3.0',
          installLogPath: '/tmp/kira-studio-update.log',
        },
      },
    ],
  });
  await emitPrompts(window, [routed('update', '1.3.0')]);

  const dialog = window.locator('[data-testid="update-dialog"]');
  await expect(dialog).toBeVisible();
  await expect(window.locator('[data-testid="update-dialog-versions"]')).toHaveText(
    'Kira Studio 1.3.0 is available. You have 1.2.0.',
  );

  const item = window.locator('[data-testid="update-available"]');
  await expect(item).toBeVisible();
  await expect(item).toHaveText('Update 1.3.0');

  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.promptsRaise)?.args)
    .toEqual({ kind: 'update', ref: '1.3.0' });
  await window.click('[data-testid="update-dialog-later"]');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.promptsDismiss)?.args)
    .toEqual({ id: 'update:1.3.0' });
  await emitPrompts(window, []);
  await expect(dialog).toHaveCount(0);
  await expect(item).toBeVisible();
  await item.hover();
  await expect(window.locator('[data-testid="tooltip-details-title"]')).toHaveText(
    'Update available',
  );
  await expect(
    window.locator('[data-testid="tooltip-details-row"][data-label="New version"]'),
  ).toContainText('1.3.0');
  await expect(
    window.locator('[data-testid="tooltip-details-row"][data-label="Installed"]'),
  ).toContainText('1.2.0');

  await item.click();
  await expect(dialog).toBeVisible();
});

test('Update click installs: control log shows the install call', async ({ relaunch }) => {
  const { window, control } = await relaunch({
    control: [
      {
        channel: IPC.updateStatus,
        response: {
          updateAvailable: true,
          currentVersion: '1.2.0',
          latestVersion: '1.3.0',
          installLogPath: '/tmp/kira-studio-update.log',
        },
      },
      { channel: IPC.updateInstall, hold: true },
    ],
  });
  await emitPrompts(window, [routed('update', '1.3.0')]);

  await window.click('[data-testid="update-dialog-install"]');

  await expect
    .poll(() => control.log().some((entry) => entry.channel === IPC.updateInstall))
    .toBe(true);
});

test('a failed install shows the error and re-enables Try again', async ({ relaunch }) => {
  const { window } = await relaunch({
    control: [
      {
        channel: IPC.updateStatus,
        response: {
          updateAvailable: true,
          currentVersion: '1.2.0',
          latestVersion: '1.3.0',
          installLogPath: '/tmp/kira-studio-update.log',
        },
      },
      {
        channel: IPC.updateInstall,
        error: { code: 'E_INTERNAL', message: 'checksum mismatch' },
      },
    ],
  });
  await emitPrompts(window, [routed('update', '1.3.0')]);

  await window.click('[data-testid="update-dialog-install"]');

  const error = window.locator('[data-testid="update-dialog-error"]');
  await expect(error).toBeVisible();
  await expect(error).toContainText('checksum mismatch');

  const install = window.locator('[data-testid="update-dialog-install"]');
  await expect(install).toBeEnabled();
  await expect(install).toHaveText('Retry');
});

test('menu check, up to date: shows the latest-version text, Close closes', async ({ kira }) => {
  const { window } = kira;

  await emitWailsEvent(window, CHANNEL.checkForUpdates, undefined);

  await expect(window.locator('[data-testid="update-dialog-up-to-date"]')).toHaveText(
    'Kira Studio 0.0.0-dev is the latest version.',
  );
  await window.click('[data-testid="update-dialog-close"]');
  await expect(window.locator('[data-testid="update-dialog"]')).toHaveCount(0);
});

test('menu check failing shows the error, Retry checks again', async ({ relaunch }) => {
  const { window, control } = await relaunch({
    control: [
      {
        channel: IPC.updateCheck,
        error: { code: 'E_CONFLICT', message: 'Updates are available only in release builds.' },
      },
    ],
  });

  await emitWailsEvent(window, CHANNEL.checkForUpdates, undefined);

  const error = window.locator('[data-testid="update-dialog-check-error"]');
  await expect(error).toContainText('Updates are available only in release builds.');
  await window.click('[data-testid="update-dialog-retry"]');
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.updateCheck).length)
    .toBe(2);
});

test('menu check finding an update shows it without a second popup', async ({ relaunch }) => {
  const { window, control } = await relaunch({
    control: [
      {
        channel: IPC.updateCheck,
        response: {
          updateAvailable: true,
          currentVersion: '1.2.0',
          latestVersion: '1.3.0',
          installLogPath: '/tmp/kira-studio-update.log',
        },
      },
    ],
  });

  await emitWailsEvent(window, CHANNEL.checkForUpdates, undefined);

  await expect(window.locator('[data-testid="update-dialog-versions"]')).toHaveText(
    'Kira Studio 1.3.0 is available. You have 1.2.0.',
  );
  await expect(window.locator('[data-testid="update-dialog-install"]')).toBeVisible();
  expect(control.log().some((e) => e.channel === IPC.promptsRaise)).toBe(false);
});

test('a failed menu check does not stick to the dialog once an update is available', async ({
  relaunch,
}) => {
  const { window } = await relaunch({
    control: [
      {
        channel: IPC.updateStatus,
        response: {
          updateAvailable: true,
          currentVersion: '1.2.0',
          latestVersion: '1.3.0',
          installLogPath: '/tmp/kira-studio-update.log',
        },
      },
      { channel: IPC.updateCheck, error: { code: 'E_CONFLICT', message: 'Offline.' } },
    ],
  });
  await emitWailsEvent(window, CHANNEL.checkForUpdates, undefined);
  await expect(window.locator('[data-testid="update-dialog-check-error"]')).toBeVisible();
  await window.locator('[data-testid="update-dialog-close"]').click();
  await expect(window.locator('[data-testid="update-dialog"]')).toHaveCount(0);

  await window.click('[data-testid="update-available"]');
  await expect(window.locator('[data-testid="update-dialog-install"]')).toBeVisible();
  await expect(window.locator('[data-testid="update-dialog-check-error"]')).toHaveCount(0);
});
