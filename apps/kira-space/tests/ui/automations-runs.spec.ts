import { registerRunsScenarios } from '@workbench/testing/ui/scenarios/automationsRuns';
import { test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P242 Part 1: Automations runs on the mocked bridge. A run's push event is emitted by hand; the
// terminal id is the client-generated tab id, read back from the terminalOpen call.

registerRunsScenarios(
  { test, contract, ipc: IPC },
  {
    extraBase: [{ channel: IPC.windowsEnsure, response: { mode: 'git' } }],
    modeTab: (page) => page.locator('[data-testid="mode-tab"][data-mode="automations"]'),
  },
);
