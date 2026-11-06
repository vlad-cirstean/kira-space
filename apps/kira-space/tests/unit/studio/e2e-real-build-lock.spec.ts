import { describe, expect, test } from 'bun:test';
import { mkdtemp, readFile, rm, utimes, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { acquireBuildLock, isLockStale } from '@workbench/testing/e2eReal';

// acquireBuildLock used to loop forever on EEXIST with no stale-lock recovery
// — a killed test run left `.e2e-real-build.lock` behind, and the next run hung silently waiting
// on a lock nobody holds. These exercise isLockStale/acquireBuildLock directly against a throwaway
// path (both take an optional lockPath, defaulting to the real cross-process lock file) rather
// than the real e2e-real fixture, which needs a full Go build and a real browser to reach at all.

async function withTempLockDir(fn: (lockPath: string) => Promise<void>): Promise<void> {
  const dir = await mkdtemp(join(tmpdir(), 'e2e-real-lock-'));
  try {
    await fn(join(dir, '.e2e-real-build.lock'));
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
}

describe('testing/e2eReal.ts — build lock stale recovery (F10)', () => {
  test('no lock file at all is never stale (nothing to reclaim)', async () => {
    await withTempLockDir(async (lockPath) => {
      expect(await isLockStale(lockPath)).toBe(false);
    });
  });

  test('a fresh lock owned by this (definitely alive) process is not stale', async () => {
    await withTempLockDir(async (lockPath) => {
      await writeFile(lockPath, String(process.pid));
      expect(await isLockStale(lockPath)).toBe(false);
    });
  });

  test('a lock naming a dead pid is stale, even if fresh', async () => {
    await withTempLockDir(async (lockPath) => {
      // PID 1 is init/PID namespace root and never a plain child this test could have spawned;
      // a genuinely-unused, very high PID is what simulates "dead" portably — use a
      // fixed, implausibly large PID instead, which process.kill(pid, 0) reports ESRCH for on any
      // real machine.
      await writeFile(lockPath, '999999999');
      expect(await isLockStale(lockPath)).toBe(true);
    });
  });

  test('an old lock is stale regardless of its own pid looking alive', async () => {
    await withTempLockDir(async (lockPath) => {
      await writeFile(lockPath, String(process.pid));
      const old = new Date(Date.now() - 60 * 60 * 1000); // 1 hour old, past staleLockAgeMs.
      await utimes(lockPath, old, old);
      expect(await isLockStale(lockPath)).toBe(true);
    });
  });

  test('acquireBuildLock reclaims a stale lock instead of hanging forever', async () => {
    await withTempLockDir(async (lockPath) => {
      await writeFile(lockPath, '999999999'); // a dead owner — immediately reclaimable.
      const release = await acquireBuildLock(lockPath);
      // The reclaimed lock file must now be owned by this process, not the dead pid it replaced.
      expect(await readFile(lockPath, 'utf8')).toBe(String(process.pid));
      await release();
      expect(await isLockStale(lockPath)).toBe(false); // gone entirely, not just no longer stale.
    });
  });
});
