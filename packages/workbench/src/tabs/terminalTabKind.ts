import { type TabScope, type TerminalTabState, terminalTabStateSchema } from '@shared/domain/tabs';
import { t } from '@theme/i18n';
import { parseStateWith, type TabKindDef } from './types';

export interface TerminalSessionStatus {
  status: 'starting' | 'running' | 'exited' | 'failed';
  exitCode: number | null;
}

// A filesystem basename (state.cwd is an absolute path, not an encoded NodePath).
function basename(path: string): string {
  const slash = path.lastIndexOf('/');
  return slash === -1 ? path : path.slice(slash + 1);
}

/** P113 F6: kira-space's and kira-studio's own `state/tabKinds.ts` each carried this terminal
 *  descriptor byte-identical apart from their own mode constant. `Icon`/`Color`/`Menu` stay each
 *  app's own type parameters (`types.ts`'s own header comment explains why they genuinely differ)
 *  — this factory's own values (a literal icon string, a `TerminalTabState['color']`, an empty
 *  menu array) are valid members of every real instantiation, so the `as Icon`/`as Color` casts
 *  below assert what each call site's own type arguments already guarantee, mirroring the
 *  `tab as TerminalTabRecord` cast the original per-app code used for the same reason (`title`/
 *  `railColor` take the whole tab-record union `R`, not the narrowed terminal member). Callers
 *  supply their own `dropResources` since it reaches into an app-local `useTerminalsStore()`. */
export function terminalTabKind<
  K extends string,
  R extends { kind: string; state: unknown },
  Icon,
  Color,
  Menu,
>(
  mode: TabScope,
  dropResources: (tabId: string) => void,
  sessionStatus: (tabId: string) => TerminalSessionStatus | undefined,
): TabKindDef<K, R, Icon, Color, Menu> {
  type TerminalRecord = Extract<R, { kind: K }> & { id: string; state: TerminalTabState };
  type TerminalState = Extract<R, { kind: K }>['state'];

  return {
    mode,
    title: (tab) => {
      const s = (tab as TerminalRecord).state;
      return s.label || basename(s.cwd) || t('common.module.terminal.label');
    },
    // 'terminal-bash', not 'terminal': the 'console' kind (a SQL console) already owns that glyph.
    // The launch kind shows in the title and the rail colour, not a second icon vocabulary.
    icon: () => 'terminal-bash' as Icon,
    railColor: (tab) => (tab as TerminalRecord).state.color as Color,
    defaultState: (): TerminalState =>
      ({
        cwd: '',
        codeRepoId: '',
        command: '',
        label: '',
        color: 'none',
        launchKind: 'shell',
        scriptId: '',
        launchToken: '',
        taskId: '',
        branchId: '',
      }) as TerminalState,
    // Copying the cwd (and command/label/color) means "Duplicate tab" on a terminal opens a
    // second session with the same launch — which needs no special case.
    // A launch token is single-use: a duplicate must not carry the original's.
    duplicateState: (tab) =>
      ({ ...(tab as TerminalRecord).state, launchToken: '' }) as TerminalState,
    // The one place a PTY dies on close — blind-called for every kind (dropPageStoresForTab), so
    // a non-terminal tab id is a registry miss here, not a branch.
    dropResources,
    menuExtras: () => [],
    // A script tab shows how its run is going; a plain shell has nothing to flag.
    badge: (tab) => {
      const rec = tab as TerminalRecord;
      if (rec.state.launchKind !== 'script') return null;
      const s = sessionStatus(rec.id);
      if (!s) return null;
      if (s.status === 'starting' || s.status === 'running')
        return { icon: 'loading', tooltip: t('common.state.running.status') };
      if (s.status === 'failed' || (s.exitCode ?? 0) !== 0)
        return { icon: 'error', tooltip: t('common.state.failed.status') };
      return { icon: 'check', tooltip: t('common.state.succeeded.status') };
    },
    parseState: parseStateWith(terminalTabStateSchema) as (raw: unknown) => TerminalState | null,
  };
}
