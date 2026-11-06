import { registerRunsScenarios } from '@workbench/testing/ui/scenarios/automationsRuns';
import { studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { modeTab } from './support/apiMode';

// P242 Part 1: Automations runs on the mocked bridge. A run's push event is emitted by hand; the
// terminal id is the client-generated tab id, read back from the terminalOpen call.

registerRunsScenarios(
  { test, contract, ipc: IPC },
  {
    extraBase: [],
    modeTab: (page) => modeTab(page, 'automations'),
  },
);
