import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import {
  createUiFixtures,
  type Relaunch as RelaunchOf,
  type UiFixturesOptions,
} from '@workbench/testing/ui/fixtures';
import type { ControlSnapshot } from '../../ipc/support/types';
import { mergeBootSnapshots } from './support/bootSnapshots';
import { type ControlMockHandle, installControlMocks } from './support/mockRuntime';
import {
  installMockStream,
  type MockStreamHandle,
  type StudioPortSnapshot,
} from './support/mockStream';

// P103 Part 4 (closing audit, §10): tests/ui/support/server.ts hoisted to
// @workbench/testing/ui/server — see that file's own doc comment for why it now takes `distDir`.
const DIST_DIR = resolve(__dirname, '../../../frontend/dist');

interface KiraAppExtra {
  control: ControlMockHandle;
  stream: MockStreamHandle;
}

export type KiraApp = { window: Page } & KiraAppExtra;

export interface RelaunchOptions extends UiFixturesOptions {
  control?: readonly ControlSnapshot[];
  stream?: readonly StudioPortSnapshot[];
  /** Playwright's own `BrowserContextOptions.timezoneId` (e.g. `'America/New_York'`) — this
   *  sandbox's own system timezone is UTC, which
   *  silently makes any "local time differs from UTC" assertion vacuously true-or-false depending
   *  on the host machine rather than the app's own behaviour, since `browser.newPage()` otherwise
   *  inherits the host's real zone. A spec that genuinely needs Local and UTC to differ should
   *  pass this rather than assume the host isn't UTC. */
  timezoneId?: string;
}

export type Relaunch = RelaunchOf<KiraAppExtra, RelaunchOptions>;

/**
 * `relaunch()` means "open a fresh page against the static build and mock both wire protocols from
 * scratch", not "restart a real app process". There is nothing to persist to, so a spec cannot
 * assert state survives a restart.
 */
const baseTest = createUiFixtures<KiraAppExtra, RelaunchOptions>({
  distDir: DIST_DIR,
  installMocks: async (page, options) => {
    // Both installs are page.route/addInitScript registrations — they must land before the first
    // navigation, exactly like tests/ipc frontend specs' own `installControlMocks` +
    // `installMockPort` ordering (mockPort.ts's own comment: "must be called after the last
    // reload" — here there is exactly one navigation, so "before it" is the only rule).
    const control = await installControlMocks(page, mergeBootSnapshots(options?.control ?? []));
    const stream = await installMockStream(page, options?.stream ?? []);
    return { control, stream };
  },
});

/**
 * The merged app boots into its first module. Studio specs expect the databases module, so every
 * launch selects it through the mode switcher before the spec runs. A spec that opens a window
 * kind directly (`?window=`) navigates on its own afterwards and is unaffected.
 */
export const test = baseTest.extend<{ relaunch: Relaunch }>({
  relaunch: async ({ relaunch }, use) => {
    await use(async (options) => {
      const app = await relaunch(options);
      await app.window.locator('[data-testid="mode-tab"][data-mode="studio"]').click();
      return app;
    });
  },
});

export { expect } from '@playwright/test';
