import { registerOrderScenarios } from '@workbench/testing/ui/scenarios/automationsOrder';
import { test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P381: Move up/down. Backend half: termflow TestScriptOrder.

registerOrderScenarios(
  { test, contract, ipc: IPC },
  {
    extraBase: [{ channel: IPC.windowsEnsure, response: { mode: 'git' } }],
    modeTab: (page) => page.locator('[data-testid="mode-tab"][data-mode="automations"]'),
  },
);
