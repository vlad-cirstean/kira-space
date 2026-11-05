<script setup lang="ts">
import type { ScriptRunRestoreItem, ScriptRunStarted } from '@shared/domain/scriptRuns';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import RestoreDialog from '@workbench/restore/RestoreDialog.vue';
import type { RestoreRow } from '@workbench/restore/types';
import { appUpdateStoreKey } from '@workbench/state/createAppUpdateStore';
import { computed, inject, ref } from 'vue';
import { useAutomationsModule } from '../module';
import { useScriptTabs } from '../scriptTabs';
import { useDismissRunsRestore, useRestoreRuns } from './restoreQueries';

const props = defineProps<{ items: readonly ScriptRunRestoreItem[] }>();
const emit = defineEmits<{ close: [] }>();

const ctx = useAutomationsModule();
const tabs = useScriptTabs();
const app = inject(appUpdateStoreKey);
const restore = useRestoreRuns();
const dismiss = useDismissRunsRestore();
const excluded = ref<string[]>([]);

const errors = computed(() => {
  const out = new Map<string, string>();
  for (const o of restore.data.value?.items ?? [])
    if (!o.started && o.error) out.set(o.runId, o.error);
  return out;
});

function badges(it: ScriptRunRestoreItem): string[] {
  const out: string[] = [];
  if (it.kind === 'smart') out.push(t('common.smartScript.label'));
  if (it.model) out.push(it.model);
  if (it.trigger === 'scheduled') out.push(t('automations.run.triggerScheduled.label'));
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

const rows = computed<RestoreRow[]>(() =>
  props.items.map((it) => ({
    id: it.runId,
    label: it.scriptName,
    detail: it.command,
    folder: it.cwd,
    badges: badges(it),
    state: it.status === 'ready' ? 'ready' : 'unavailable',
    status: it.status,
    reason: it.status === 'ready' ? undefined : it.reason,
    error: errors.value.get(it.runId),
  })),
);

// Opens the new run's tab the way the run dialog does.
function openStarted(it: ScriptRunRestoreItem, started: ScriptRunStarted): void {
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
}

const callError = computed(() => {
  const err = restore.error.value ?? dismiss.error.value;
  return err ? errorText(err) : undefined;
});

async function submit(ids: string[]): Promise<void> {
  dismiss.reset();
  const byId = new Map(props.items.map((i) => [i.runId, i]));
  try {
    const res = await restore.mutateAsync(
      ids.flatMap((id) => {
        const it = byId.get(id);
        return it ? [{ runId: id, hash: it.hash }] : [];
      }),
    );
    for (const o of res.items) {
      const it = byId.get(o.runId);
      if (it && o.started) openStarted(it, o.started);
    }
    if (res.items.every((o) => o.started)) emit('close');
  } catch {
    // Surfaced through `callError`.
  }
}

async function dismissAll(): Promise<void> {
  restore.reset();
  try {
    await dismiss.mutateAsync();
    emit('close');
  } catch {
    // Surfaced through `callError`.
  }
}
</script>

<template>
  <RestoreDialog
    v-model:excluded="excluded"
    :title="t('automations.restore.dialog.title')"
    :description="t('automations.restore.dialog.description', { app: app?.appName ?? '' })"
    :rows="rows"
    :pending="restore.isPending.value"
    :dismissing="dismiss.isPending.value"
    :call-error="callError"
    @confirm="submit"
    @dismiss="dismissAll"
    @close="emit('close')"
  />
</template>
