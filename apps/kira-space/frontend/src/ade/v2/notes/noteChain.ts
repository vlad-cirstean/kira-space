import { t } from '@theme/i18n';
import type { Handover } from '../wire';

export type NoteAuthor = 'user' | 'agent' | 'child' | 'orchestrator' | 'pass';

/**
 * Who wrote the entry: the user, an agent (`agent` and `run` rows are both agent output), a child
 * task's finish copied in, the orchestrator's brief, or the start of a pass.
 */
export function noteAuthor(h: Handover): NoteAuthor {
  if (h.source === 'user') return 'user';
  if (h.source === 'child' || h.source === 'orchestrator' || h.source === 'pass') return h.source;
  return 'agent';
}

export const AUTHOR_ICON: Record<NoteAuthor, string> = {
  user: 'account',
  agent: 'hubot',
  child: 'type-hierarchy-sub',
  orchestrator: 'organization',
  pass: 'run-all',
};

export function authorLabel(a: NoteAuthor): string {
  switch (a) {
    case 'user':
      return t('common.picker.you.label');
    case 'agent':
      return t('memory.common.origin.agent.label');
    case 'child':
      return t('space.ade.orchestrator.chain.child.label');
    case 'orchestrator':
      return t('space.ade.orchestrator.badge.label');
    case 'pass':
      return t('space.ade.orchestrator.chain.pass.label');
  }
}

export function authorAria(a: NoteAuthor): string {
  switch (a) {
    case 'user':
      return t('space.ade.notes.author.user.aria');
    case 'agent':
      return t('space.ade.notes.author.agent.aria');
    case 'child':
      return t('space.ade.notes.author.child.aria');
    case 'orchestrator':
      return t('space.ade.notes.author.orchestrator.aria');
    case 'pass':
      return t('space.ade.notes.author.pass.aria');
  }
}

/** A child's finish is a copy of another task's text: it is not edited here. */
export function isReadOnlyEntry(h: Handover): boolean {
  return h.source === 'child';
}

/** The child a `child` row came from, `null` when the child is gone. */
export function childOf(h: Handover): { id: string; title: string } | null {
  if (h.source !== 'child' || !h.fromTaskId) return null;
  return { id: h.fromTaskId, title: h.fromTaskTitle || t('space.ade.board.newTask.label') };
}

function stepLabel(name: string, id: string): string {
  return name || (id === 'done' ? t('common.done.status') : id);
}

/** The entry's heading: a note is a note; a draft or handed row names the stages it spans. */
export function entryTitle(h: Handover): string {
  if (h.source === 'child') {
    return t('space.ade.orchestrator.chain.child.title', {
      title: h.fromTaskTitle || t('space.ade.board.newTask.label'),
    });
  }
  if (h.source === 'orchestrator') {
    return t('space.ade.orchestrator.chain.brief.title', {
      title: h.fromTaskTitle || h.fromStepName,
    });
  }
  if (h.source === 'pass') return t('space.ade.orchestrator.chain.pass.title');
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
