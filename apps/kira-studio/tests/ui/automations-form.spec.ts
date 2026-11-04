import { registerFormScenarios } from '@workbench/testing/ui/scenarios/automationsForm';
import { test } from './fixtures';
import { modeTab } from './support/apiMode';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P514: a run file as a form and as YAML over the same draft. Backend half: termflow
// TestRunsFormStudio stores the runs-form contract.

registerFormScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [], modeTab: (page) => modeTab(page, 'automations') },
);
