import { t } from '@theme/i18n';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { atRisk } from '../dialog/compose';
import { blockedReason } from '../dialog/flow';
import { useTaskBranchRisk } from '../queries';
import type { BranchRisk } from '../wire';

/** Dirty paths named in a confirm; the rest collapse into a count. */
const DIRTY_SHOWN = 8;

function riskText(risk: BranchRisk): string {
  return [
    risk.dirty.length
      ? t('space.ade.agentDialog.riskUncommitted.label', { count: risk.dirty.length })
      : '',
    risk.unmerged ? t('space.ade.agentDialog.riskUnmerged.label', { count: risk.unmerged }) : '',
  ]
    .filter(Boolean)
    .join(', ');
}

function lostText(risk: BranchRisk): string {
  const paths = risk.dirty.slice(0, DIRTY_SHOWN).map((d) => d.path);
  const rest = risk.dirty.length - paths.length;
  if (rest > 0) paths.push(t('common.more.label', { count: rest }));
  return [riskText(risk), ...paths].join(' · ');
}

export interface DiscardGuardOptions {
  branchId: string;
  /** Row name in messages. */
  name: string;
  title: string;
  /** Confirm body, given the text naming what is lost. */
  body: (lost: string) => string;
  confirmLabel: string;
}

/**
 * Edits that drop a row's worktree read its risk first: a blocked worktree throws, a risky one asks
 * for confirm. `discard` is true only after that confirm; `proceed` is false when it was declined.
 * The confirm closes on answer: the caller runs the work.
 */
export function useDiscardGuard(): (
  opts: DiscardGuardOptions,
) => Promise<{ proceed: boolean; discard: boolean }> {
  const confirm = useConfirmDialogStore();
  const riskM = useTaskBranchRisk();
  return async (opts) => {
    const risk = await riskM.mutateAsync({ branchId: opts.branchId });
    if (risk.blocked !== '') {
      throw new Error(
        t('space.ade.taskTab.branchMenu.blocked.error', {
          name: opts.name,
          reason: blockedReason(risk.blocked),
        }),
      );
    }
    if (!atRisk(risk)) return { proceed: true, discard: false };
    const confirmed = await confirm.confirm({
      tone: 'danger',
      title: opts.title,
      message: opts.body(lostText(risk)),
      confirmLabel: opts.confirmLabel,
    });
    return { proceed: confirmed, discard: confirmed };
  };
}
