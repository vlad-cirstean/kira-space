import { registerSecretScenarios } from '@workbench/testing/ui/scenarios/automationsSecrets';
import { test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P447: write-only saved secret params in the editor and run dialog, on the mocked bridge.

registerSecretScenarios({ test, contract, ipc: IPC });
