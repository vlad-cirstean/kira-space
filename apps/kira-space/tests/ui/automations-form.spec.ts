import { registerFormScenarios } from '@workbench/testing/ui/scenarios/automationsForm';
import { test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P514: a run file as a form and as YAML over the same draft. Backend half: termflow
// TestRunsForm stores the runs-form contract.

registerFormScenarios(
  { test, contract, ipc: IPC },
  {
    extraBase: [{ channel: IPC.windowsEnsure, response: { mode: 'git' } }],
    modeTab: (page) => page.locator('[data-testid="mode-tab"][data-mode="automations"]'),
    mcp: true,
  },
);
