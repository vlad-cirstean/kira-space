import { defineConfig, devices } from '@playwright/test';

// Kira Studio's own playwright.config.ts, trimmed to the one project this app's own
// tests/ui/ tier needs — no `ui-timing` (no wall-clock-budget spec moved here), no `ipc-frontend`
// (this app has no tests/ipc/ tier), no `e2e-real` (no
// real-Go-backend tier built for this app). `fullyParallel`/`browserName: 'webkit'` match Kira
// Studio's own `ui` project for the same reason theirs does: a packaged build embeds
// WebKit (WKWebView/WebKitGTK), and this tier has none of the wall-clock/Docker contention that
// forced Studio's own `e2e` tier to serialize.
//
// `visual` added, mirroring Kira Studio's own project (playwright.config.ts:100-110) —
// same `webkit`/`animations: disabled`/pinned-fonts contract, so this app's own baselines are
// authoritative against the same CI-Linux font/rendering policy (docs/DEV_ENVIRONMENT.md's
// `tests/visual/*` section applies here identically).
export default defineConfig({
  workers: '100%',
  retries: process.env.CI ? 1 : 0,
  timeout: 60_000,
  reporter: 'line',
  use: { screenshot: 'only-on-failure', trace: 'on-first-retry' },
  outputDir: 'test-results',
  projects: [
    {
      name: 'ui',
      testDir: './tests/ui',
      testIgnore: 'studio/**',
      use: { browserName: 'webkit' },
      fullyParallel: true,
      workers: '100%',
    },
    // P212: the phone app, on a phone-sized iOS (WebKit) profile.
    {
      name: 'mobile-ios',
      testDir: './tests/mobile',
      testIgnore: 'insecure-origin.spec.ts',
      use: { ...devices['iPhone 15'] },
      fullyParallel: true,
    },
    // P223: insecure-origin.spec.ts opens the app on a non-loopback host name, which is not a
    // secure context the way localhost is. The host mapping is a Chromium launch arg, so this
    // one spec gets its own Chromium project.
    {
      name: 'mobile-insecure',
      testDir: './tests/mobile',
      testMatch: 'insecure-origin.spec.ts',
      use: {
        ...devices['iPhone 15'],
        browserName: 'chromium',
        launchOptions: { args: ['--host-resolver-rules=MAP kira-lan.test 127.0.0.1'] },
      },
    },
    // Real Go server (-tags server), real SQLite/git/PTY/stream; fake claude/gh. Chromium: the
    // server build serves a plain browser page, no WebKit embed involved. Only what cannot split at
    // the IPC boundary stays here (boot, multi-window routing, keystroke order); the rest is a flow
    // test plus a tests/ui contract spec.
    {
      name: 'e2e-real',
      testDir: './tests/e2e-real',
      testIgnore: 'studio/**',
      use: { browserName: 'chromium' },
      fullyParallel: true,
      workers: 2,
    },
    {
      name: 'visual',
      testDir: './tests/visual',
      testIgnore: 'studio/**',
      use: { browserName: 'webkit' },
      fullyParallel: true,
      workers: '100%',
      expect: {
        toHaveScreenshot: {
          animations: 'disabled',
          stylePath: './tests/visual/support/pin-fonts.css',
        },
      },
    },
    // P280: presentation screenshots (`bun run screenshots`). `.shot.ts` keeps these out of `ui` and
    // `visual`. Serial: the output files are shared.
    {
      name: 'screenshots',
      testDir: './tests/screenshots',
      testIgnore: 'studio/**',
      testMatch: '**/*.shot.ts',
      use: { browserName: 'webkit' },
      fullyParallel: false,
      workers: 1,
      retries: 0,
    },
  ],
});
