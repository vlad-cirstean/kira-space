import { spawn } from 'node:child_process';
import { mkdir, mkdtemp, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test as base, type Page } from '@playwright/test';
import { bound, getFreePort, waitForHealth } from '@workbench/testing/e2eReal';
import { APP_DIR, buildPrerequisites, FAKE_AGENT_BINARY, SERVER_BINARY } from '../support/build';
import { type FlowServers, startFlowServers } from './support/flowServers';

// The Studio modules' real-backend tier: the merged app's `go build -tags server` binary (built
// once per run by ../support/build.ts, shared with the Space tier), a real embedded engine and a
// real database adapter, reached over plain HTTP/WebSocket by a plain Chromium tab. Each test
// selects its module through the mode switcher after boot.

const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge';

/** Opens a Studio module; the merged app boots into the first module, not the databases one. */
async function selectMode(page: Page, mode: string): Promise<void> {
  await page.locator(`[data-testid="mode-tab"][data-mode="${mode}"]`).click();
}

export interface KiraApp {
  window: Page;
  baseURL: string;
  /** Calls a bound service method on the real server, e.g. `call('VariablesService', 'Upsert', args)`. */
  call: <T>(service: string, method: string, args?: unknown) => Promise<T>;
  /** Kills the server (SIGKILL) and starts it again on the same KIRA_HOME and port, then reloads the page. */
  relaunch: () => Promise<void>;
}

interface KiraFixtures {
  kiraHome: string;
  consoleErrors: string[];
  /** Extra environment for the spawned server (merged last): `test.use({ serverEnv: { HOME } })`. */
  serverEnv: Record<string, string>;
  /** Fake claude scenario (internal/flowtest/fakeagent). Set, the server gets a scratch HOME whose login
   *  shell puts the fake claude first on PATH; unset, the environment is untouched. */
  scenario: object | undefined;
  kira: KiraApp;
  /** The flow harness's real HTTP, HTTPS and gRPC servers, one set per test so recorded requests never leak between tests. */
  flowServers: FlowServers;
}

