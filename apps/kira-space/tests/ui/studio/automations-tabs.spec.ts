import { registerTabScenarios } from '@workbench/testing/ui/scenarios/automationsTabs';
import { studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { modeTab } from './support/apiMode';

// P380: one tab per script, play-only start. Backend half: termflow TestScriptRunTwice.

registerTabScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [], modeTab: (page) => modeTab(page, 'automations') },
);
