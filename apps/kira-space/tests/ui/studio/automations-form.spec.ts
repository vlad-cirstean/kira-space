import { registerFormScenarios } from '@workbench/testing/ui/scenarios/automationsForm';
import { studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { modeTab } from './support/apiMode';

// P514: a run file as a form and as YAML over the same draft. Backend half: termflow
// TestRunsFormStudio stores the runs-form contract.

registerFormScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [], modeTab: (page) => modeTab(page, 'automations'), mcp: true },
);
