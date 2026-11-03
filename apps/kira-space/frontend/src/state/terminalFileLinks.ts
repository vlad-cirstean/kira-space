import { announceApp } from '@workbench/behaviour/liveRegion';
import { errorText } from '@workbench/bridge/codedError';
import { logWarn } from '@workbench/bridge/log';
import type { TerminalFileLinks } from '@workbench/terminal/terminalRenderer';
import { control } from '../bridge/control';

/** fallbackCwd answers for a terminal that already exited, where the backend has no live cwd. */
export function createTerminalFileLinks(fallbackCwd: (tabId: string) => string): TerminalFileLinks {
  return {
    resolve: (tabId, paths) =>
      control.openedFilesTerminalResolve({
        terminalId: tabId,
        fallbackCwd: fallbackCwd(tabId),
        paths,
      }),
    open: (tabId, target) => {
      control
        .openedFilesTerminalOpen({
          terminalId: tabId,
          fallbackCwd: fallbackCwd(tabId),
          path: target.path,
          line: target.line ?? 0,
          col: target.col ?? 0,
        })
        .catch((err: unknown) => {
          logWarn('terminalFileLinks', 'open failed', err);
          announceApp(errorText(err));
        });
    },
  };
}
