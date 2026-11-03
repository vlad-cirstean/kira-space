import type { OpenedFileTarget } from '@shared/domain/repo';
import { logError } from '@workbench/bridge/log';
import { control } from '../bridge/control';
import { openLocalFileTab, openRepoFileTab } from './repoTabs';

// Opens one target in the files window. A `line` places the cursor; `col` refines it.
function openTarget(target: OpenedFileTarget): void {
  const reveal = target.line ? { line: target.line, column: target.col } : undefined;
  if (target.repoId) {
    openRepoFileTab(target.repoId, target.relPath, { preview: false, reveal });
  } else {
    openLocalFileTab(target.path, reveal);
  }
}

// Drains the files waiting for the files window: at boot and on every `kira:openedFiles:arrived`.
// Last target ends up active. A failure is logged; the window stays up.
export async function openPendingFiles(): Promise<void> {
  try {
    for (const target of await control.openedFilesPending()) openTarget(target);
  } catch (err) {
    logError('bootstrap', 'opening pending files failed', err);
  }
}
