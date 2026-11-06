import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import {
  createUiFixtures,
  type Relaunch as RelaunchOf,
  type UiFixturesOptions,
} from '@workbench/testing/ui/fixtures';
import {
  installMockStream,
  type MockStreamHandle,
  type StudioPortSnapshot,
} from './studio/support/mockStream';
import { mergeBootSnapshots } from './support/bootSnapshots';
import { type GitStreamMockArgs, installGitStreamMockOnInit } from './support/gitStreamMock';
import { type ControlMockHandle, installControlMocks } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// P103 Part 4 (closing audit, §10): tests/ui/support/server.ts hoisted to
// @workbench/testing/ui/server — see that file's own doc comment for why it now takes `distDir`.
const DIST_DIR = resolve(__dirname, '../../frontend/dist');

interface KiraAppExtra {
  control: ControlMockHandle;
  stream: MockStreamHandle;
}

export type KiraApp = { window: Page } & KiraAppExtra;

export interface RelaunchOptions extends UiFixturesOptions {
  control?: readonly ControlSnapshot[];
  /** Bulk-data grid pages the databases module streams (`streamFactory('data')`). */
  stream?: readonly StudioPortSnapshot[];
  /** Set only when boot itself opens the git transport (a restored, active repo-file
   *  tab) — installed via `installGitStreamMockOnInit`, before `relaunch()`'s one navigation. See
   *  `support/gitStreamMock.ts`'s own doc comment for why that's a different install path than the
   *  per-spec `installGitStreamMock` default. */
  gitStream?: GitStreamMockArgs;
  /** localStorage entries seeded before the page's scripts run. */
  storage?: Readonly<Record<string, string>>;
}

/** The review window's agent panel is off by default; specs that drive it seed it on. */
export const AGENT_PANEL_ON = { 'kira.ade.review.agentPanel': 'true' } as const;

export type Relaunch = RelaunchOf<KiraAppExtra, RelaunchOptions>;

/**
 * `relaunch()` opens a fresh page against the static build and mocks both wire protocols from
 * scratch; nothing persists across it. Every install is a page.route/addInitScript registration
 * and must land before the one navigation. The data-grid stream mock installs before the git
 * stream mock: the git mock chains the factory already on the page.
 */
const installMocks = async (page: Page, options?: RelaunchOptions): Promise<KiraAppExtra> => {
  const control = await installControlMocks(page, mergeBootSnapshots(options?.control ?? []));
  const stream = await installMockStream(page, options?.stream ?? []);
  if (options?.storage) {
    await page.addInitScript((entries: Record<string, string>) => {
      for (const [key, value] of Object.entries(entries)) localStorage.setItem(key, value);
    }, options.storage);
  }
  if (options?.gitStream) {
    await installGitStreamMockOnInit(page, options.gitStream);
  }
  return { control, stream };
};

export const test = createUiFixtures<KiraAppExtra, RelaunchOptions>({
  distDir: DIST_DIR,
  installMocks,
});

/**
 * The merged app boots into its first module. Databases-module specs select it through the mode
 * switcher after every launch; a spec that opens a window kind directly (`?window=`) navigates on
 * its own afterwards and is unaffected.
 */
export const studioTest = test.extend<{ relaunch: Relaunch }>({
  relaunch: async ({ relaunch }, use) => {
    await use(async (options) => {
      const app = await relaunch(options);
      await app.window.locator('[data-testid="mode-tab"][data-mode="studio"]').click();
      return app;
    });
  },
});

/** A review window has no status bar: it is ready once its own root renders. */
export const reviewTest = createUiFixtures<KiraAppExtra, RelaunchOptions>({
  distDir: DIST_DIR,
  installMocks,
  readySelector: '[data-testid="ade-review-window"]',
});

/** The files window has no status bar: it is ready once its own root renders. */
export const filesTest = createUiFixtures<KiraAppExtra, RelaunchOptions>({
  distDir: DIST_DIR,
  installMocks,
  readySelector: '[data-testid="opened-files-window"]',
});

export { expect } from '@playwright/test';
