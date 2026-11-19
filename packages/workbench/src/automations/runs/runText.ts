import type { ScriptRun } from '@shared/domain/scriptRuns';
import { type MessageKey, t } from '@theme/i18n';
import { formatCurrency } from '@workbench/behaviour/format';

const STATE_LABEL: Record<ScriptRun['state'], MessageKey> = {
  running: 'common.state.running.status',
  done: 'automations.run.state.done.status',
  failed: 'common.state.failed.status',
  cancelled: 'common.cancelled.status',
  blocked: 'common.needsYou.status',
  waiting: 'common.state.waitingForYou.status',
  skipped: 'common.skipped.status',
};

export function stateLabel(state: ScriptRun['state']): string {
  return t(STATE_LABEL[state]);
}

/** Elapsed for a run at `now`: live while running, fixed once finished. */
export function runElapsedMs(run: ScriptRun, now: number): number | null {
  const start = run.startedAt ?? run.createdAt;
  const end = run.finishedAt ?? (run.state === 'running' ? now : null);
  return end === null ? null : end - start;
}

/** Plain-text report a person pastes to an agent. `tail` is the live terminal tail; the stored output tail backs it once the tab is gone. */
export function runReportText(run: ScriptRun, tail: string): string {
  const out = run.outcome;
  const smart = run.kind === 'smart';
  const state = stateLabel(run.state);
  const lines = [
    t('automations.run.report.script.label', { value: run.scriptName }),
    smart
      ? t('automations.run.report.model.label', { value: run.model })
      : t('common.command.label', { value: run.command }),
    t('automations.run.report.folder.label', { value: run.cwd }),
    out?.reason
      ? t('automations.run.report.resultReason.label', { state, reason: out.reason })
      : t('automations.run.report.result.label', { state }),
  ];
  if (out?.summary) lines.push(t('automations.run.report.summary.label', { value: out.summary }));
  if (out?.costUsd !== undefined)
    lines.push(t('automations.run.report.cost.label', { value: formatCurrency(out.costUsd) }));
  if (out?.permissionDenials?.length)
    lines.push(
      t('automations.run.report.deniedTools.label', { value: out.permissionDenials.join(', ') }),
    );
  if (out?.exitCode !== undefined)
    lines.push(t('automations.run.report.exitCode.label', { value: out.exitCode }));
  if (out?.lastError)
    lines.push(t('automations.run.report.lastError.label', { value: out.lastError }));
  const output = tail !== '' ? tail : (out?.outputTail ?? '');
  if (output !== '') lines.push('', t('automations.run.report.lastOutput.label'), output);
  return lines.join('\n');
}
