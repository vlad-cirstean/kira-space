import { open, readFile, rm, stat } from 'node:fs/promises';
import { createServer } from 'node:net';
import { resolve } from 'node:path';
import { newUuid } from '@shared/ids';

// Helpers both apps' tests/e2e-real fixtures share: a real `-tags server` Go backend is built under
// one cross-process lock, spawned on a free loopback port and polled until it answers /health.

const ROOT_DIR = resolve(__dirname, '../../../..');
// Serializes the prerequisite-build filesystem writes (frontend/dist, the compiled binary) across
// worker *processes* — each fixture's in-module memo only dedupes within one process. Two workers
// building at once would race on the same output paths.
const LOCK_PATH = resolve(ROOT_DIR, '.e2e-real-build.lock');

// staleLockAgeMs is the age-based backstop (F10, P108 Part 6): a lock this old is reclaimed
// regardless of whether its own owner PID looks alive — generous past this repo's own build time,
// so it only ever fires for a genuinely abandoned lock, never a real in-progress build.
const staleLockAgeMs = 10 * 60 * 1000; // 10 minutes
// acquireLockTimeoutMs is the hard ceiling on how long acquireBuildLock waits overall — a loud,
// specific failure instead of a silent-forever hang on a killed run's leftover lock file.
const acquireLockTimeoutMs = 15 * 60 * 1000; // 15 minutes

/** True when pid does not name a live process on this machine — the PID-liveness half of the
 *  stale-lock check. ESRCH means no such process (dead); EPERM means it exists but isn't ours to
 *  signal (still alive, owned by someone else) — anything else is treated as "can't tell,
 *  assume alive" so this never falsely reclaims a lock a live process still holds. */
function processIsDead(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return false;
  } catch (err) {
    return (err as NodeJS.ErrnoException).code === 'ESRCH';
  }
}

// isLockStale is the reclaim decision: age-based (a lock older than staleLockAgeMs, the
// simplest and most robust signal — a build genuinely running this long would be its own separate
// problem) OR the PID the lock file itself records is no longer alive (a killed run's own lock,
// caught quickly even before the age threshold). Either check returning false means "keep
// waiting", never "this lock is definitely healthy" — a lock that vanished between the EEXIST
// above and this check (a racing worker already reclaimed or released it) reads as not stale
// either, which is correct: there is nothing left here for this call to reclaim. lockPath defaults
// to the real LOCK_PATH; parameterized (like acquireBuildLock below) so a unit test can point this
// at a throwaway file instead of the real cross-process lock.
export async function isLockStale(lockPath: string = LOCK_PATH): Promise<boolean> {
  let mtimeMs: number;
  let pidText: string;
  try {
    [{ mtimeMs }, pidText] = await Promise.all([stat(lockPath), readFile(lockPath, 'utf8')]);
  } catch {
    return false;
  }
  if (Date.now() - mtimeMs > staleLockAgeMs) return true;
  const pid = Number.parseInt(pidText, 10);
  return Number.isInteger(pid) && pid > 0 && processIsDead(pid);
}

export async function acquireBuildLock(lockPath: string = LOCK_PATH): Promise<() => Promise<void>> {
  const startedAt = Date.now();
  for (;;) {
    try {
      const handle = await open(lockPath, 'wx');
      // The owning process's own pid — F10: what isLockStale reads back to decide whether a
      // later caller's lock is abandoned.
      await handle.writeFile(String(process.pid));
      await handle.close();
      return () => rm(lockPath, { force: true });
    } catch (err) {
      if ((err as NodeJS.ErrnoException).code !== 'EEXIST') throw err;
      if (Date.now() - startedAt > acquireLockTimeoutMs) {
        throw new Error(
          `timed out after ${acquireLockTimeoutMs}ms waiting for the e2e-real build lock ` +
            `(${lockPath}) — a previous run may have been killed without cleaning it up; ` +
            'delete that file by hand and retry if so.',
        );
      }
      if (await isLockStale(lockPath)) {
        await rm(lockPath, { force: true });
        continue; // retry acquiring immediately — no sleep, another waiter may win it first.
      }
      await new Promise((r) => setTimeout(r, 200));
    }
  }
}

// Per-test port (D3: proven sufficient for parallel isolation alongside a per-test home) — bind a
// real listener to port 0 to get one the OS guarantees is free, then hand it to the spawned binary
// instead of that listener.
export async function getFreePort(): Promise<number> {
  return new Promise((resolvePort, reject) => {
    const probe = createServer();
    probe.once('error', reject);
    probe.listen(0, '127.0.0.1', () => {
      const address = probe.address();
      const port = typeof address === 'object' && address ? address.port : 0;
      probe.close(() => resolvePort(port));
    });
  });
}

export async function waitForHealth(url: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
    } catch {
      // Not listening yet — keep polling rather than sleeping a fixed guess (§8).
    }
    if (Date.now() > deadline) {
      throw new Error(`server did not answer ${url} within ${timeoutMs}ms`);
    }
    await new Promise((r) => setTimeout(r, 100));
  }
}

export class BoundError extends Error {
  constructor(
    readonly service: string,
    readonly method: string,
    readonly status: number,
    readonly body: string,
  ) {
    super(`${service}.${method} -> ${status}: ${body}`);
  }
}

/**
 * Calls a bound service method on the real server over POST /wails/runtime, the same wire the
 * renderer's generated bindings use. `bridgePkg` is the app's Go bridge package path; `object: 0`
 * is the Wails Call object, `method: 0` its only method.
 */
export async function bound<T>(
  baseURL: string,
  bridgePkg: string,
  service: string,
  method: string,
  args?: unknown,
): Promise<T> {
  const res = await fetch(`${baseURL}/wails/runtime`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', 'x-wails-window-id': '0' },
    body: JSON.stringify({
      object: 0,
      method: 0,
      args: {
        'call-id': newUuid(),
        methodName: `${bridgePkg}.${service}.${method}`,
        args: args === undefined ? [] : [args],
      },
    }),
  });
  const text = await res.text();
  if (!res.ok) throw new BoundError(service, method, res.status, text);
  if (!text) return undefined as T;
  try {
    return JSON.parse(text) as T;
  } catch {
    // Wails returns a bare string result unquoted.
    return text as T;
  }
}

/** What `openMainWindow` needs from the e2e-real `kira` fixture (both apps' fixtures satisfy it). */
interface MainWindowApp {
  baseURL: string;
  window: {
    goto(url: string): Promise<unknown>;
    waitForSelector(selector: string): Promise<unknown>;
  };
  call<T>(service: string, method: string, args?: unknown): Promise<T>;
}

/**
 * Reloads the fixture page as the app's main window. The prompt router (P246) shows a popup only
 * in the window it targets, and a plain browser page has a random key unless it asks for this one.
 */
export async function openMainWindow(kira: MainWindowApp): Promise<string> {
  const main = await kira.call<string>('PromptsService', 'MainWindow');
  if (!main) throw new Error('PromptsService.MainWindow returned no window key');
  await kira.window.goto(`${kira.baseURL}/?window=${encodeURIComponent(main)}`);
  await kira.window.waitForSelector('[data-testid="status-bar"]');
  return main;
}
