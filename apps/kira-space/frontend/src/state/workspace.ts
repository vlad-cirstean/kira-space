import { t } from '@theme/i18n';
import { usePersistedState } from '@workbench/behaviour/persisted';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { windowKey } from '@workbench/lib/window';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { control } from '../bridge/control';
import { disposeGitTransport } from '../repo/git/transport';
import { useFileTreeStore } from '../repo/state/fileTree';
import { useRepoSearchStore } from '../repo/state/search';
import { useModeStore } from './mode';
import { useOpenedFilesWindowStore } from './openedFilesWindow';
import { ensureWorkspaceShell } from './repoTabs';
import { closeWorkspaceTabs } from './tabs';

// Kira Studio's own state/workspace.ts (Pinia store), adapted for an app with exactly
// one workspace kind — a repository — instead of layering a repo key inside one of several
// AppModes. packages/shared/domain/workspace.ts's WorkspaceKey (`AppMode | \`repo:${string}\``) had
// no equivalent to port: here a WorkspaceKey **is** a bare code_repos id, so the prefix/parse pair
// it defined (repoWorkspaceKey/repoIdOfWorkspace) collapses to identity — kept below only so the
// ported files that already call them (written against the old prefixed type) compile unchanged.
export type WorkspaceKey = string;

// The one non-repo slot: an unscoped terminal tab (state/terminalTabs.ts's own default, no repo in
// scope) and the moment before any repository has ever been opened. Never a real code_repos id
// (a UUID never matches this literal), so it can never collide with one.
export const GENERAL_WORKSPACE = '__general__';

// The files window's one tab partition: repo and loose file tabs share a strip. Never a real
// code_repos id, same reasoning as GENERAL_WORKSPACE.
export const FILES_WORKSPACE = '__files__';

/** Identity — every WorkspaceKey here already is a repo id (or GENERAL_WORKSPACE). Kept as a named
 *  function so call sites written against the old `repo:${id}`-prefixed type need no edit. */
export function repoWorkspaceKey(repoId: string): WorkspaceKey {
  return repoId;
}

/** Identity, GENERAL_WORKSPACE aside — mirrors the old parser's `null` for "not a repo workspace". */
export function repoIdOfWorkspace(key: WorkspaceKey): string | null {
  return key === GENERAL_WORKSPACE ? null : key;
}

/** `repoIdOfWorkspace` applied to a tab's own `workspaceId` — every repo view's own mount() rebuilt
 *  this exact `tab.workspaceId ? repoIdOfWorkspace(tab.workspaceId as WorkspaceKey) : null` ternary
 *  (P115 H9). `null` for a tab with no workspace at all, same as `repoIdOfWorkspace`. */
export function repoIdOfTab(tab: { workspaceId: string | null }): string | null {
  return tab.workspaceId ? repoIdOfWorkspace(tab.workspaceId as WorkspaceKey) : null;
}

/** The message every repo view's own "no repository" error/empty state shows, hand-copied ×4
 *  (P115 H9) — moved beside `repoIdOfTab` since it always accompanies a null result from it. */
export const noRepositoryMessage = (): string => t('space.repo.view.noRepoTab.error');

