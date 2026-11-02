import type { PaletteColor } from '@shared/domain/color';
import { canonicalPath } from '@shared/domain/path';
import type { TerminalLaunchKind, TerminalTabState } from '@shared/domain/tabs';

// `state/terminalTabs.ts`'s only real divergence between the two apps was the workspace-key type
// (`AppMode` in Kira Studio, `WorkspaceKey` in Kira Space) — everything else, including the doc
// comments, was already identical. Each app now instantiates this once in its own thin
// `state/terminalTabs.ts`.

/** What a non-plain launch (Claude Code, or a custom script) seeds a terminal tab with. `kind` is
 *  the tab's own launchKind — always given explicitly by the one caller that constructs one, never
 *  left to infer from `command`. */
export interface TerminalLaunch {
  command: string;
  label: string;
  color: PaletteColor;
  kind: TerminalLaunchKind;
  /** Set for a stored script: Go then runs the stored command in its resolved folder. */
  scriptId?: string;
  /** Single-use token of a parametrised script's launch (ScriptRuns Start), or of an agent launch's env (ScriptRuns ResumeCommand). */
  launchToken?: string;
  /** The ADE task and branch a script runs for; with `scriptId`, the tab's run key. */
  taskId?: string;
  branchId?: string;
}

export interface OpenTerminalTabOpts<K extends string> {
  workspaceId: K;
  cwd: string;
  codeRepoId?: string;
  launch?: TerminalLaunch;
  /** Id of a tab the new one replaces in place. */
  replace?: string;
}

export interface OpenTerminalTabResult {
  id: string;
  reused: boolean;
}

interface TerminalTabsStore<K extends string> {
  openTab(
    kind: 'terminal',
    connectionId: null,
    path: string,
    makeState: () => TerminalTabState,
    opts: { reuse: boolean; workspaceId: K; replace?: string },
  ): OpenTerminalTabResult;
}

/** Opens a terminal tab in `opts.workspaceId`, generic over the workspace-key type `K` — pass the
 *  app's own `useTabsStore`. */
export function createOpenTerminalTab<K extends string>(useTabsStore: () => TerminalTabsStore<K>) {
  return function openTerminalTab(opts: OpenTerminalTabOpts<K>): OpenTerminalTabResult {
    return useTabsStore().openTab(
      'terminal',
      null,
      opts.cwd,
      () => ({
        cwd: canonicalPath(opts.cwd),
        codeRepoId: opts.codeRepoId ?? '',
        command: opts.launch?.command ?? '',
        label: opts.launch?.label ?? '',
        color: opts.launch?.color ?? 'none',
        launchKind: opts.launch?.kind ?? 'shell',
        scriptId: opts.launch?.scriptId ?? '',
        launchToken: opts.launch?.launchToken ?? '',
        taskId: opts.launch?.taskId ?? '',
        branchId: opts.launch?.branchId ?? '',
      }),
      { reuse: false, workspaceId: opts.workspaceId, replace: opts.replace },
    );
  };
}
