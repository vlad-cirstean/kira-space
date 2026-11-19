import type { ScriptRun } from '@shared/domain/scriptRuns';
import type { CustomScript } from '@shared/domain/scripts';
import { t } from '@theme/i18n';
import { useAutomationsModule } from './module';
import { type RunDialogRequest, useScriptRunDialogStore } from './run/runDialog';
import { useScriptRuns } from './runs/runsQueries';
import { keyOf, sameKey, useScriptTabs } from './scriptTabs';
import { isBuiltinVar, varsUsed } from './smart/smartSettings';

/** The task and branch a run is for, and where it was started from. */
export type RunContext = Pick<RunDialogRequest, 'taskId' | 'branchId' | 'from'>;

/** Starts a script. A normal script without params opens a terminal tab at its resolved folder; any
 *  other script opens the run dialog, where the person confirms a preview. A script with a live tab or a running run only focuses or reopens that tab. Throws the
 *  folder's blocker (nothing opens) or the failure of resolving it. */
export function useRunScript(): (
  script: CustomScript,
  prefill?: Record<string, string[]>,
  context?: RunContext,
) => Promise<void> {
  const ctx = useAutomationsModule();
  const dialog = useScriptRunDialogStore();
  const tabs = useScriptTabs();
  const { data: runs } = useScriptRuns();
  return async (script, prefill, context) => {
    const key = {
      scriptId: script.id,
      taskId: context?.taskId ?? '',
      branchId: context?.branchId ?? '',
    };
    if (tabs.focusLive(key)) return;
    const running = runs.value?.find(
      (r) => r.state === 'running' && sameKey(keyOf(r), key) && r.kind === 'smart',
    );
    if (running) {
      tabs.openRun({ ...key, runId: running.id, label: script.name });
      return;
    }
    const usesTask = ctx.ade && varsUsed(script.command).some(isBuiltinVar);
    if (script.kind === 'smart' || script.params.length > 0 || usesTask || context?.taskId) {
      dialog.open({ scriptId: script.id, prefill, ...context });
      return;
    }
    const dir = await ctx.runs.resolveDir(script.id);
    if (dir.blocker !== '') throw new Error(dir.blocker);
    ctx.openTerminalTab({
      cwd: dir.path,
      launch: {
        command: script.command,
        label: script.name,
        color: script.color,
        kind: 'script',
        scriptId: script.id,
        taskId: key.taskId,
        branchId: key.branchId,
      },
      replace: tabs.replaceTarget(key),
    });
  };
}

/** Run again: reopens the script with the run's own non-secret params. A multiselect's stored value
 *  is newline-joined. */
export function useRerun(): (run: ScriptRun) => Promise<void> {
  const ctx = useAutomationsModule();
  const runScript = useRunScript();
  return async (run) => {
    const script = ctx.scripts.records().find((s) => s.id === run.scriptId);
    if (!script) throw new Error(t('automations.script.gone.error'));
    const prefill: Record<string, string[]> = {};
    for (const p of script.params) {
      const was = run.params.find((x) => x.name === p.name);
      if (!was || p.secret) continue;
      prefill[p.name] =
        p.type === 'multiselect' ? was.value.split('\n') : was.value === '' ? [] : [was.value];
    }
    const withTask: RunContext | undefined = run.taskId
      ? { taskId: run.taskId, branchId: run.branchId, from: 'automations' }
      : undefined;
    await runScript(script, prefill, withTask);
  };
}
