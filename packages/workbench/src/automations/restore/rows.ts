import type { ScriptRunRestoreItem, ScriptRunStarted } from '@shared/domain/scriptRuns';
import { t } from '@theme/i18n';
import type { RestoreRow } from '@workbench/restore/types';
import { useAutomationsModule } from '../module';
import { useScriptTabs } from '../scriptTabs';

function badges(it: ScriptRunRestoreItem): string[] {
  const out: string[] = [];
  if (it.kind === 'smart') out.push(t('common.smartScript.label'));
  if (it.model) out.push(it.model);
  if (it.trigger === 'scheduled') out.push(t('automations.run.triggerScheduled.label'));
  if (it.trigger === 'mcp') out.push(t('automations.run.triggerMcp.label'));
  if (it.taskTitle)
    out.push(
      it.branchLabel
        ? t('automations.run.taskOnBranch.label', { task: it.taskTitle, branch: it.branchLabel })
        : t('automations.run.taskOnly.label', { task: it.taskTitle }),
    );
  if (it.resume) out.push(t('automations.restore.resume.label'));
  if (it.params.length > 0)
    out.push(t('automations.restore.params.label', { count: it.params.length }));
  return out;
}

/** Dialog rows for the script runs a restart cut; `errors` holds each run's last restore failure by run id. */
export function runRestoreRows(
  items: readonly ScriptRunRestoreItem[],
  errors: ReadonlyMap<string, string>,
  group?: string,
): RestoreRow[] {
  return items.map((it) => ({
    id: it.runId,
    label: it.scriptName,
    detail: it.command,
    folder: it.cwd,
    badges: badges(it),
    state: it.status === 'ready' ? 'ready' : 'unavailable',
    status: it.status,
    reason: it.status === 'ready' ? undefined : it.reason,
    error: errors.get(it.runId),
    group,
  }));
}

/** Opens a restored run's tab the way the run dialog does. */
export function useOpenRestoredRun(): (
  it: ScriptRunRestoreItem,
  started: ScriptRunStarted,
) => void {
  const ctx = useAutomationsModule();
  const tabs = useScriptTabs();
  return (it, started) => {
    const key = { scriptId: it.scriptId, taskId: it.taskId, branchId: it.branchId };
    if (!started.terminal) {
      tabs.openRun({ ...key, runId: started.runId, label: it.scriptName });
      return;
    }
    ctx.openTerminalTab({
      cwd: started.terminal.cwd,
      launch: {
        command: it.command,
        label: it.scriptName,
        color: it.color,
        kind: 'script',
        scriptId: it.scriptId,
        launchToken: started.terminal.token,
        taskId: it.taskId,
        branchId: it.branchId,
      },
      replace: tabs.replaceTarget(key),
    });
  };
}
