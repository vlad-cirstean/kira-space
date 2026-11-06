import { t } from '@theme/i18n';
import type { Artifact, Task } from '../wire';

export interface TimelineEvent {
  key: string;
  type: 'created' | 'branch' | 'artifact';
  at: number;
  label: string;
  url: string;
  sessionId: string;
}

export interface TimelineBranch {
  id: string;
  name: string;
  addedAt: number;
}

/** What happened to the task outside notes, handovers and proof: its creation, each branch and each
 *  stored artifact (detected ones have no real time). Unsorted; `taskChain` orders them. */
export function timelineEvents(
  task: Task,
  branches: readonly TimelineBranch[],
  artifacts: readonly Artifact[],
): TimelineEvent[] {
  const out: TimelineEvent[] = [
    {
      key: 'created',
      type: 'created',
      at: task.createdAt,
      label: t('space.ade.details.timeline.created.label'),
      url: '',
      sessionId: '',
    },
  ];
  for (const b of branches) {
    out.push({
      key: `branch:${b.id}`,
      type: 'branch',
      at: b.addedAt,
      label: t('space.ade.details.timeline.branch.label', {
        name: b.name || t('common.field.branch.label'),
      }),
      url: '',
      sessionId: '',
    });
  }
  for (const a of artifacts) {
    if (a.id === '') continue;
    out.push({
      key: `artifact:${a.id}`,
      type: 'artifact',
      at: a.createdAt,
      label: t('space.ade.details.timeline.artifact.label', { title: a.title || a.ref || a.url }),
      url: a.url,
      sessionId: a.sessionId,
    });
  }
  return out;
}
