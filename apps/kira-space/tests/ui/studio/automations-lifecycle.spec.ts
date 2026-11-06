import { registerLifecycleScenarios } from '@workbench/testing/ui/scenarios/automationsLifecycle';
import { test } from './fixtures';
import { modeTab } from './support/apiMode';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P514: run collection rename, delete and move, and runs dragged between sets. Backend half:
// termflow TestRunsLifecycle stores the runs-lifecycle contract.

registerLifecycleScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [], modeTab: (page) => modeTab(page, 'automations') },
);
