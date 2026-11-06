import { registerRestoreScenarios } from '@workbench/testing/ui/scenarios/automationsRestore';
import { test } from './fixtures';
import { modeTab } from './support/apiMode';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P441: the Runs restore banner and the shared restore dialog, on the mocked bridge.

registerRestoreScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [], modeTab: (page) => modeTab(page, 'automations') },
);
