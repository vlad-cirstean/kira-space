import type { ScriptRunState } from '@shared/domain/scriptRuns';
import {
  type ScriptRunTabState,
  scriptRunTabStateSchema,
  type TabScope,
} from '@shared/domain/tabs';
import { t } from '@theme/i18n';
import { parseStateWith, type TabKindDef } from './types';

/** The tab of one smart script run; its badge follows the run's state. `runState` reads the live
 *  run list (automations/runs/runsQueries.ts). */
export function scriptRunTabKind<
  K extends string,
  R extends { kind: string; state: unknown },
  Icon,
  Color,
  Menu,
>(
  mode: TabScope,
  runState: (runId: string) => ScriptRunState | undefined,
): TabKindDef<K, R, Icon, Color, Menu> {
  type RunRecord = Extract<R, { kind: K }> & { state: ScriptRunTabState };
  type RunState = Extract<R, { kind: K }>['state'];

  return {
    mode,
    title: (tab) => (tab as RunRecord).state.label || t('common.smartScript.label'),
    icon: () => 'sparkle' as Icon,
    railColor: () => undefined,
    defaultState: (): RunState =>
      ({ runId: '', label: '', scriptId: '', taskId: '', branchId: '' }) as RunState,
    duplicateState: (tab) => ({ ...(tab as RunRecord).state }) as RunState,
    dropResources: () => {},
    menuExtras: () => [],
    badge: (tab) => {
      switch (runState((tab as RunRecord).state.runId)) {
        case 'running':
          return { icon: 'loading', tooltip: t('common.state.running.status') };
        case 'done':
          return { icon: 'check', tooltip: t('automations.run.state.done.status') };
        case 'failed':
          return { icon: 'error', tooltip: t('common.state.failed.status') };
        case 'blocked':
          return { icon: 'warning', tooltip: t('common.needsYou.status') };
        case 'waiting':
          return { icon: 'bell', tooltip: t('common.state.waitingForYou.status') };
        default:
          return null;
      }
    },
    parseState: parseStateWith(scriptRunTabStateSchema) as (raw: unknown) => RunState | null,
  };
}
