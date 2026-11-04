import { type MessageKey, t } from '@theme/i18n';
import type { Tone } from '../board/actions';
import type { Artifact } from '../wire';

const TONE: Record<string, Tone> = {
  open: 'blue',
  merged: 'green',
  deployed: 'green',
  pushed: 'blue',
  closed: 'grey',
  draft: 'grey',
  local: 'grey',
  stale: 'amber',
};
const LABEL: Record<string, MessageKey> = {
  open: 'common.action.open.label',
  merged: 'common.merged.status',
  closed: 'common.closed.status',
  draft: 'common.draft.status',
  deployed: 'space.ade.branchPanel.state.deployed.status',
  stale: 'common.stale.status',
  pushed: 'space.ade.artifacts.status.pushed.status',
  local: 'space.ade.artifacts.status.local.status',
};

export const artifactStatusTone = (status: string): Tone => TONE[status] ?? 'grey';

/** A known status in words; a tracker's own status name (In Review) shows as sent. */
export function artifactStatusLabel(status: string): string {
  const key = LABEL[status];
  return key ? t(key) : status;
}

const KIND_LABEL = {
  changeRequest: 'common.gitHost.changeRequest.label',
  issue: 'space.ade.taskTab.issue.label',
  branch: 'common.field.branch.label',
  deployment: 'space.ade.artifacts.kind.deployment.label',
  doc: 'space.ade.artifacts.kind.doc.label',
  link: 'space.ade.artifacts.kind.link.label',
} as const satisfies Record<Artifact['kind'], MessageKey>;

export const artifactKindLabel = (kind: Artifact['kind']): string => t(KIND_LABEL[kind]);
