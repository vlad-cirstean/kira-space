import { createOpenTerminalTab, type TerminalLaunch } from '@workbench/state/createTerminalTabs';
import { useTabsStore } from './tabs';
import type { WorkspaceKey } from './workspace';

export type { TerminalLaunch };

// Opens a terminal tab in `opts.workspaceId` — in practice always a repo id (state/repoTabs.ts's
// openRepoTerminalTab) or GENERAL_WORKSPACE (an unscoped terminal launch, if this app ever grows
// one outside a repo workspace).
export const openTerminalTab = createOpenTerminalTab<WorkspaceKey>(
  // Thunk: tabs.ts -> tabKinds -> coderepos -> workspace -> repoTabs -> here is an import cycle, so
  // `useTabsStore` may still be uninitialised when this module evaluates.
  () => useTabsStore(),
);
