import { registerResumeScenarios } from '@workbench/testing/ui/scenarios/automationsResume';
import { test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P279: Resume on an interrupted smart run, on the mocked bridge.

registerResumeScenarios({ test, contract, ipc: IPC });
