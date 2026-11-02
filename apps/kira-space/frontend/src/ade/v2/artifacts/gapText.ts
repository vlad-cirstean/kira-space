import { type MessageKey, t } from '@theme/i18n';
import type { ArtifactGap } from '../wire';

const TRACKER_LABEL: Record<string, MessageKey> = { jira: 'common.product.jira.label' };

const REASON_KEY: Record<string, MessageKey> = {
  unavailable: 'space.ade.artifacts.gap.unavailable.body',
  unauthenticated: 'space.ade.artifacts.gap.unauthenticated.body',
  notFound: 'space.ade.artifacts.gap.notFound.body',
  otherSite: 'space.ade.artifacts.gap.otherSite.body',
  timeout: 'space.ade.artifacts.gap.timeout.body',
};

/** A tracker's display name; an unknown provider shows its id. */
function trackerName(id: string): string {
  const key = TRACKER_LABEL[id];
  return key ? t(key) : id;
}

/** Why a source is partial. Any other tracker failure reads as a plain read failure. */
export function gapText(g: ArtifactGap): string {
  const source = trackerName(g.source);
  const reason = g.reason ?? '';
  const key = REASON_KEY[reason];
  if (key) return t(key, { source });
  return g.repo
    ? t('space.ade.artifacts.gap.body', { source, repo: g.repo })
    : t('space.ade.artifacts.gapNoRepo.body', { source });
}
