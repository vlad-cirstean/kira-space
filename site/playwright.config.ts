import { defineConfig } from '@playwright/test';

const PORT = 4329;
const ORIGIN = `http://127.0.0.1:${PORT}`;

// `bun run site:check` builds first; this serves that build under the Pages base path.
export default defineConfig({
  testDir: './tests',
  reporter: 'list',
  retries: 0,
  use: { baseURL: ORIGIN },
  webServer: {
    command: 'bun run preview',
    url: `${ORIGIN}/kira-space/`,
    reuseExistingServer: !process.env.CI,
  },
  projects: [
    {
      name: 'phone',
      use: {
        browserName: 'webkit',
        viewport: { width: 375, height: 812 },
        isMobile: true,
        hasTouch: true,
        deviceScaleFactor: 2,
      },
    },
    {
      name: 'desktop',
      use: { browserName: 'webkit', viewport: { width: 1280, height: 800 } },
    },
  ],
});