export const test = base.extend<KiraFixtures, { prerequisites: true }>({
  // Worker-scoped with its own timeout: the build can outlast a test's 60s, and a test fixture
  // would charge it to the first test of every worker.
  prerequisites: [
    // biome-ignore lint/correctness/noEmptyPattern: Playwright requires a literal destructuring pattern here, even with no fixture deps.
    async ({}, use) => {
      await buildPrerequisites();
      await use(true);
    },
    { scope: 'worker', auto: true, timeout: 600_000 },
  ],

  serverEnv: [{}, { option: true }],

  scenario: [undefined, { option: true }],

  flowServers: [
    // biome-ignore lint/correctness/noEmptyPattern: Playwright requires a literal destructuring pattern here, even with no fixture deps.
    async ({}, use) => {
      const { servers, stop } = await startFlowServers();
      await use(servers);
      await stop();
    },
    { scope: 'test' },
  ],

  // biome-ignore lint/correctness/noEmptyPattern: Playwright requires a literal destructuring pattern here, even with no fixture deps.
  kiraHome: async ({}, use) => {
    const dir = await mkdtemp(join(tmpdir(), 'kira-e2e-real-'));
    await use(dir);
    await rm(dir, { recursive: true, force: true });
  },

  // biome-ignore lint/correctness/noEmptyPattern: Playwright requires a literal destructuring pattern here, even with no fixture deps.
  consoleErrors: async ({}, use) => {
    await use([]);
  },

  kira: async ({ browser, kiraHome, consoleErrors, serverEnv, scenario }, use) => {
    // Non-negotiable per tests/e2e/fixtures.ts's own precedent (P25 D10): a real ~/.kira-studio
    // must never be touched by a test run.
    if (!kiraHome.startsWith(tmpdir())) {
      throw new Error(`kiraHome fixture "${kiraHome}" is not under the OS tmpdir`);
    }

    const fake: Record<string, string> = {};
    if (scenario) {
      const home = join(kiraHome, 'fake', 'home');
      const binDir = join(kiraHome, 'fake', 'bin');
      const fakeDir = join(kiraHome, 'fake', 'calls');
      await Promise.all([home, binDir, fakeDir].map((d) => mkdir(d, { recursive: true })));
      // Run shells are login shells that reorder PATH; their profile keeps the fake claude first.
      const profile = `export PATH=${JSON.stringify(binDir)}:"$PATH"\n`;
      const scenarioPath = join(kiraHome, 'fake', 'scenario.json');
      await Promise.all([
        ...['.bash_profile', '.zprofile', '.profile'].map((f) => writeFile(join(home, f), profile)),
        symlink(FAKE_AGENT_BINARY, join(binDir, 'claude')),
        writeFile(scenarioPath, JSON.stringify(scenario)),
      ]);
      Object.assign(fake, {
        HOME: home,
        PATH: `${binDir}:${process.env.PATH ?? ''}`,
        KIRA_FAKE_DIR: fakeDir,
        KIRA_FAKE_SCEN: scenarioPath,
      });
    }

    const port = await getFreePort();
    const env: NodeJS.ProcessEnv = {
      ...process.env,
      ...fake,
      KIRA_HOME: kiraHome,
      KIRA_SPACE_HOME: join(kiraHome, 'space'),
      KIRA_MEMORY_HOME: join(kiraHome, 'memory'),
      // Linux has no real keychain backing (CLAUDE.md) — same Linux-only development fallback
      // tests/e2e/fixtures.ts already sets, a no-op on macOS.
      KIRA_INSECURE_SECRETS: '1',
      // Non-negotiable: bind to 127.0.0.1 always, set explicitly rather than trusted as a
      // default. There is no authentication of any kind on /wails/runtime — a server-mode binary
      // exposes the entire bound surface, secrets and file services included, to anyone who can
      // reach the port. This binary is a test artifact and must never be reachable beyond
      // loopback, let alone packaged or shipped.
      WAILS_SERVER_HOST: '127.0.0.1',
      WAILS_SERVER_PORT: String(port),
      ...serverEnv,
    };

    const baseURL = `http://127.0.0.1:${port}`;
    interface Server {
      proc: ReturnType<typeof spawn>;
      exited: Promise<number | null>;
    }
    const start = async (): Promise<Server> => {
      const proc = spawn(SERVER_BINARY, [], {
        cwd: APP_DIR,
        env,
        stdio: ['ignore', 'pipe', 'pipe'],
      });
      const stderr: string[] = [];
      proc.stderr?.on('data', (chunk: Buffer) => {
        stderr.push(chunk.toString());
        process.stderr.write(chunk);
      });
      const exited = new Promise<number | null>((r) => proc.once('exit', r));
      try {
        await Promise.race([
          waitForHealth(`${baseURL}/health`, 20_000),
          exited.then((code) => {
            throw new Error(
              `kira-space-server-test exited (code ${code}) before /health:\n${stderr.join('')}`,
            );
          }),
        ]);
      } catch (err) {
        proc.kill('SIGKILL');
        throw err;
      }
      return { proc, exited };
    };
    let server = await start();

    const page = await browser.newPage();
    page.on('console', (msg) => {
      if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    await page.setViewportSize({ width: 1440, height: 960 });
    await page.goto(`${baseURL}/`);
    await page.waitForSelector('[data-testid="status-bar"]');
    await selectMode(page, 'studio');

    await use({
      window: page,
      baseURL,
      call: (service, method, args) => bound(baseURL, BRIDGE_PKG, service, method, args),
      relaunch: async () => {
        server.proc.kill('SIGKILL');
        await server.exited;
        server = await start();
        await page.goto(`${baseURL}/`);
        await page.waitForSelector('[data-testid="status-bar"]');
        await selectMode(page, 'studio');
      },
    });

    await page.close();
    // SIGKILL, not a graceful shutdown (§8) — this tier does not test lifecycle/quit handshakes
    // (that's explicitly out of scope, §4), and a process left to its own OnShutdown hooks between
    // tests is exactly the kind of teardown flakiness this fixture doesn't need to own.
    server.proc.kill('SIGKILL');
    await server.exited;
  },
});

export { expect } from '@playwright/test';
