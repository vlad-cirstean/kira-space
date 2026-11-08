import { t } from '@theme/i18n';
import type { Task } from '../wire';

/** A work task: a plain task or an orchestrator. Review and parked tasks are neither. */
export function isWorkKind(kind: Task['kind']): boolean {
  return kind === 'task' || kind === 'orchestrator';
}

export interface KindToggle {
  /** The orchestrator switch cannot flip; `why` says why. */
  disabled: boolean;
  why: string;
}

/** Whether a task can switch to or from orchestrator; the backend refuses the same cases. */
export function orchestratorToggle(card: {
  task: Pick<Task, 'kind'>;
  parent: unknown;
  children: readonly unknown[];
  archivedChildren: readonly unknown[];
}): KindToggle {
  if (card.task.kind === 'orchestrator') {
    return card.children.length + card.archivedChildren.length > 0
      ? { disabled: true, why: t('space.ade.orchestrator.switch.hasChildren.tooltip') }
      : { disabled: false, why: '' };
  }
  if (card.parent) {
    return { disabled: true, why: t('space.ade.orchestrator.switch.hasParent.tooltip') };
  }
  if (card.task.kind !== 'task') {
    return { disabled: true, why: t('space.ade.orchestrator.switch.notTask.tooltip') };
  }
  return { disabled: false, why: '' };
}
