import { t } from '@theme/i18n';
import type { ArtifactGap } from '../wire';

/** Why a source is partial. */
export function gapText(g: ArtifactGap): string {
  return g.repo
    ? t('space.ade.artifacts.gap.body', { source: g.source, repo: g.repo })
    : t('space.ade.artifacts.gapNoRepo.body', { source: g.source });
}
