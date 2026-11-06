import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { modeTab, openHttpModeAndNewRequest } from './support/apiMode';

// A window cannot take another window's tab id: the save is refused with E_CONTRACT, the strip
// keeps its tabs and the next save retries. A new terminal sends a v7 id.
// Contract tabs-scope. Backend half: windowflow TestTabsScope, termflow TestTerminalIDBounds.

const V7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const refused = contract<{ code: string; message: string }>(
  'tabs-scope-db',
  'TabsService.Save#ownedElsewhere',
);

async function newTerminal(page: import('@playwright/test').Page): Promise<void> {
  await modeTab(page, 'automations').click();
  await page.locator('[data-testid="tab-strip-new"]').click();
  await page.locator('[data-testid="menu-item-new-terminal"]').click();
}

test('a refused tab save keeps the strip and the next save retries', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.tabsSave, error: refused },
      { channel: IPC.tabsSave, response: null },
    ],
  });
  const saves = () => control.log().filter((e) => e.channel === IPC.tabsSave).length;
  await openHttpModeAndNewRequest(page);
  const tabs = page.locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"]');
  await expect(tabs).toHaveCount(1);
  await expect.poll(saves, { timeout: 20_000 }).toBeGreaterThan(0);

  await page.fill('[data-testid="http-url"]', 'https://api.example.com/v1/x');
  await expect(tabs).toHaveCount(1);
  await expect.poll(saves, { timeout: 20_000 }).toBeGreaterThan(1);
});

test('a new terminal tab sends a v7 terminalId', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } }],
  });
  await newTerminal(page);
  await expect
    .poll(
      () =>
        (
          control.log().find((e) => e.channel === IPC.terminalOpen)?.args as
            | { terminalId?: string }
            | undefined
        )?.terminalId,
    )
    .toMatch(V7);
});
