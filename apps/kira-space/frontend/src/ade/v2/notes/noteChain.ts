import { t } from '@theme/i18n';
import type { Handover } from '../wire';

export type NoteAuthor = 'user' | 'agent';

/** Who wrote the entry: the user, or an agent (`agent` and `run` rows are both agent output). */
export function noteAuthor(h: Handover): NoteAuthor {
  return h.source === 'user' ? 'user' : 'agent';
}

function stepLabel(name: string, id: string): string {
  return name || (id === 'done' ? t('common.done.status') : id);
}

/** The entry's heading: a note is a note; a draft or handed row names the stages it spans. */
export function entryTitle(h: Handover): string {
  if (h.state === 'note') return t('space.ade.notes.entry.note.title');
  const from = stepLabel(h.fromStepName, h.fromStepId);
  return h.state === 'draft'
    ? t('space.ade.handover.draftFrom.label', { from })
    : t('space.ade.handover.fromTo.label', { from, to: stepLabel(h.toStepName, h.toStepId) });
}

/** "during {stage}" for a note written in a stage; '' otherwise. */
export function noteStage(h: Handover): string {
  if (h.state !== 'note' || !(h.fromStepName || h.fromStepId)) return '';
  return t('space.ade.notes.entry.during.label', {
    stage: stepLabel(h.fromStepName, h.fromStepId),
  });
}
