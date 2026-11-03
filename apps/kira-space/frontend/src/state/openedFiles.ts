import type { OpenedFileTarget } from '@shared/domain/repo';
import { logError } from '@workbench/bridge/log';
import { control } from '../bridge/control';
import { useModeStore } from './mode';
import { openLocalFileTab, openRepoFileTab } from './repoTabs';
import { GENERAL_WORKSPACE, useWorkspaceStore } from './workspace';

// Opens one target. A `line` places the cursor; `col` refines it.
export function openTarget(target: OpenedFileTarget): void {
  const reveal = target.line ? { line: target.line, column: target.col } : undefined;
  if (target.repoId) {
    useWorkspaceStore().openRepoWorkspace(target.repoId);
    openRepoFileTab(target.repoId, target.relPath, { preview: false, reveal });
  } else {
    openLocalFileTab(target.path, reveal);
    useWorkspaceStore().activateWorkspace(GENERAL_WORKSPACE);
    useModeStore().setMode('git');
  }
}

// Boot-time drain of the files Finder handed this window. Last target ends up active. A failure
// is logged; boot continues.
export async function openPendingFiles(): Promise<void> {
  try {
    for (const target of await control.openedFilesPending()) openTarget(target);
  } catch (err) {
    logError('bootstrap', 'opening pending files failed', err);
  }
}
