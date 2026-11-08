import { expect, studioTest } from './fixtures';
import { PG_CONN } from './studio/support/copyScenarios';
import { openSettings } from './studio/support/settings';
import { connectionRow } from './studio/support/tree';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// P579: dialogs sit on --kira-bg-dialog (#141416) over a 0.6 scrim; menus keep --kira-bg-elevated.
// Renderer only: no IPC, no backend half.
const DIALOG = 'rgb(20, 20, 22)';
const ELEVATED = 'rgb(33, 33, 37)';

studioTest(
  'the settings dialog paints the dark surface over the 0.6 scrim',
  async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.connectionsList, response: [] }],
    });
    await openSettings(page);
    await expect(page.locator('[data-slot="dialog-content"]')).toHaveCSS(
      'background-color',
      DIALOG,
    );
    await expect(page.locator('[data-slot="dialog-overlay"]')).toHaveCSS(
      'background-color',
      'rgba(0, 0, 0, 0.6)',
    );
  },
);

studioTest('the command palette is dark and a menu stays elevated', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [PG_CONN] }],
  });
  await emitWailsEvent(page, IPC.commandPalette, undefined);
  const dialog = page.locator('[data-slot="dialog-content"]');
  await expect(dialog).toHaveCSS('background-color', DIALOG);
  await expect(dialog.locator('[data-slot="command"]')).toHaveCSS('background-color', DIALOG);
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);

  await (await connectionRow(page, PG_CONN.name)).click({ button: 'right' });
  await expect(page.locator('[data-slot="dropdown-menu-content"]')).toHaveCSS(
    'background-color',
    ELEVATED,
  );
});
