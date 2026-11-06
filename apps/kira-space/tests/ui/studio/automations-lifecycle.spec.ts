import { registerLifecycleScenarios } from '@workbench/testing/ui/scenarios/automationsLifecycle';
import { studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { modeTab } from './support/apiMode';

// P514: run collection rename, delete and move, and runs dragged between sets. Backend half:
// termflow TestRunsLifecycle stores the runs-lifecycle contract.

registerLifecycleScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [], modeTab: (page) => modeTab(page, 'automations') },
);
