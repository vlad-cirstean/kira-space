import { registerFileScenarios } from '@workbench/testing/ui/scenarios/automationsFile';
import { studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { modeTab } from './support/apiMode';

// P472: a run is a YAML file; the panel lists them and a dialog edits the text. Backend half:
// termflow TestRunsConfigFileStudio stores the runs-file contract.

registerFileScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [], modeTab: (page) => modeTab(page, 'automations') },
);
