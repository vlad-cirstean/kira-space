import { CHANNEL } from '@shared/protocol/events';
import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, studioTest, test } from './fixtures';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// The status-bar update-available item and the modal update dialog it opens — Kira Studio's
// own tests/ui/update-dialog.spec.ts, trimmed to this app's own first three cases (the error state
// is the same shared @workbench component, already covered there). Default boot answers "no
// update" (mockRuntime.ts's own WILDCARD_DEFAULTS[IPC.updateStatus]).

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
          targetVersion: '1.3.0',
          newestVersion: '1.3.0',
          installLogPath: '/tmp/kira-space-update.log',
        },
      },
    ],
  });
  await emitPrompts(window, [routed('update', '1.3.0')]);

  const dialog = window.locator('[data-testid="update-dialog"]');
  await expect(dialog).toBeVisible();
  await expect(window.locator('[data-testid="update-dialog-versions"]')).toHaveText(
    'Kira Space 1.3.0 is available. You have 1.2.0.',
  );
  await expect(window.locator('[data-testid="update-dialog-step-note"]')).toHaveCount(0);

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
  await expect(
    window.locator('[data-testid="tooltip-details-row"][data-label="Newest version"]'),
  ).toHaveCount(0);

  await item.click();
  await expect(dialog).toBeVisible();
});

test('stepped update: offers the next minor and names the newest', async ({ relaunch }) => {
  const status = (currentVersion: string, targetVersion: string) => ({
    control: [
      {
        channel: IPC.updateStatus,
        response: {
          updateAvailable: true,
          currentVersion,
          targetVersion,
          newestVersion: '0.5.1',
          installLogPath: '/tmp/kira-space-update.log',
        },
      },
    ],
  });

  const first = await relaunch(status('0.3.14', '0.4.0'));
  await emitPrompts(first.window, [routed('update', '0.4.0')]);
  await expect(first.window.locator('[data-testid="update-dialog-versions"]')).toHaveText(
    'Kira Space 0.4.0 is available. You have 0.3.14.',
  );
  await expect(first.window.locator('[data-testid="update-dialog-step-note"]')).toHaveText(
    '0.5.1 is the newest version. Kira Space updates one minor version at a time, so it offers the next update after this one.',
  );
  const item = first.window.locator('[data-testid="update-available"]');
  await expect(item).toHaveText('Update 0.4.0');
  await expect
    .poll(() => first.control.log().find((e) => e.channel === IPC.promptsRaise)?.args)
    .toEqual({ kind: 'update', ref: '0.4.0' });
  await first.window.click('[data-testid="update-dialog-later"]');
  await emitPrompts(first.window, []);
  await item.hover();
  await expect(
    first.window.locator('[data-testid="tooltip-details-row"][data-label="Newest version"]'),
  ).toContainText('0.5.1');

  // After the step the app relaunches and checks again: the next target raises its own popup.
  const second = await relaunch(status('0.4.0', '0.4.3'));
  await expect
    .poll(() => second.control.log().find((e) => e.channel === IPC.promptsRaise)?.args)
    .toEqual({ kind: 'update', ref: '0.4.3' });
});

test('Update click installs: control log shows the install call', async ({ relaunch }) => {
  const { window, control } = await relaunch({
    control: [
      {
        channel: IPC.updateStatus,
        response: {
          updateAvailable: true,
          currentVersion: '1.2.0',
          targetVersion: '1.3.0',
          newestVersion: '1.3.0',
          installLogPath: '/tmp/kira-space-update.log',
        },
      },
      { channel: IPC.updateInstall },
    ],
  });
  await emitPrompts(window, [routed('update', '1.3.0')]);

  await window.click('[data-testid="update-dialog-install"]');

  await expect
    .poll(() => control.log().some((entry) => entry.channel === IPC.updateInstall))
    .toBe(true);
});

test('menu check, up to date: shows the latest-version text, Close closes', async ({ kira }) => {
  const { window } = kira;

  await emitWailsEvent(window, CHANNEL.checkForUpdates, undefined);

  await expect(window.locator('[data-testid="update-dialog-up-to-date"]')).toHaveText(
    'Kira Space 0.0.0-dev is the latest version.',
  );
  await window.click('[data-testid="update-dialog-close"]');
  await expect(window.locator('[data-testid="update-dialog"]')).toHaveCount(0);
});

studioTest.describe('databases module', () => {
  studioTest('a failed install shows the error and re-enables Try again', async ({ relaunch }) => {
    const { window } = await relaunch({
      control: [
        {
          channel: IPC.updateStatus,
          response: {
            updateAvailable: true,
            currentVersion: '1.2.0',
            targetVersion: '1.3.0',
            newestVersion: '1.3.0',
            installLogPath: '/tmp/kira-space-update.log',
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
    await expect(install).toContainText('Retry');
  });

  studioTest('menu check failing shows the error, Retry checks again', async ({ relaunch }) => {
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

  studioTest(
    'menu check finding an update shows it without a second popup',
    async ({ relaunch }) => {
      const { window, control } = await relaunch({
        control: [
          {
            channel: IPC.updateCheck,
            response: {
              updateAvailable: true,
              currentVersion: '1.2.0',
              targetVersion: '1.3.0',
              newestVersion: '1.3.0',
              installLogPath: '/tmp/kira-space-update.log',
            },
          },
        ],
      });

      await emitWailsEvent(window, CHANNEL.checkForUpdates, undefined);

      await expect(window.locator('[data-testid="update-dialog-versions"]')).toHaveText(
        'Kira Space 1.3.0 is available. You have 1.2.0.',
      );
      await expect(window.locator('[data-testid="update-dialog-install"]')).toBeVisible();
      expect(control.log().some((e) => e.channel === IPC.promptsRaise)).toBe(false);
    },
  );

  studioTest(
    'a failed menu check does not stick to the dialog once an update is available',
    async ({ relaunch }) => {
      const { window } = await relaunch({
        control: [
          {
            channel: IPC.updateStatus,
            response: {
              updateAvailable: true,
              currentVersion: '1.2.0',
              targetVersion: '1.3.0',
              newestVersion: '1.3.0',
              installLogPath: '/tmp/kira-space-update.log',
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
    },
  );
});
