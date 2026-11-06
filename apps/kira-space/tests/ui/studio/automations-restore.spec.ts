import { registerRestoreScenarios } from '@workbench/testing/ui/scenarios/automationsRestore';
import { studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { modeTab } from './support/apiMode';

// P441: the Runs restore banner and the shared restore dialog, on the mocked bridge.

registerRestoreScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [], modeTab: (page) => modeTab(page, 'automations') },
);
