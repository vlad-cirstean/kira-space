import { t } from '@theme/i18n';
import type { BranchRequestItem } from './payload';

/** What asked for a branch call: a script run, a workflow run step or an agent session. */
export function branchSource(it: BranchRequestItem): string {
  if (it.source === 'automation') return t('space.ade.branchRequest.source.automation.label');
  if (it.source === 'run')
    return t('space.ade.branchRequest.source.run.label', { step: it.stepName });
  return it.stepName
    ? t('space.ade.branchRequest.source.session.label', { step: it.stepName })
    : t('space.settings.agentUsage.session.label');
}
