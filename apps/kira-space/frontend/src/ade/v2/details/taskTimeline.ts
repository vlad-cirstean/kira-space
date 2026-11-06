import { t } from '@theme/i18n';
import type { Artifact, Handover, Task } from '../wire';

export interface TimelineEntry {
  key: string;
  kind: 'created' | 'branch' | 'artifact' | 'handover';
  /** Draft handovers only. */
  draft: boolean;
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

const stepName = (name: string, id: string): string =>
  name || (id === 'done' ? t('common.done.status') : id);

/** What happened to the task, oldest first: its creation, each branch, each stored artifact (detected
 *  ones have no real time) and each handover. */
export function taskTimeline(
  task: Task,
  branches: readonly TimelineBranch[],
  artifacts: readonly Artifact[],
  handovers: readonly Handover[],
): TimelineEntry[] {
  const out: TimelineEntry[] = [
    {
      key: 'created',
      kind: 'created',
      draft: false,
      at: task.createdAt,
      label: t('space.ade.details.timeline.created.label'),
      url: '',
      sessionId: '',
    },
  ];
  for (const b of branches) {
    out.push({
      key: `branch:${b.id}`,
      kind: 'branch',
      draft: false,
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
      kind: 'artifact',
      draft: false,
      at: a.createdAt,
      label: t('space.ade.details.timeline.artifact.label', { title: a.title || a.ref || a.url }),
      url: a.url,
      sessionId: a.sessionId,
    });
  }
  for (const h of handovers) {
    if (h.state === 'note') continue; // shown in the notes chain
    const from = stepName(h.fromStepName, h.fromStepId);
    out.push({
      key: `handover:${h.id}`,
      kind: 'handover',
      draft: h.state === 'draft',
      at: h.createdAt,
      label:
        h.state === 'draft'
          ? t('space.ade.details.timeline.draft.label', { from })
          : t('space.ade.details.timeline.handover.label', {
              from,
              to: stepName(h.toStepName, h.toStepId),
            }),
      url: '',
      sessionId: h.sessionId,
    });
  }
  return out.sort((a, b) => a.at - b.at);
}
