import type { OpLogRecord } from '@shared/domain/ops';
import type { ScriptRun } from '@shared/domain/scriptRuns';
import { t } from '@theme/i18n';
import { announceApp } from '@workbench/behaviour/liveRegion';
import { type ComputedRef, computed } from 'vue';
import { copyAndAnnounce } from '../../behaviour/clipboard';
import { formatCurrency } from '../../behaviour/format';
import { type OpLogDetailLine, type OpLogStatusFilter, opLogMenuItems } from '../../lib/opLog';
import type { MenuItem } from '../../state/contextMenu';
import { opLogMatches } from '../../state/createOpLogStore';
import { useAutomationsModule } from '../module';
import { useRerun } from '../runScript';
import { keyOf, useScriptTabs } from '../scriptTabs';
import { type AutomationOp, automationOp, isLiveRun } from './runOps';
import { useResumeScriptRun, useScriptRuns, useStopScriptRun } from './runsQueries';
import { runReportText, stateLabel } from './runText';

export function canStopRun(run: ScriptRun): boolean {
  return isLiveRun(run);
}

/** Stops a live run: a waiting one is declined, a running one is stopped. */
export function useStopAutomationRun(): (run: ScriptRun) => void {
  const ctx = useAutomationsModule();
  const stop = useStopScriptRun();
  return (run) => {
    if (run.state === 'waiting') void ctx.runs.confirmDecline(run.id);
    else if (run.state === 'running') stop.mutate(run.id);
  };
}

/** The slice of an op store this reads; a Pinia store satisfies it. */
export interface OpFilterState {
  filterText: string;
  statusFilter: OpLogStatusFilter;
  clearedAt: number;
}

export function triggerLabel(run: ScriptRun): string {
  switch (run.trigger) {
    case 'ade':
      return t('automations.run.triggerAde.label', { task: run.taskTitle });
    case 'scheduled':
      return t('automations.run.triggerScheduled.label');
    case 'mcp':
      return t('automations.run.triggerMcp.label');
    case 'manual':
      return t('automations.ops.triggerManual.label');
    default:
      return t('common.module.terminal.label');
  }
}

function extraText(run: ScriptRun): string {
  return `${run.scriptName} ${run.taskTitle} ${run.trigger}`;
}

// Both linked lists are newest first.
function mergeNewestFirst<R extends OpLogRecord>(
  ops: readonly R[],
  autos: readonly AutomationOp[],
): (R | AutomationOp)[] {
  const out: (R | AutomationOp)[] = [];
  let i = 0;
  let j = 0;
  while (i < ops.length && j < autos.length) {
    const op = ops[i] as R;
    const auto = autos[j] as AutomationOp;
    if (Date.parse(auto.startedAt) > Date.parse(op.startedAt)) {
      out.push(auto);
      j++;
    } else {
      out.push(op);
      i++;
    }
  }
  while (i < ops.length) out.push(ops[i++] as R);
  while (j < autos.length) out.push(autos[j++] as AutomationOp);
  return out;
}

export interface AutomationOps {
  /** Live runs, for the panel's running count. */
  runningCount: ComputedRef<number>;
  merge<R extends OpLogRecord>(ops: readonly R[]): (R | AutomationOp)[];
  kindLabel(): string;
  /** Shown in place of the command while a run waits for its confirmation. */
  runningNote(op: AutomationOp): string | null;
  statusNote(op: AutomationOp): string | null;
  detailLines(op: AutomationOp): OpLogDetailLine[];
  canCancel(op: AutomationOp): boolean;
  cancel(op: AutomationOp): void;
  menuFor(op: AutomationOp): MenuItem[];
}

