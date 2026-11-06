import { registerOpsScenarios } from '@workbench/testing/ui/scenarios/automationsOps';
import { studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { modeTab } from './support/apiMode';

// Automation runs in the Operations panel. Backend half: termflow TestScriptRunNonZeroExit.

registerOpsScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [], modeTab: (page) => modeTab(page, 'automations') },
);
