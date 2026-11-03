import type { Transport } from '@kira/git-ipc';
import { logError } from '@workbench/bridge/log';

/**
 * The per-connection `repo.open` hold `blameAnnotation.ts`'s `blame.line` calls need, so
 * `repo/git/transport.ts`'s own `disposeGitTransport` can clear an entry on its own dispose path:
 * `repo/` must not import `views/` directly (the restricted-imports rule), but both may import
 * `state/`.
 *
 * `blame.line` needs this connection to already hold the repo. `repo.open` is idempotent
 * per (connection, repoId) and git-ui never calls `repo.close`, so the hold lives as long as the
 * transport — one memoised promise per gitRepoId, module-level so two open file tabs on the same
 * repository share it. Mirrors `reviewDecorations.ts`'s own `baseMemo`: only a
 * SUCCESSFUL resolution stays cached, so a transient failure can retry on the next attach rather
 * than replay the same rejection forever.
 */
const repoOpenMemo = new Map<string, Promise<void>>();

export function ensureRepoOpen(transport: Transport, gitRepoId: string): Promise<void> {
  let cached = repoOpenMemo.get(gitRepoId);
  if (!cached) {
    cached = transport
      .request('repo.open', { path: gitRepoId }, undefined)
      .then(() => undefined)
      .catch((err: unknown) => {
        repoOpenMemo.delete(gitRepoId);
        logError('repoOpenHold', 'repo.open failed', err);
        throw err;
      });
    repoOpenMemo.set(gitRepoId, cached);
  }
  return cached;
}
