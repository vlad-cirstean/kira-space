import { defineConfig } from '@playwright/test';

// Perf probes: report numbers, assert nothing. Own config so no normal tier ever runs them.
// `bun run perf:probe`; docs/DEV_ENVIRONMENT.md "Perf probes". `tests/perf/studio/` holds the
// Studio-module scroll and parse probes; Studio RAM and heap worlds sit in `heap-ram`/`heap-tabs`.
export default defineConfig({
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 180_000,
  reporter: [['list']],
  outputDir: 'test-results-perf',
  projects: [
    {
      name: 'perf',
      testDir: './tests/perf',
      testIgnore: 'heap-*.spec.ts',
      timeout: 600_000,
      use: { browserName: 'webkit', headless: !process.env.PERF_HEADED },
    },
    // JS heap per tab kind and RAM attribution need CDP, so Chromium.
    // `--project heap heap-tabs` or `heap-ram`.
    {
      name: 'heap',
      testDir: './tests/perf',
      testMatch: 'heap-*.spec.ts',
      timeout: 600_000,
      use: { browserName: 'chromium', headless: !process.env.PERF_HEADED },
    },
    // Process RSS only (no CDP) on the shipped engine family.
    {
      name: 'heap-webkit',
      testDir: './tests/perf',
      testMatch: 'heap-ram.spec.ts',
      timeout: 600_000,
      use: { browserName: 'webkit', headless: !process.env.PERF_HEADED },
    },
  ],
});
