import type { Page } from '@playwright/test';
import { IPC } from './ipcChannels';
import { type ControlMockHandle, emitWailsEvent } from './mockRuntime';
import type { ControlSnapshot } from './types';

// A repo file tab no longer opens from a project tree: the files window opens it from the opened
// files the app was asked to show (flows/windowflow TestOpenedFilesWindow). These helpers boot a
// files window with repo files already pending, and add more later.

export interface RepoFx {
  id: string;
  root: string;
}

const target = (repo: RepoFx, relPath: string) => ({
  path: `${repo.root}/${relPath}`,
  repoId: repo.id,
  relPath,
});

/** Snapshots for a files window whose first opens are `relPaths` of `repo`. `repo` itself must be
 *  among the `extra` codeWorkspaceListRepos response, or it is added with `repoRecord`. */
export function filesWindowControl(
  repo: RepoFx & Record<string, unknown>,
  relPaths: readonly string[],
  extra: readonly ControlSnapshot[] = [],
): ControlSnapshot[] {
  const base: ControlSnapshot[] = [
    { channel: IPC.openedFilesIsFilesWindow, response: true },
    { channel: IPC.openedFilesPending, response: relPaths.map((p) => target(repo, p)) },
    { channel: IPC.codeWorkspaceListRepos, response: [repo] },
  ];
  const overridden = new Set(extra.map((s) => s.channel));
  return [...base.filter((s) => !overridden.has(s.channel)), ...extra];
}

/** A later open: the files window drains the next pending targets, the last one becoming active. */
export async function openMoreFiles(
  page: Page,
  control: ControlMockHandle,
  repo: RepoFx,
  relPaths: readonly string[],
  line?: number,
): Promise<void> {
  control.respond(
    IPC.openedFilesPending,
    relPaths.map((p) => ({ ...target(repo, p), ...(line ? { line } : {}) })),
  );
  await emitWailsEvent(page, IPC.openedFilesArrived, null);
}

export interface RestoredTab {
  kind: 'repo-file' | 'repo-diff';
  path: string;
  active?: boolean;
  /** Merged over the kind's empty state. */
  state?: Record<string, unknown>;
}

const EMPTY_STATE = {
  'repo-file': { revealLine: null, markdownReading: false, rev: null },
  'repo-diff': {
    left: null,
    right: null,
    leftLabel: null,
    rightLabel: null,
    review: null,
    working: null,
  },
} as const;

/** The persisted tabs of workspace `workspaceId`: the pinned graph, then `tabs`. Nothing opens a
 *  file or diff tab from a project tree any more, so a spec about those tabs restores them. */
export function restoredRepoTabs(
  workspaceId: string,
  tabs: readonly RestoredTab[],
  graphActive = false,
): ControlSnapshot {
  const base = { connectionId: null, workspaceId };
  return {
    channel: IPC.tabsList,
    response: [
      {
        ...base,
        id: 'restored-repo-graph',
        kind: 'repo-graph',
        path: '',
        order: 0,
        active: graphActive,
        state: { viewState: null, reviewSession: null },
      },
      ...tabs.map((tab, i) => ({
        ...base,
        id: `restored-${tab.kind}-${i}`,
        kind: tab.kind,
        path: tab.path,
        order: i + 1,
        active: tab.active ?? false,
        state: { ...EMPTY_STATE[tab.kind], ...tab.state },
      })),
    ],
  };
}
