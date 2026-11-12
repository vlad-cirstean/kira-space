import { t } from '@theme/i18n';
import type { Branch } from '../wire';

/** How a branch row shows its attach state; `tip` is the tooltip. */
export interface AttachFact {
  state: 'detached' | 'shared';
  tip: string;
}

/**
 * Detached: the row's worktree was left for another place that has the branch now. Shared: another
 * live task row has the same branch and this one holds it. A row with neither shows nothing.
 * `titleOf` names a task by id.
 */
export function attachOf(br: Branch, titleOf: (taskId: string) => string): AttachFact | null {
  const d = br.detached;
  if (d) {
    if (d.head !== '')
      return {
        state: 'detached',
        tip: t('space.ade.plan.branchRow.detachedSwitched.tooltip', {
          branch: br.name,
          head: d.head,
        }),
      };
    const vars = { branch: br.name, worktree: d.holder.worktree };
    switch (d.holder.kind) {
      case 'task':
        return {
          state: 'detached',
          tip: t('space.ade.plan.branchRow.detachedTask.tooltip', {
            ...vars,
            task: titleOf(d.holder.taskId),
          }),
        };
      case 'main':
        return {
          state: 'detached',
          tip: t('space.ade.plan.branchRow.detachedMain.tooltip', vars),
        };
      case 'other':
        return {
          state: 'detached',
          tip: t('space.ade.plan.branchRow.detachedOther.tooltip', vars),
        };
      case 'none':
        return {
          state: 'detached',
          tip: t('space.ade.plan.branchRow.detachedNone.tooltip', vars),
        };
    }
  }
  if (br.shared?.length && br.worktree !== '') {
    return {
      state: 'shared',
      tip: t('space.ade.plan.branchRow.shared.tooltip', {
        tasks: br.shared.map((p) => titleOf(p.taskId)).join(', '),
      }),
    };
  }
  return null;
}
