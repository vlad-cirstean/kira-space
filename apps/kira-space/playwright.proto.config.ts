import { defineConfig } from '@playwright/test';

// P165 grid prototype specs against `frontend/dist-proto` (hooks build). Own config so no normal
// tier runs them. `bun run test:proto:studio`; docs/DEV_ENVIRONMENT.md "Grid prototype (P165)".
export default defineConfig({
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 60_000,
  reporter: [['list']],
  outputDir: 'test-results-proto',
  projects: [
    {
      name: 'proto',
      testDir: './tests/proto',
      use: { browserName: 'webkit' },
    },
  ],
});
