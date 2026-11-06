import { spawn } from 'node:child_process';
import { mkdir, mkdtemp, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test as base, type Page } from '@playwright/test';
import { getFreePort, waitForHealth } from '@workbench/testing/e2eReal';
import { bound } from './support/bound';
import { APP_DIR, buildPrerequisites, FAKE_AGENT_BINARY, SERVER_BINARY } from './support/build';
import { type FlowServers, startFlowServers } from './support/flowServers';
import { stubNativeDialogs } from './support/routes';

// Kira Space's real-backend tier: a `go build -tags server` binary on real SQLite, git, PTY and
// stream, reached by a plain browser page. The claude and gh CLIs are the fake agent
// (internal/flowtest/fakeagent); dialogs, keep-awake and the window manager have no server-build
// counterpart. A database spec also gets the flow harness's HTTP, HTTPS and gRPC servers and can open
// a Studio module on boot (`startMode`).

export interface KiraSpaceApp {
  window: Page;
  baseURL: string;
  /** Per-test scratch root holding home, state and the repos a test creates. */
  root: string;
  /** Directory repos for this test live under. */
  work: string;
  /** Directory the fake agent records its calls into (KIRA_FAKE_DIR). */
  fakeDir: string;
  /** Calls a bound service method on the real server. */
  call<T>(service: string, method: string, args?: unknown): Promise<T>;
  /** Reloads the page, re-running boot against current server state. */
  reload(): Promise<void>;
  /** Kills the server (SIGKILL) and starts it again on the same home and port, then reloads the page. */
  relaunch(): Promise<void>;
}

interface KiraSpaceFixtures {
  consoleErrors: string[];
  scenario: object | undefined;
  /** Extra environment for the spawned server (merged last): `test.use({ serverEnv: { DOCKER_HOST } })`. */
  serverEnv: Record<string, string>;
  /** Mode tab opened after boot and after every relaunch; unset keeps the app's first mode. */
  startMode: string | undefined;
  /** The flow harness's real HTTP, HTTPS and gRPC servers, one set per test so recorded requests never leak between tests. */
  flowServers: FlowServers;
  kira: KiraSpaceApp;
}

export const test = base.extend<KiraSpaceFixtures, { prerequisites: true }>({
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

  // biome-ignore lint/correctness/noEmptyPattern: Playwright requires a literal destructuring pattern here, even with no fixture deps.
  consoleErrors: async ({}, use) => {
    await use([]);
  },

  scenario: [undefined, { option: true }],

  serverEnv: [{}, { option: true }],

  startMode: [undefined, { option: true }],

  flowServers: [
    // biome-ignore lint/correctness/noEmptyPattern: Playwright requires a literal destructuring pattern here, even with no fixture deps.
    async ({}, use) => {
      const { servers, stop } = await startFlowServers();
      await use(servers);
      await stop();
    },
    { scope: 'test' },
  ],

  kira: async ({ browser, consoleErrors, scenario, serverEnv, startMode }, use) => {
    // Short root: unix-socket paths under it must stay below the sun_path limit.
    const root = await mkdtemp(join(tmpdir(), 'ksr-'));
    const home = join(root, 'home');
    const work = join(root, 'work');
    const binDir = join(root, 'bin');
    const fakeDir = join(root, 'fake');
    await Promise.all([home, work, binDir, fakeDir].map((d) => mkdir(d, { recursive: true })));
    await writeFile(
      join(home, '.gitconfig'),
      '[user]\n\tname = Flow Test\n\temail = flow@example.test\n[init]\n\tdefaultBranch = main\n',
    );
    // Run shells are login shells that reorder PATH; their profile keeps the fake claude first.
    const profile = `export PATH=${JSON.stringify(binDir)}:"$PATH"\n`;
    await Promise.all([
      ...['.bash_profile', '.zprofile', '.profile'].map((f) => writeFile(join(home, f), profile)),
      symlink(FAKE_AGENT_BINARY, join(binDir, 'claude')),
      symlink(FAKE_AGENT_BINARY, join(binDir, 'gh')),
    ]);
    const scenarioPath = join(root, 'scenario.json');
    await writeFile(scenarioPath, JSON.stringify(scenario ?? {}));

    const port = await getFreePort();
    const env: NodeJS.ProcessEnv = {
      ...process.env,
      HOME: home,
      KIRA_SPACE_HOME: join(root, 'space'),
      KIRA_MEMORY_HOME: join(root, 'memory'),
      GIT_CONFIG_NOSYSTEM: '1',
      GIT_CONFIG_GLOBAL: join(home, '.gitconfig'),
      PATH: `${binDir}:${process.env.PATH ?? ''}`,
      TZ: 'UTC',
      KIRA_INSECURE_SECRETS: '1',
      KIRA_FAKE_DIR: fakeDir,
      KIRA_FAKE_SCEN: scenarioPath,
      // Loopback only: /wails/runtime has no authentication, so a server-mode binary exposes the
      // whole bound surface to anyone who can reach the port.
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
      proc.stderr?.on('data', (chunk: Buffer) => stderr.push(chunk.toString()));
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

    let server: Server;
    try {
      server = await start();
    } catch (err) {
      await rm(root, { recursive: true, force: true });
      throw err;
    }

    const page = await browser.newPage();
    page.on('console', (msg) => {
      if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    await stubNativeDialogs(page);
    await page.setViewportSize({ width: 1440, height: 960 });
    const url = `${baseURL}/?window=${crypto.randomUUID()}`;
    const boot = async (): Promise<void> => {
      await page.goto(url);
      await page.waitForSelector('[data-testid="status-bar"]');
      if (startMode)
        await page.locator(`[data-testid="mode-tab"][data-mode="${startMode}"]`).click();
    };
    await boot();

    await use({
      window: page,
      baseURL,
      root,
      work,
      fakeDir,
      call: (service, method, args) => bound(baseURL, service, method, args),
      reload: boot,
      relaunch: async () => {
        server.proc.kill('SIGKILL');
        await server.exited;
        server = await start();
        await boot();
      },
    });

    await page.close();
    // SIGKILL: this tier does not test quit handshakes.
    server.proc.kill('SIGKILL');
    await server.exited;
    await rm(root, { recursive: true, force: true });
  },
});

export { expect } from '@playwright/test';
