import { registerTabScenarios } from '@workbench/testing/ui/scenarios/automationsTabs';
import { test } from './fixtures';
import { modeTab } from './support/apiMode';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P380: one tab per script, play-only start. Backend half: termflow TestScriptRunTwice.

registerTabScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [], modeTab: (page) => modeTab(page, 'automations') },
);
