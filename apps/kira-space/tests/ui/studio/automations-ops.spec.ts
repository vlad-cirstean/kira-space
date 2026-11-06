import { registerOpsScenarios } from '@workbench/testing/ui/scenarios/automationsOps';
import { test } from './fixtures';
import { modeTab } from './support/apiMode';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// Automation runs in the Operations panel. Backend half: termflow TestScriptRunNonZeroExit.

registerOpsScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [], modeTab: (page) => modeTab(page, 'automations') },
);
