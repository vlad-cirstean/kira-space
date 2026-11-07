import type { Page } from '@playwright/test';
import { expect } from '@playwright/test';
import { defaultSettings, type Settings } from '../../../frontend/src/state/settingsDomain';
import { IPC } from './ipcChannels';
import type { ControlSnapshot } from './types';

export async function openMemory(page: Page): Promise<void> {
  await page.locator('[data-testid="mode-tab"][data-mode="memory"]').click();
  await expect(page.locator('[data-testid="memory-panel"]')).toBeVisible();
}

export const MEMORY_ON: Settings = { ...defaultSettings, experimental: { memory: true } };

/** Turns `experimental.memory` on in a spec's boot snapshots: merges into its own settingsGetAll
 *  answer when it has one, else adds one. */
export function withMemory(control: ControlSnapshot[] = []): ControlSnapshot[] {
  const own = control.find((c) => c.channel === IPC.settingsGetAll);
  if (!own) return [...control, { channel: IPC.settingsGetAll, response: MEMORY_ON }];
  const response = {
    ...(own.response as Settings),
    experimental: { ...(own.response as Settings).experimental, memory: true },
  };
  return control.map((c) => (c === own ? { ...own, response } : c));
}
