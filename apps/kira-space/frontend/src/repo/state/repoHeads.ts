import { shortSha } from '@kira/git-core';
import type { HeadState } from '@kira/git-ipc';
import { t } from '@theme/i18n';
import { logWarn } from '@workbench/bridge/log';
import { defineStore } from 'pinia';
import { reactive, watch } from 'vue';
import { control } from '../../bridge/control';
import { useCodeReposStore } from '../../state/coderepos';
import { useWorkspaceStore } from '../../state/workspace';
import { onGitRepoEvent } from '../git/gitCall';
import { type GitRepoHold, holdGitRepo } from '../git/repoHolds';
import { useRepoVisibilityStore } from './repoVisibility';

export const useRepoHeadsStore = defineStore('repoHeads', () => {
  const codeReposStore = useCodeReposStore();
  const workspaceStore = useWorkspaceStore();
  const visibility = useRepoVisibilityStore();

  // Every repo row's checked-out branch, session-scoped and module-level — same
  // shape as worktrees.ts's byRepo/search.ts's repoSearchView. `null` means "known, and there is
  // none" (a bare repository, or a row RepoHeads could not read); an absent key means "not fetched
  // yet", so a collapsed row renders no label rather than a stale one.
  const byRepoId = reactive(new Map<string, HeadState | null>());

  /** git-ui's `pickerModel.ts` own helper, mirrored rather than imported — worktreeLabel's own
   *  comment (worktrees.ts:133) names the same reason: not on `@kira/git-ui`'s exports map. Kept in
   *  step with worktreeLabel by hand so a collapsed row's branch and its expanded children's read
   *  the same way. '' when unknown, so the template's `v-if` renders nothing rather than an empty
   *  pill. */
  function repoHeadLabel(codeRepoId: string): string {
    const head = byRepoId.get(codeRepoId);
    if (!head) return '';
    switch (head.kind) {
      case 'branch':
        return head.name;
      case 'detached':
        return t('space.repo.heads.detached.label', { sha: shortSha(head.sha) });
      case 'unborn':
        return head.name; // the branch HEAD points at, not yet committed to
    }
  }

  let warned = false;

  /** One batched `RepoHeads` call — every listed row when `ids` is omitted (mount, a records
   *  change; hidden repos stay out so they cost no git spawn), one row for a `refsChanged` event
   *  (a one-row `RepoHeads` call over the same batched method). No coalescing:
   *  each trigger fires at most once per its own event, nothing bursts this. */
  async function refreshRepoHeads(ids?: string[]): Promise<void> {
    const target = ids ?? visibility.listedIds;
    if (target.length === 0) return;
    try {
      const rows = await control.codeWorkspaceRepoHeads(target);
      for (const row of rows) byRepoId.set(row.id, row.head);
    } catch (err) {
      // Fired without awaiting from watchers and events; keep the labels already shown.
      if (!warned) logWarn('repoHeads', 'refresh failed', err);
      warned = true;
    }
  }

  // §12.3 trigger 2: an import, a remove, or a P82 worktree switch changes the row set — a new row
  // must not render headless, and a removed one must not linger in the map.
  watch(
    () => [codeReposStore.records, visibility.showHidden] as const,
    ([records]) => {
      const live = new Set(records.map((r) => r.id));
      for (const id of [...byRepoId.keys()]) if (!live.has(id)) byRepoId.delete(id);
      void refreshRepoHeads();
    },
  );

  // One window-level repo hold plus its repo.changed subscription per open repo workspace; a
  // checkout, which is what changes a HEAD, happens in the workspace's own graph. `immediate: true`
  // so a repo workspace already open when this store first activates is covered too.
  const holds = new Map<string, { hold: GitRepoHold; off: () => void }>();

  function track(id: string, gitRepoId: string): void {
    const off = onGitRepoEvent('repo.changed', gitRepoId, (event) => {
      if (event.kind === 'refsChanged') void refreshRepoHeads([id]);
    });
    holds.set(id, { hold: holdGitRepo(gitRepoId), off });
  }

  watch(
    () => [workspaceStore.openRepos, codeReposStore.records] as const,
    ([openRepos]) => {
      for (const id of openRepos) {
        const record = codeReposStore.codeRepoRecord(id);
        if (record && !holds.has(id)) track(id, record.repoId);
      }
      for (const [id, held] of [...holds]) {
        if (openRepos.includes(id)) continue;
        holds.delete(id);
        held.off();
        held.hold.release();
      }
    },
    { immediate: true },
  );

  return { repoHeadLabel, refreshRepoHeads };
});
