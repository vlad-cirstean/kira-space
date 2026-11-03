import { registerSmartScenarios } from '@workbench/testing/ui/scenarios/automationsSmart';
import { test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P242 Part 2: smart scripts on the mocked bridge: the run dialog, the run tab.

registerSmartScenarios({ test, contract, ipc: IPC });
