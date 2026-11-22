import { t } from '@theme/i18n';
import { formatDateTime } from '@workbench/behaviour/format';
import type { ArtifactFound } from '../wire';

/** One source of a found link in words: "note of …", "handover A to B", "draft handover of A". */
export function foundSourceLabel(f: ArtifactFound): string {
  if (f.state === 'handed')
    return t('space.ade.links.found.handover.label', { from: f.fromStep, to: f.toStep });
  if (f.state === 'draft') return t('space.ade.links.found.draft.label', { step: f.fromStep });
  if (f.fromTask) return t('space.ade.links.found.child.label', { task: f.fromTask });
  return t('space.ade.links.found.note.label', { date: formatDateTime(f.createdAt) });
}

/** The pill's tooltip: "Found in note of …, handover A to B". */
export function foundTooltip(found: readonly ArtifactFound[]): string {
  return t('space.ade.links.found.tooltip', { sources: found.map(foundSourceLabel).join(', ') });
}