/** Automation runs as Operations panel rows, for an app's panel to merge into its own op list. */
export function useAutomationOps(filters: OpFilterState): AutomationOps {
  const ctx = useAutomationsModule();
  const { data } = useScriptRuns();
  const stopRun = useStopAutomationRun();
  const resume = useResumeScriptRun();
  const rerun = useRerun();
  const tabs = useScriptTabs();

  const visible = computed<AutomationOp[]>(() =>
    (data.value ?? [])
      .map(automationOp)
      .filter(
        (op) =>
          Date.parse(op.startedAt) >= filters.clearedAt &&
          opLogMatches(op, filters.filterText, filters.statusFilter, extraText(op.run)),
      )
      .sort((a, b) => Date.parse(b.startedAt) - Date.parse(a.startedAt)),
  );
  const runningCount = computed(() => (data.value ?? []).filter(isLiveRun).length);

  function terminalTabOpen(run: ScriptRun): boolean {
    return run.terminalId !== '' && ctx.tabs.list().some((tab) => tab.id === run.terminalId);
  }
  function canOpen(run: ScriptRun): boolean {
    if (run.state === 'skipped') return false;
    return run.kind === 'smart' || run.terminalId === '' || terminalTabOpen(run);
  }
  function openRun(run: ScriptRun): void {
    if (!canOpen(run)) return;
    ctx.showAutomations();
    if (run.kind !== 'smart' && run.terminalId !== '') ctx.tabs.activate(run.terminalId);
    else tabs.openRun({ ...keyOf(run), runId: run.id, label: run.scriptName });
  }

  function canCancel(op: AutomationOp): boolean {
    return canStopRun(op.run);
  }
  function cancel(op: AutomationOp): void {
    stopRun(op.run);
  }

  function menuFor(op: AutomationOp): MenuItem[] {
    const { run } = op;
    const ended = !isLiveRun(run);
    return [
      ...opLogMenuItems(op, () => cancel(op), canCancel(op)),
      {
        type: 'item',
        id: 'copy-for-agent',
        label: t('common.copyForAgent.label'),
        icon: 'copy',
        disabled: !ended,
        run: () =>
          void copyAndAnnounce(runReportText(run, ''), t('automations.run.reportCopied.announce')),
      },
      {
        type: 'item',
        id: 'open-run',
        label: t('automations.ops.openRun.label'),
        disabled: !canOpen(run),
        run: () => openRun(run),
      },
      {
        type: 'item',
        id: 'resume-run',
        label: t('common.action.resume.label'),
        disabled: !ended || !run.resumable,
        run: () => {
          ctx.showAutomations();
          resume.mutate(run);
        },
      },
      {
        type: 'item',
        id: 'run-again',
        label: t('automations.run.again.label'),
        disabled: !ended || !run.scriptId,
        run: () => {
          ctx.showAutomations();
          void rerun(run).then((message) => {
            if (message) announceApp(message);
          });
        },
      },
    ];
  }

  function statusNote(op: AutomationOp): string | null {
    const { state } = op.run;
    return state === 'blocked' || state === 'skipped' ? stateLabel(state) : null;
  }

  function runningNote(op: AutomationOp): string | null {
    return op.run.state === 'waiting' ? stateLabel('waiting') : null;
  }

  function detailLines(op: AutomationOp): OpLogDetailLine[] {
    const out = op.run.outcome;
    const lines: OpLogDetailLine[] = [];
    if (out?.summary) lines.push({ text: out.summary });
    if (out?.costUsd !== undefined)
      lines.push({ text: t('common.cost.label', { value: formatCurrency(out.costUsd) }) });
    if (out?.permissionDenials?.length)
      lines.push({
        text: t('automations.run.outcome.denied.label', {
          tools: out.permissionDenials.join(', '),
        }),
      });
    if (out?.exitCode !== undefined)
      lines.push({ text: t('automations.run.outcome.exitCode.label', { code: out.exitCode }) });
    for (const line of (out?.outputTail ?? '').split('\n'))
      if (line !== '') lines.push({ text: line, tone: 'error' });
    return lines;
  }

  return {
    runningCount,
    merge: (ops) => mergeNewestFirst(ops, visible.value),
    kindLabel: () => t('common.module.automations.label'),
    runningNote,
    statusNote,
    detailLines,
    canCancel,
    cancel,
    menuFor,
  };
}
