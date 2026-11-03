import { worktreeLabel } from '@kira/git-core';
import type { WorktreeEntry } from '@kira/git-ipc';
import { errorText } from '@workbench/bridge/codedError';
import { queryClient } from '@workbench/state/queryClient';
import { defineStore } from 'pinia';
import { computed, shallowReactive, watch } from 'vue';
import { useCodeReposStore } from '../../state/coderepos';
import { useWorkspaceStore } from '../../state/workspace';
import { onGitRepoEvent } from '../git/gitCall';
import { type GitRepoHold, holdGitRepo } from '../git/repoHolds';
import { createPerRepoState } from './perRepo.ts';
import { useRepoLinksStore } from './repoLinks';
import { type ExpandedWorktreeRow, worktreesQueryKey } from './worktreeQueries';

// P82 §6: per-repo worktree disclosure state, session-scoped and module-level — same shape and
// reasoning as fileTree.ts's byRepo/search.ts's repoSearchView. Not persisted (§0/§10): a future
// phase that wants that needs a new settings key or DB column. The worktree lists themselves are
// TanStack Query state (`worktreeQueries.ts`).
interface RepoWorktreeState {
  expanded: boolean;
  switchError: string | null;
}

export const useWorktreesStore = defineStore('worktrees', () => {
  const codeReposStore = useCodeReposStore();
  const workspaceStore = useWorkspaceStore();
  const repoLinksStore = useRepoLinksStore();

  const perRepo = createPerRepoState(
    (): RepoWorktreeState => ({ expanded: false, switchError: null }),
  );
  const byRepo = perRepo.all;
  const { stateFor } = perRepo;

  // One window-level repo hold plus its repo.changed subscription per expanded row, kept for exactly
  // as long as the row is expanded.
  const holds = shallowReactive(
    new Map<string, { gitRepoId: string; hold: GitRepoHold; off: () => void }>(),
  );

  const expandedRows = computed<readonly ExpandedWorktreeRow[]>(() =>
    [...holds].map(([codeRepoId, { gitRepoId, hold }]) => ({
      codeRepoId,
      gitRepoId,
      ready: () => hold.ready,
    })),
  );

  function isWorktreesExpanded(codeRepoId: string): boolean {
    return byRepo.get(codeRepoId)?.expanded ?? false;
  }

  function switchError(codeRepoId: string): string | null {
    return byRepo.get(codeRepoId)?.switchError ?? null;
  }

  /** Releases this row's hold, subscription and cached list: a re-expand fetches afresh. */
  function release(codeRepoId: string): void {
    const held = holds.get(codeRepoId);
    if (!held) return;
    holds.delete(codeRepoId);
    held.off();
    held.hold.release();
    queryClient.removeQueries({ queryKey: worktreesQueryKey(held.gitRepoId) });
  }

  function toggleRepoWorktrees(codeRepoId: string): void {
    const state = stateFor(codeRepoId);
    if (state.expanded) {
      collapseRepoWorktrees(codeRepoId);
      return;
    }
    state.expanded = true;
    state.switchError = null;
    acquire(codeRepoId);
  }

  function acquire(codeRepoId: string): void {
    const record = codeReposStore.codeRepoRecord(codeRepoId);
    if (!record) return;
    const gitRepoId = record.repoId;
    // §6.4: live refresh while expanded — a worktree created from the graph's own dialog appears
    // without a collapse/expand round trip.
    const off = onGitRepoEvent('repo.changed', gitRepoId, (event) => {
      if (event.kind !== 'refsChanged') return;
      void queryClient.invalidateQueries({ queryKey: worktreesQueryKey(gitRepoId) });
    });
    holds.set(codeRepoId, { gitRepoId, hold: holdGitRepo(gitRepoId), off });
  }

  function collapseRepoWorktrees(codeRepoId: string): void {
    release(codeRepoId);
    perRepo.drop(codeRepoId);
  }

  /** §7: "switch to this worktree" — a worktree row's click. Clicking the entry that *is* this row's
   *  own repository needs no special case: openRepoAtPath's codeRepoRecordForPath finds that row and
   *  openRepoWorkspace opens-or-activates it, same as onRowClick.
   *
   *  P84 §4.5: takes the entry, not the bare path, so it can write the click-time parent hint —
   *  between the import resolving and the next batched RepoWorktreeLinks answering, this is what
   *  keeps the freshly-imported row from flashing at the top level. */
  async function switchToWorktree(codeRepoId: string, wt: WorktreeEntry): Promise<void> {
    const state = stateFor(codeRepoId);
    state.switchError = null;
    try {
      await codeReposStore.openRepoAtPath(wt.path);
      if (wt.isMain) return; // clicking the main worktree imports the *anchor*, which has no parent
      const record = codeReposStore.codeRepoRecordForPath(wt.path);
      if (record) {
        repoLinksStore.noteWorktreeLink(
          record.id,
          repoLinksStore.worktreeParentId(codeRepoId) || codeRepoId,
        );
      }
    } catch (err) {
      state.switchError = errorText(err);
    }
  }

  // Eviction, without inverting an import — state/workspace.ts must not import this module
  // (it already imports fileTree.ts/search.ts, and this module imports it), so both closures are
  // watched here instead, the pattern quickOpen.ts's own equivalent watches established for
  // exactly this.

  // A closed workspace collapses its row. openRepos is always reassigned wholesale, so a plain watch
  // sees the pre-close membership as `previous`.
  watch(
    () => workspaceStore.openRepos,
    (openRepos, previous) => {
      if (!previous) return;
      for (const id of previous) if (!openRepos.includes(id)) collapseRepoWorktrees(id);
    },
  );

  // removeCodeRepo reassigns `records` wholesale; a removed repository must not keep an entry (or
  // a hold) here. It also calls closeRepoWorkspace, but only the open case is covered by the
  // watch above.
  watch(
    () => codeReposStore.records,
    (records) => {
      const live = new Set(records.map((r) => r.id));
      for (const id of [...byRepo.keys()]) if (!live.has(id)) collapseRepoWorktrees(id);
    },
  );

  return {
    isWorktreesExpanded,
    expandedRows,
    switchError,
    toggleRepoWorktrees,
    collapseRepoWorktrees,
    switchToWorktree,
  };
});

export { worktreeLabel };
