import type { KeptList } from '@shared/domain/kept';
import type { RevealResult } from '@shared/domain/variables';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openKeptPane } from './support/settings';

// Contract kept-vars-user. Backend half: dbmcpflow TestKeptVarsUserReveal. A value the user typed for an
// agent request lists as supplied by the user with the origin it goes to, shows dots, and Reveal asks
// the OS gate first; the value shows only in the row, hides again, and is never in the list.

const TYPED = 'typed-secret-value-42';

function liveList(): KeptList {
  const list = contract<KeptList>('kept-vars-user', 'KeptVarsService.List');
  return {
    ...list,
    entries: list.entries.map((e) => ({
      ...e,
      createdAt: new Date(Date.now() - 60_000).toISOString(),
      expiresAt: new Date(Date.now() + 59 * 60_000).toISOString(),
    })),
  };
}
const reveal = (key: string) =>
  contract<RevealResult>('kept-vars-user', `KeptVarsService.Reveal${key}`);

test('contract: a user value lists as supplied by you with its origin and dots', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.keptList, response: liveList() }],
  });
  await openKeptPane(page);
  const row = page.locator('[data-testid="settings-kept-entry"]');
  await row.locator('[data-testid="kept-name"]').hover();
  const tip = page.locator('[data-testid="kept-details"]');
  await expect(tip).toContainText('Typed by you');
  await expect(tip).toContainText('Goes only to https://api.example.test');
  await expect(row.locator('[data-testid="kept-value-secret"]')).toBeVisible();
  await expect(row.locator('[data-testid="kept-value-copy"]')).toHaveCount(0);
  await expect(page.locator('body')).not.toContainText(TYPED);
});

test('contract: Reveal shows the value in the row with Copy, Hide removes it', async ({
  relaunch,
}) => {
  const list = liveList();
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.keptList, response: list },
      { channel: IPC.keptReveal, response: reveal('') },
    ],
  });
  await openKeptPane(page);
  const row = page.locator('[data-testid="settings-kept-entry"]');
  const eye = row.locator('[data-testid="kept-value-reveal"]');
  await expect(eye).toHaveAttribute('aria-label', 'Show value');
  await expect(eye).toHaveAttribute('aria-pressed', 'false');
  await eye.click();
  await expect(row.locator('[data-testid="kept-value-revealed"]')).toHaveText(TYPED);
  await expect(eye).toHaveAttribute('aria-label', 'Hide value');
  await expect(eye).toHaveAttribute('aria-pressed', 'true');
  await expect(row.locator('[data-testid="kept-value-copy"]')).toBeVisible();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.keptReveal)?.args)
    .toEqual({ id: list.entries[0]?.id, confirmed: false });
  await row.locator('[data-testid="kept-value-reveal"]').click();
  await expect(row.locator('[data-testid="kept-value-revealed"]')).toHaveCount(0);
  await expect(page.locator('body')).not.toContainText(TYPED);
});

test('contract: a gate that needs confirmation asks, then reveals', async ({ relaunch }) => {
  const list = liveList();
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.keptList, response: list },
      {
        channel: IPC.keptReveal,
        args: { id: list.entries[0]?.id, confirmed: false },
        response: reveal('#confirmation'),
      },
      {
        channel: IPC.keptReveal,
        args: { id: list.entries[0]?.id, confirmed: true },
        response: reveal(''),
      },
    ],
  });
  await openKeptPane(page);
  const row = page.locator('[data-testid="settings-kept-entry"]');
  await row.locator('[data-testid="kept-value-reveal"]').click();
  await expect(row.locator('[data-testid="kept-value-revealed"]')).toHaveCount(0);
  await page.locator('[data-testid="confirm-dialog-confirm"]').click();
  await expect(row.locator('[data-testid="kept-value-revealed"]')).toHaveText(TYPED);
  await expect
    .poll(() =>
      control
        .log()
        .filter((e) => e.channel === IPC.keptReveal)
        .map((e) => e.args),
    )
    .toEqual([
      { id: list.entries[0]?.id, confirmed: false },
      { id: list.entries[0]?.id, confirmed: true },
    ]);
});
