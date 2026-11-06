import { resolve } from 'node:path';
import { test as base, type Page } from '@playwright/test';
import { startServer, type UiServer } from '@workbench/testing/ui/server';

const DIST_DIR = resolve(__dirname, '../../frontend/dist-proto-hooks');

export type ProtoPage = 'cheetah' | 'slick';

interface ProtoFixtures {
  /** Opens the prototype grid with the features fixture and waits for the debug hook. */
  grid: (query?: string) => Promise<Page>;
  consoleErrors: string[];
}

export const test = base.extend<ProtoFixtures, { server: UiServer }>({
  server: [
    // biome-ignore lint/correctness/noEmptyPattern: Playwright requires a literal destructuring pattern here, even with no fixture deps.
    async ({}, use) => {
      const server = await startServer(DIST_DIR);
      await use(server);
      await server.close();
    },
    { scope: 'worker' },
  ],
  // biome-ignore lint/correctness/noEmptyPattern: Playwright requires a literal destructuring pattern here, even with no fixture deps.
  consoleErrors: async ({}, use) => {
    await use([]);
  },
  grid: async ({ page, server, consoleErrors }, use) => {
    page.on('console', (msg) => {
      if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    await page.setViewportSize({ width: 1440, height: 960 });
    await use(async (query = '?fixture=features') => {
      await page.goto(`${server.url}proto/grid/cheetah.html${query}`);
      await page.waitForFunction(() => window.__kiraGridProto !== undefined);
      return page;
    });
  },
});

export { expect } from '@playwright/test';
