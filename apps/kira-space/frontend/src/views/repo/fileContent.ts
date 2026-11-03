import type { FileStat } from '@shared/domain/repo';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
// P94 pass 3 §4.3: RepoFileView.vue's mount() read its file two different ways (worktree vs a
// revision-pinned tab) with near-duplicate try/catch/error-message plumbing around each — lifted
// out as one pure(ish) async call so mount() itself only branches on the result.

import { control } from '../../bridge/control';
import { gitCall } from '../../repo/git/gitCall';
import { gitRepoIdFor } from '../../repo/git/hostHandlers';
import { withGitRepoHold } from '../../repo/git/repoHolds';

/** A full commit id: the backend pins a branch or tag to one, so a shorter hex spelling may be a
 *  moving ref and never counts as fixed. */
export const FULL_COMMIT_ID = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/i;

export type FileContentResult =
  | { kind: 'found'; text: string; stat?: FileStat }
  | { kind: 'missing' | 'binary' | 'tooLarge' }
  | { kind: 'error'; message: string };

/** Reads `path` from the worktree (`rev === null`) or, for a revision-pinned tab, via
 *  `file.read` (the same request RepoDiffView.vue's own commit-diff sides
 *  use, never the worktree file). Never throws — a read failure or a missing git repo both come
 *  back as `{ kind: 'error' }`, `mount()`'s own contract for every failure mode it renders. */
export async function loadFileContent(
  repoId: string,
  path: string,
  rev: string | null,
  /** The worktree text a previous mount read, with the stat it had. When the file still matches,
   *  the backend sends no content and this text is the result. */
  known?: { stat: FileStat; text: () => string },
): Promise<FileContentResult> {
  if (rev === null) {
    try {
      const content = await control.codeWorkspaceReadFile(repoId, path, known?.stat);
      if (content.kind === 'unchanged') {
        return known
          ? { kind: 'found', text: known.text(), stat: known.stat }
          : loadFileContent(repoId, path, rev);
      }
      return content.kind === 'found'
        ? { kind: 'found', text: content.text, stat: content.stat }
        : { kind: content.kind };
    } catch (err) {
      return { kind: 'error', message: errorText(err) };
    }
  }

  const gitRepoId = gitRepoIdFor(repoId);
  if (!gitRepoId) return { kind: 'error', message: t('space.repo.view.repoNotOpen.error') };
  try {
    const result = await withGitRepoHold(gitRepoId, () =>
      gitCall('file.read', { repoId: gitRepoId, rev, path }),
    );
    return result.kind === 'found'
      ? { kind: 'found', text: result.content }
      : { kind: result.kind };
  } catch (err) {
    return { kind: 'error', message: errorText(err) };
  }
}
