import { registerFileScenarios } from '@workbench/testing/ui/scenarios/automationsFile';
import { test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P472: a run is a YAML file; the panel lists them and a dialog edits the text. Backend half:
// termflow TestRunsConfigFile stores the runs-file contract.

registerFileScenarios(
  { test, contract, ipc: IPC },
  {
    extraBase: [{ channel: IPC.windowsEnsure, response: { mode: 'git' } }],
    modeTab: (page) => page.locator('[data-testid="mode-tab"][data-mode="automations"]'),
  },
);
