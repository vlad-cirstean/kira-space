import type { AppMode } from '@shared/domain/mode';
import { createOpenTerminalTab } from '@workbench/state/createTerminalTabs';
import { useTabsStore } from './tabs';

// Opens a terminal tab in `opts.workspaceId`, generic over any AppMode — in practice
// always 'terminal' (this app's own Automations module, every caller). `codeRepoId` stays on the
// options shape (TerminalTabState's own field, @shared/domain/tabs, shared with Kira Space) even
// though this app never passes it any more — a plain string, so it costs this app nothing to keep
// passing through unchanged.
export const openTerminalTab = createOpenTerminalTab<AppMode>(useTabsStore);
