import { registerTerminalLinkScenarios } from '@workbench/testing/ui/scenarios/terminalLinks';
import { test } from './fixtures';
import { modeTab } from './support/apiMode';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// Cmd/Ctrl-click opens a terminal link. Backend half: appflow TestTerminalLinkOpen.

registerTerminalLinkScenarios(
  { test, contract, ipc: IPC },
  {
    extraBase: [],
    openTerminal: async (page) => {
      await modeTab(page, 'automations').click();
      await page.locator('[data-testid="tab-strip-new"]').click();
      await page.locator('[data-testid="menu-item-new-terminal"]').click();
    },
  },
);
