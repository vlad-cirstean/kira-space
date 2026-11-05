import { newUuid } from '@shared/ids';
import { createTabsStore } from '@workbench/state/createTabsStore';
import { control } from '../bridge/control';
import { useOpenedFilesWindowStore } from './openedFilesWindow';
import {
  defaultRepoGraphTabState,
  type LocalFileTabState,
  type RepoFileTabState,
  type RepoGraphTabState,
  type TabRecord,
} from './tabDomain';
import { TAB_KINDS } from './tabKinds';
import {
  FILES_WORKSPACE,
  GENERAL_WORKSPACE,
  useWorkspaceStore,
  visibleWorkspace,
  type WorkspaceKey,
} from './workspace';

// The shared skeleton (persistableTabs/saveIfChanged/openTab/closeTab/…, see that file's own
// header) now lives in packages/workbench/src/state/createTabsStore.ts. This file
// is what's left: the workspace-key mapping, this app's own persistence rule (a terminal tab is
// never saved — the factory's own default, so no `persistable` override is needed), the
// post-hydrate `openRepos` derivation, and the extras no other app has
// (`createPinnedRepoGraphTab`, `closeWorkspaceTabs`, the two repo-kind patchers).

// A tab's own workspace: its `workspaceId` when a repo (or a scoped terminal) set one, else the
// one shared slot for a fully-unscoped terminal tab — see state/workspace.ts's own header comment
// for why there is exactly one non-repo slot here.
// The files window partitions every tab under FILES_WORKSPACE; `workspaceId` keeps the repo id.
function workspaceKeyOf(tab: Pick<TabRecord, 'kind' | 'workspaceId'>): WorkspaceKey {
  if (useOpenedFilesWindowStore().active) return FILES_WORKSPACE;
  return tab.workspaceId ?? GENERAL_WORKSPACE;
}

export interface OpenTabResult {
  id: string;
  reused: boolean;
}

export const useTabsStore = createTabsStore({
  kinds: TAB_KINDS,
  workspaceKeyOf,
  control,
  // No explicit workspaceId ⇒ the one shared unscoped slot, regardless of kind — this app has no
  // per-kind mode the way Kira Studio's STUDIO_TAB_KIND_MODE does.
  fallbackWorkspaceKey: () =>
    useOpenedFilesWindowStore().active ? FILES_WORKSPACE : GENERAL_WORKSPACE,
  // state/workspace.ts imports closeWorkspaceTabs from this file, a two-way call graph — not a
  // cycle, since neither side reads the other at module-evaluation time, only inside a function
  // body called after both modules have finished loading (Pinia's own store registry resolves
  // useWorkspaceStore() lazily regardless).
  onHydrated(tabs) {
    if (useOpenedFilesWindowStore().active) return;
    const openRepos: string[] = [];
    for (const t of tabs) {
      const key = workspaceKeyOf(t);
      if (key === GENERAL_WORKSPACE) continue;
      if (!openRepos.includes(key)) openRepos.push(key);
    }
    useWorkspaceStore().openRepos = openRepos;
  },
  extend(actions) {
    // Creates workspace `workspaceId`'s pinned graph tab — a plain push that never activates it
    // (unlike openTab's own always-active-on-create behavior), so ensureWorkspaceShell
    // (state/repoTabs.ts) can decide separately whether to activate it (only when the workspace
    // has no active tab at all) — a restored session's own active tab must never be stolen by
    // shell creation.
    function createPinnedRepoGraphTab(workspaceId: string): TabRecord {
      const id = newUuid();
      const record = {
        id,
        connectionId: null,
        // A repo-graph tab has no file behind it, but `path` is a required column and an empty
        // one aborts the whole window's tab save, not just this row — the workspace key is this
        // tab's real identity, stable and unique per workspace.
        path: workspaceId,
        kind: 'repo-graph',
        state: defaultRepoGraphTabState(),
        order: actions.tabs.value.length,
        active: false,
        workspaceId,
      } as unknown as TabRecord;
      actions.tabs.value.push(record);
      actions.saveNow();
      return record;
    }

    // Closes every tab of workspace `key`, pinned tabs included — the one path that bypasses
    // closeTab's own pin guard, since discarding the whole workspace (closeRepoWorkspace,
    // state/workspace.ts) is a different act from closing one of its tabs.
    function closeWorkspaceTabsAction(key: WorkspaceKey): void {
      const ids = actions.tabs.value.filter((t) => workspaceKeyOf(t) === key).map((t) => t.id);
      for (const id of ids) actions.dropAllPagesForTab(id);
      actions.tabs.value = actions.tabs.value.filter((t) => workspaceKeyOf(t) !== key);
      delete actions.activeIdByWorkspace.value[key];
      delete actions.previewIdsByWorkspace.value[key];
      actions.saveNow();
    }

    // revealLine is re-patched (debounced) as the user scrolls/navigates — skipUnchanged since
    // Monaco's own scroll events fire far more often than the line actually changes.
    function patchRepoFileTabState(id: string, patch: Partial<RepoFileTabState>): void {
      actions.patchTabState(id, 'repo-file', patch, { skipUnchanged: true });
    }

    function patchLocalFileTabState(id: string, patch: Partial<LocalFileTabState>): void {
      actions.patchTabState(id, 'local-file', patch, { skipUnchanged: true });
    }

    // TabViewStateStore's own write() — git-ui re-serializes its whole PersistedViewState on
    // nearly every interaction (scroll, selection, column resize), so this skips a save when the
    // value is reference-unchanged, the same posture patchRepoFileTabState's own revealLine
    // follows.
    function patchRepoGraphTabState(id: string, patch: Partial<RepoGraphTabState>): void {
      actions.patchTabState(id, 'repo-graph', patch, { skipUnchanged: true });
    }

    // Next/Previous/Close Tab (the Window menu's own three items) — Kira Studio's own
    // tabs.ts activateNextTab/activatePrevTab/closeActiveTab, scoped to whichever workspace's own
    // tab strip is on screen right now. That's `visibleWorkspace()` — the open repo (or
    // GENERAL_WORKSPACE) while `git` mode is active, else the active module's own id — not
    // `useWorkspaceStore().active` directly, which stays git-only and would keep stepping the
    // Git module's own tabs even while, say, the Automations module is on screen.
    function activateNextTab(): void {
      actions.stepTab(visibleWorkspace(), 1);
    }

    function activatePrevTab(): void {
      actions.stepTab(visibleWorkspace(), -1);
    }

    function closeActiveTab(): void {
      const id = actions.activeIdByWorkspace.value[visibleWorkspace()];
      if (id) actions.closeTab(id);
    }

    return {
      createPinnedRepoGraphTab,
      closeWorkspaceTabs: closeWorkspaceTabsAction,
      patchRepoFileTabState,
      patchLocalFileTabState,
      patchRepoGraphTabState,
      activateNextTab,
      activatePrevTab,
      closeActiveTab,
    };
  },
});

/** Every tab in workspace `key` — pinned-kind tabs first (in their existing relative order), then
 *  the rest, regardless of where each sits in the store's own tab array. */
export function tabsForWorkspace(key: WorkspaceKey): TabRecord[] {
  return useTabsStore().tabsForWorkspace(key);
}

// Exported for state/workspace.ts's own closeRepoWorkspace, which must call it without importing
// useTabsStore's whole surface back — mirrors state/tabs.ts (Kira Studio)'s own export shape.
export function closeWorkspaceTabs(key: WorkspaceKey): void {
  useTabsStore().closeWorkspaceTabs(key);
}
