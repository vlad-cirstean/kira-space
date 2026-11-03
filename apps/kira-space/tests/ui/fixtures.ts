import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import { createUiFixtures, type UiFixturesOptions } from '@workbench/testing/ui/fixtures';
import { mergeBootSnapshots } from './support/bootSnapshots';
import { type GitStreamMockArgs, installGitStreamMockOnInit } from './support/gitStreamMock';
import { type ControlMockHandle, installControlMocks } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// P103 Part 4 (closing audit, §10): tests/ui/support/server.ts hoisted to
// @workbench/testing/ui/server — see that file's own doc comment for why it now takes `distDir`.
const DIST_DIR = resolve(__dirname, '../../frontend/dist');

interface KiraAppExtra {
  control: ControlMockHandle;
}

export type KiraApp = { window: Page } & KiraAppExtra;

export interface RelaunchOptions extends UiFixturesOptions {
  control?: readonly ControlSnapshot[];
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

/**
 * Kira Studio's own `tests/ui/fixtures.ts`, ported — `relaunch()` means "open a fresh page against
 * the static build and mock the control-call wire protocol from scratch". No `stream`/`MockStreamHandle`
 * fixture here: that whole mechanism (mockStream.ts) is Studio's own bulk-data grid-page protocol
 * (`window._wails.streamFactory('data')`), which this app has no counterpart for — the one stream
 * this app's own repo-graph mount opens (`window._wails.streamFactory('gitgraph')`) is a different wire
 * entirely, mocked per-spec by `installGitStreamMock` (support/gitStreamMock.ts) by default, or via
 * this fixture's own `gitStream` option when boot itself opens the transport.
 */
const installMocks = async (page: Page, options?: RelaunchOptions): Promise<KiraAppExtra> => {
  const control = await installControlMocks(page, mergeBootSnapshots(options?.control ?? []));
  if (options?.storage) {
    await page.addInitScript((entries: Record<string, string>) => {
      for (const [key, value] of Object.entries(entries)) localStorage.setItem(key, value);
    }, options.storage);
  }
  if (options?.gitStream) {
    await installGitStreamMockOnInit(page, options.gitStream);
  }
  return { control };
};

export const test = createUiFixtures<KiraAppExtra, RelaunchOptions>({
  distDir: DIST_DIR,
  installMocks,
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
