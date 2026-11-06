import { execFileSync, spawn } from 'node:child_process';
import { mkdir, mkdtemp, rm, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { test as base, type Page } from '@playwright/test';
import { acquireBuildLock, bound, getFreePort, waitForHealth } from '@workbench/testing/e2eReal';
import { type FlowServers, startFlowServers } from './support/flowServers';

// The tests/e2e-real/ counterpart to tests/e2e/fixtures.ts's `_electron.launch()`.
// There is no Electron process and no native window: `relaunch`-equivalent
// setup here means "build (if needed) and spawn a `go build -tags server` binary, wait for it to
// answer /health, then open a plain browser page against it" — a real Go backend, a real embedded
// engine, a real database adapter, reached over plain HTTP/WebSocket.

const ROOT_DIR = resolve(__dirname, '../../../../..');
const APP_DIR = resolve(ROOT_DIR, 'apps/kira-space');
const SERVER_BINARY = resolve(APP_DIR, 'bin/kira-server-test');
const FAKE_AGENT_BINARY = resolve(APP_DIR, 'bin/fakeclaude');
const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge';

function goBinDir(): string {
  return `${execFileSync('go', ['env', 'GOPATH'], { encoding: 'utf8' }).trim()}/bin`;
}

// `scripts/setup.sh` installs `wails3` via `go install` and expects it on PATH afterward (its own
// shell-profile note) — set explicitly here so this fixture never depends on the invoking shell
// having done that.
function envWithGoBin(): NodeJS.ProcessEnv {
  const extra = goBinDir();
  const path = process.env.PATH ?? '';
  return { ...process.env, PATH: path.includes(extra) ? path : `${path}:${extra}` };
}

// Build prerequisites, all idempotent: `scripts/setup.sh` (pinned
// wails3, generated bindings — no vendored Node runtime or bundled engine to check for)
// plus `bun run build:test:studio` (apps/kira-space/frontend/dist, which main.go's `//go:embed
// all:frontend/dist` picks up — the hooks-enabled build, same as test:ui/test:ipc:fe),
// then the one step that script doesn't do — `go build -tags server`. Memoized per worker process
// so a spec file with multiple tests builds once, not once per test.
let prerequisitesReady: Promise<void> | undefined;

function buildPrerequisites(): Promise<void> {
  prerequisitesReady ??= (async () => {
    const release = await acquireBuildLock();
    try {
      const env = envWithGoBin();
      execFileSync('sh', ['scripts/setup.sh'], { cwd: ROOT_DIR, env, stdio: 'inherit' });
      // `build:wails` (a separate `vite.wails.config.ts`) was folded into the main `vite build`
      // once P57 removed Electron — this repo's own `vite.config.ts` already outputs straight to
      // `apps/kira-space/frontend/dist`, main.go's `//go:embed` target.
      execFileSync('bun', ['run', 'build:test:studio'], { cwd: ROOT_DIR, env, stdio: 'inherit' });
      await mkdir(resolve(APP_DIR, 'bin'), { recursive: true });
      execFileSync('go', ['build', '-tags', 'server', '-o', SERVER_BINARY, '.'], {
        cwd: APP_DIR,
        env,
        stdio: 'inherit',
      });
      execFileSync(
        'go',
        ['build', '-o', FAKE_AGENT_BINARY, '../../../internal/flowtest/fakeagent/cmd/fakeclaude'],
        { cwd: APP_DIR, env, stdio: 'inherit' },
      );
    } finally {
      await release();
    }
  })();
  return prerequisitesReady;
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
      throw new Error(`KIRA_HOME fixture "${kiraHome}" is not under the OS tmpdir`);
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
              `kira-server-test exited (code ${code}) before /health:\n${stderr.join('')}`,
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