// The active workspace, the Git panel's repo switcher (`openRepos`), and which repo was last
// active (`lastRepoKey`, persisted per window so a relaunch reopens on it) — Kira Studio's own
// useWorkspaceStore (state/workspace.ts).
// This app now has its own mode dimension too (state/mode.ts), but `state.active` stays
// git-only — a module key (`terminal`/`ade`) never reaches it, `repoIdOfWorkspace`, `GitPanel`, or
// `lastRepoKey`. `visibleWorkspace()` below is the one function that reads across both stores.
export const useWorkspaceStore = defineStore('workspace', () => {
  const state = reactive({
    active: GENERAL_WORKSPACE as WorkspaceKey,
    openRepos: [] as string[], // code_repos.id, in switcher order
  });
  const lastRepoKey = usePersistedState<WorkspaceKey | null>(
    `kira.space.lastRepo.${windowKey}`,
    null,
  );

  function activateWorkspace(key: WorkspaceKey): void {
    state.active = key;
    if (key !== GENERAL_WORKSPACE) lastRepoKey.value = key;
  }

  // Opens repoId's own workspace — adds it to the switcher (a no-op if already open), ensures its
  // pinned graph tab exists, starts its index (fire-and-forget: a failed index start must never
  // block opening a workspace whose tree and viewer work regardless), then activates it.
  //
  // Also switches the mode to `git` — opening a repo from any path (a future
  // command-palette entry, say) must show it, not leave it open behind whichever other module is
  // active. `activateWorkspace` itself does NOT do this (below): its own callers are either
  // already git-mode-only UI (GitPanel's row click, closeRepoWorkspace's own fallback) or this
  // app's boot-time fall-forward onto the first restored repo (main.ts) — which must honour
  // whatever mode `windowsEnsure` persisted, not silently override it back to `git`.
  function openRepoWorkspace(repoId: string): void {
    if (!state.openRepos.includes(repoId)) {
      state.openRepos = [...state.openRepos, repoId];
    }
    ensureWorkspaceShell(repoId);
    // Bookkeeping for the next launch; the workspace state here is already authoritative.
    fireAndForget(control.codeWorkspaceOpenWorkspace(repoId), 'workspace');
    activateWorkspace(repoId);
    useModeStore().setMode('git');
  }

  // A review window shows one repo's diff tabs and nothing else: no pinned graph tab, no mode change.
  function openReviewWorkspace(repoId: string): void {
    if (!state.openRepos.includes(repoId)) {
      state.openRepos = [...state.openRepos, repoId];
    }
    // Bookkeeping for the next launch; the workspace state here is already authoritative.
    fireAndForget(control.codeWorkspaceOpenWorkspace(repoId), 'workspace');
    activateWorkspace(repoId);
  }

  // Closes every one of repoId's own tabs (its pinned graph tab included — closeWorkspaceTabs is
  // the one path that bypasses the pin guard, tearing down the whole workspace being a different
  // act from closing one of its tabs), drops it from the switcher, stops its index/watcher, and
  // drops its file-tree/search caches (previously wired only to removeCodeRepo's own explicit
  // calls, state/coderepos.ts — closing a workspace without deleting the repo never freed either
  // otherwise). dropQuickOpen is NOT called directly here — repo/state/quickOpen.ts imports this
  // module and watches `openRepos` itself, evicting on the same transition, to keep the import
  // direction one-way.
  function closeRepoWorkspace(repoId: string): void {
    closeWorkspaceTabs(repoId);
    state.openRepos = state.openRepos.filter((id) => id !== repoId);
    // Bookkeeping for the next launch; the workspace state here is already authoritative.
    fireAndForget(control.codeWorkspaceCloseWorkspace(repoId), 'workspace');
    // The pinned graph tab's own transport is cached per repo workspace, independent of the tab's
    // own mount/unmount (a tab switch alone must not tear it down) — closing the workspace itself
    // is the one event that actually ends it. A no-op when the graph tab was never mounted.
    disposeGitTransport(repoId);
    useFileTreeStore().dropRepoTree(repoId);
    useRepoSearchStore().dropRepoSearch(repoId);
    if (lastRepoKey.value === repoId) lastRepoKey.value = null;
    if (state.active === repoId) activateWorkspace(GENERAL_WORKSPACE);
  }

  return {
    ...toRefs(state),
    lastRepoKey,
    activateWorkspace,
    openRepoWorkspace,
    openReviewWorkspace,
    closeRepoWorkspace,
  };
});

// Which workspace's own tabs the tab strip currently shows — the active repo (or
// GENERAL_WORKSPACE) while the Git module is active, else the active module's own id. A module
// tab (a Terminal-module terminal) always carries that same id as its `workspaceId`
// (state/terminalTabs.ts's module opener), and `ade` opens no tab at all, so this is the one
// function `host.ts`'s `activeWorkspace` and `state/tabs.ts`'s tab-stepping actions need to read
// across both the workspace and mode stores.
export function visibleWorkspace(): WorkspaceKey {
  if (useOpenedFilesWindowStore().active) return FILES_WORKSPACE;
  const modeStore = useModeStore();
  if (modeStore.active === 'git') return useWorkspaceStore().active;
  return modeStore.active;
}
