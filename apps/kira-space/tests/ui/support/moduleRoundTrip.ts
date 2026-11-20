import type { Page } from '@playwright/test';
import { expect } from '@playwright/test';

export type ModuleMode = 'ade' | 'automations' | 'git' | 'studio' | 'api' | 'docker' | 'kubernetes';

// One testid per module that is on screen only once the module's panel (or, for Agents, its full
// view) has mounted.
const READY: Record<ModuleMode, string> = {
  ade: 'ade-shell',
  automations: 'automations-panel',
  git: 'toggle-search',
  studio: 'toggle-search',
  api: 'toggle-search',
  docker: 'docker-panel',
  kubernetes: 'kubernetes-panel',
};

export async function switchModule(page: Page, mode: ModuleMode): Promise<void> {
  const tab = page.locator(`[data-testid="mode-tab"][data-mode="${mode}"]`);
  await tab.click();
  await expect(tab).toHaveClass(/is-active/);
  await expect(page.locator(`[data-testid="${READY[mode]}"]`).first()).toBeVisible();
}

/** Leaves `back` for `away` and returns, so every surface of `back` remounts. */
export async function roundTrip(page: Page, away: ModuleMode, back: ModuleMode): Promise<void> {
  await switchModule(page, away);
  await switchModule(page, back);
}
