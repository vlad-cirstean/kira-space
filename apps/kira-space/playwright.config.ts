import { defineConfig, devices } from '@playwright/test';

// Titles of every test with a real-millisecond wall-clock assertion: budgets.spec.ts's and
// perf.spec.ts's own single test each, plus slick-grid.spec.ts's two "150ms sandbox gate" tests.
// `ui-timing` selects exactly these by title, not by file: slick-grid.spec.ts also holds 15 tests
// with no wall-clock assertion that gain nothing from running serially. `ui-studio` excludes them
// the same way, so nothing runs twice.
const wallClockBudgetTitles = /150ms sandbox gate|interaction budgets|perf tripwires/;

// One config for the one app. Every project runs against the same test build (`bun run build:test`)
// or the same `-tags server` binary. Studio-module specs live in `studio/` subfolders and get a
// `-studio` project beside the Space one: their baselines, fixtures and timing rules differ, and
// their fixtures select their module through the mode switcher.
//
// `browserName: 'webkit'` matches what a packaged build embeds (WKWebView on macOS, WebKitGTK on
// Linux). `bunx playwright install webkit` plus the system libs its post-install warning names
// (docs/DEV_ENVIRONMENT.md) fetches and runs a real one.
export default defineConfig({
  // The worker pool size. A project's own `workers` can only narrow it, never widen it.
  workers: '100%',
  // One retry on CI absorbs shared-runner timing noise while a flaky test still shows as "flaky".
  // 0 locally: a dev loop wants the fastest failure signal.
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
    {
      name: 'ui-studio',
      testDir: './tests/ui/studio',
      testIgnore: ['budgets.spec.ts', 'perf.spec.ts'],
      grepInvert: wallClockBudgetTitles,
      use: { browserName: 'webkit' },
      fullyParallel: true,
      workers: '100%',
    },
    // grid-copy-menu.spec.ts reads the real clipboard, which only Chromium allows. The same file
    // also runs in `ui-studio` (WebKit) against a pass-through recorder (support/clipboard.ts).
    {
      name: 'ui-clipboard',
      testDir: './tests/ui/studio',
      testMatch: ['grid-copy-menu.spec.ts'],
      use: { browserName: 'chromium' },
      fullyParallel: true,
      workers: '100%',
    },
    // The wall-clock specs measure real milliseconds, so other browsers contending for the CPU
    // are part of what they measure. They run alone, serially, after `ui-studio` has finished.
    // CI runs this with `--no-deps` (pr.yml). Locally it is out of `test:ui`: it needs an idle
    // host, so run `bun run test:ui:timing` alone.
    {
      name: 'ui-timing',
      testDir: './tests/ui/studio',
      testMatch: ['budgets.spec.ts', 'perf.spec.ts', 'slick-grid.spec.ts'],
      grep: wallClockBudgetTitles,
      use: { browserName: 'webkit' },
      fullyParallel: false,
      workers: 1,
      dependencies: ['ui-studio'],
    },
    {
      name: 'ipc-frontend',
      testDir: './tests/ipc',
      testMatch: '**/*.frontend.spec.ts',
      fullyParallel: true,
      workers: '100%',
    },
    // The phone app, on a phone-sized iOS (WebKit) profile.
    {
      name: 'mobile-ios',
      testDir: './tests/mobile',
      testIgnore: 'insecure-origin.spec.ts',
      use: { ...devices['iPhone 15'] },
      fullyParallel: true,
    },
    // insecure-origin.spec.ts opens the app on a non-loopback host name, which is not a secure
    // context the way localhost is. The host mapping is a Chromium launch arg, so this one spec
    // gets its own Chromium project.
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
    // Real Go server (-tags server), real SQLite/git/PTY/stream, real database adapters; fake
    // claude/gh. Chromium: the server build serves a plain browser page, no WebKit embed involved.
    // Only what cannot split at the IPC boundary stays here (boot, multi-window routing, keystroke
    // order, real Docker and database adapters); the rest is a flow test plus a tests/ui contract
    // spec. Each test gets its own homes and port, so two workers are safe.
    {
      name: 'e2e-real',
      testDir: './tests/e2e-real',
      testIgnore: 'studio/**',
      use: { browserName: 'chromium' },
      fullyParallel: true,
      workers: 2,
    },
    {
      name: 'e2e-real-studio',
      testDir: './tests/e2e-real/studio',
      use: { browserName: 'chromium' },
      fullyParallel: true,
      workers: 2,
    },
    // Pixel-diff tiers. Their own projects: a screenshot comparison, not a functional check.
    // Nothing degrades under CPU contention, so they run fully parallel. `animations: 'disabled'`
    // freezes every CSS animation to its terminal frame. `stylePath` collapses the font stacks to
    // their trailing generic keyword, removing run-to-run fontconfig ambiguity on the CI image that
    // is the sole baseline authority (never applied to `ui`, whose font-roles spec wants the real
    // stacks).
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
    {
      name: 'visual-studio',
      testDir: './tests/visual/studio',
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
    // Presentation screenshots (`bun run screenshots`). `.shot.ts` keeps these out of `ui` and
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
    {
      name: 'screenshots-studio',
      testDir: './tests/screenshots/studio',
      testMatch: '**/*.shot.ts',
      use: { browserName: 'webkit' },
      fullyParallel: false,
      workers: 1,
      retries: 0,
    },
  ],
});
