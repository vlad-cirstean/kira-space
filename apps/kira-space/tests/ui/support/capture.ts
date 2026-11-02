import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import { createUiFixtures } from '@workbench/testing/ui/fixtures';
import type { RelaunchOptions } from '../fixtures';
import { mergeBootSnapshots } from './bootSnapshots';
import { contract } from './contract';
import { IPC } from './ipcChannels';
import { type ControlMockHandle, installControlMocks } from './mockRuntime';
import type { ControlSnapshot } from './types';

// A capture popup is ready once its root renders; it has no status bar. installMocks is a reduced
// copy of fixtures.ts's, same as hostView.ts.
const DIST_DIR = resolve(__dirname, '../../../frontend/dist');

const installMocks = async (
  page: Page,
  options?: RelaunchOptions,
): Promise<{ control: ControlMockHandle }> => ({
  control: await installControlMocks(page, mergeBootSnapshots(options?.control ?? [])),
});

export const captureTest = createUiFixtures<{ control: ControlMockHandle }, RelaunchOptions>({
  distDir: DIST_DIR,
  installMocks,
  readySelector: '[data-testid="capture-window"]',
});

export const t = (id: string) => `[data-testid="${id}"]`;

export interface Item {
  id: string;
  kind: string;
  text: string;
  truncated: boolean;
  label: string;
  title: string;
}

export const changeRequestItem = contract<Item>(
  'capture',
  'CaptureService.WindowItem#changeRequest',
);
export const issueItem = contract<Item>('capture', 'CaptureService.WindowItem#issue');
export const textItem = contract<Item>('capture', 'CaptureService.WindowItem#text');
export const emptyItem = contract<Item>('capture', 'CaptureService.WindowItem#empty');
export const laterResult = contract<Record<string, unknown>>(
  'capture',
  'CaptureService.Confirm#later',
);
export const noBranchResult = contract<Record<string, unknown>>(
  'capture',
  'CaptureService.Confirm#noBranch',
);
export const failedStatus = contract<Record<string, unknown>>(
  'capture',
  'CaptureService.Status#failed',
);
export const requestedStatus = contract<Record<string, unknown>>(
  'capture',
  'CaptureService.Status#requested',
);
export const offStatus = contract<Record<string, unknown>>('capture', 'CaptureService.Status#off');

export function captureControl(
  item: Item,
  extra: readonly ControlSnapshot[] = [],
): ControlSnapshot[] {
  return [{ channel: IPC.captureWindowItem, response: item }, ...extra];
}

export const calls = (control: ControlMockHandle, channel: string) =>
  control.log().filter((e) => e.channel === channel);
