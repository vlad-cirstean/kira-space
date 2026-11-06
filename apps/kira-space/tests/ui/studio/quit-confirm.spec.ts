import { registerQuitScenarios } from '@workbench/testing/ui/scenarios/quitConfirm';
import { test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P279: the quit confirmation, on the mocked bridge.

registerQuitScenarios({ test, contract, ipc: IPC });
