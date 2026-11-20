<script setup lang="ts">
import { dockerRestoreIds, dockerRestoreRows } from '@kira/docker-ui';
import type { SessionRestoreAgent, SessionRestoreState } from '@shared/domain/sessionRestore';
import { t } from '@theme/i18n';
import { runRestoreRows, useOpenRestoredRun } from '@workbench/automations/restore/rows';
import { errorText } from '@workbench/bridge/codedError';
import RestoreDialog from '@workbench/restore/RestoreDialog.vue';
import type { RestoreConfirm, RestoreRow } from '@workbench/restore/types';
import { computed, ref } from 'vue';
import { useLaunchOpener } from '../ade/v2/dialog/useLaunchOpener';
import { useAdeBoardUiStore } from '../ade/v2/state/adeBoardUi';
import { useCloseSessionRestore, useRestoreSession } from './queries';

// One dialog for what a restart cut: script runs, Docker containers and agents. The X restores
// nothing (P696): there is no second button beside it.
const props = defineProps<{ state: SessionRestoreState }>();
const emit = defineEmits<{ close: [] }>();

const restore = useRestoreSession();
const close = useCloseSessionRestore();
const openStarted = useOpenRestoredRun();
const opener = useLaunchOpener();
const boardUi = useAdeBoardUiStore();
const excluded = ref<string[]>([]);
const options = ref<Record<string, boolean>>({});
// Terminal opens that failed after the call succeeded, by row id.
const openErrors = ref(new Map<string, string>());

const result = computed(() => restore.data.value);

const scriptErrors = computed(() => {
  const out = new Map<string, string>();
  for (const o of result.value?.scripts ?? []) if (!o.started && o.error) out.set(o.runId, o.error);
  return out;
});
const dockerErrors = computed(() => {
  const out = new Map<string, string>();
  for (const i of props.state.docker.items) if (i.error) out.set(i.id, i.error);
  for (const i of result.value?.docker?.items ?? []) if (!i.ok && i.error) out.set(i.id, i.error);
  return out;
});
const agentErrors = computed(() => {
  const out = new Map(openErrors.value);
  for (const a of result.value?.agents ?? []) if (a.error) out.set(a.id, a.error);
  return out;
});

const scriptRows = computed(() =>
  runRestoreRows(props.state.scripts, scriptErrors.value, t('common.module.scripts.label')),
);
const dockerRows = computed(() =>
  props.state.docker.state === 'pending'
    ? dockerRestoreRows(props.state.docker, dockerErrors.value, t('common.product.docker.label'))
    : [],
);

function agentRow(a: SessionRestoreAgent): RestoreRow {
  const badges = [a.provider];
  if (a.branchLabel) badges.push(a.branchLabel);
  if (a.working) badges.push(t('space.restore.working.label'));
  return {
    id: a.id,
    label: a.taskTitle,
    detail: a.step || undefined,
    badges,
    state: a.state === 'ready' ? 'ready' : 'unavailable',
    status: a.state,
    reason: a.state === 'ready' ? undefined : a.reason,
    error: agentErrors.value.get(a.id),
    group: t('common.module.agents.label'),
    toggleLabel: t('common.action.resume.label'),
    option: {
      label: t('common.continue.label'),
      required: a.continue === 'required',
      default: a.continueDefault,
    },
  };
}
const agentRows = computed(() => props.state.agents.map(agentRow));

const rows = computed(() => [...scriptRows.value, ...dockerRows.value, ...agentRows.value]);

const callError = computed(() => {
  const err = restore.error.value ?? close.error.value;
  return err ? errorText(err) : undefined;
});

async function submit({ ids, options: on }: RestoreConfirm): Promise<void> {
  openErrors.value = new Map();
  const picked = new Set(ids);
  const scripts = props.state.scripts.filter((s) => picked.has(s.runId));
  const dockerIds = dockerRestoreIds(
    dockerRows.value,
    ids.filter((id) => dockerRows.value.some((r) => r.id === id)),
  );
  const agents = props.state.agents.filter((a) => picked.has(a.id));
  let res: Awaited<ReturnType<typeof restore.mutateAsync>>;
  try {
    res = await restore.mutateAsync({
      scripts: scripts.map((s) => ({ runId: s.runId, hash: s.hash })),
      docker: dockerIds.length ? { engineId: props.state.docker.engineId, ids: dockerIds } : null,
      agents: agents.map((a) => ({ id: a.id, continue: on.includes(a.id) })),
    });
  } catch {
    // Surfaced through `callError`.
    return;
  }
  // Script tabs open before agent tabs: a normal script's run row exists once its tab does.
  for (const o of res.scripts) {
    const it = scripts.find((s) => s.runId === o.runId);
    if (it && o.started) openStarted(it, o.started);
  }
  for (const o of res.agents) {
    const a = agents.find((x) => x.id === o.id);
    if (!a || !o.launch) continue;
    try {
      await opener.open(o.launch);
      boardUi.openSession({
        taskId: a.taskId,
        branchId: a.branchId,
        sessionId: o.launch.sessionId,
      });
    } catch (err) {
      openErrors.value = new Map(openErrors.value).set(a.id, errorText(err));
    }
  }
  const ok =
    res.scripts.every((o) => o.started) &&
    (res.docker?.items.every((i) => i.ok) ?? true) &&
    res.agents.every((o) => !o.error) &&
    openErrors.value.size === 0;
  if (ok) emit('close');
}

async function dismiss(): Promise<void> {
  try {
    await close.mutateAsync();
  } catch {
    // The offer is per boot: nothing stays pending on a failed close.
  }
  emit('close');
}
</script>

<template>
  <RestoreDialog
    v-model:excluded="excluded"
    v-model:options="options"
    :title="t('space.restore.dialog.title')"
    :description="t('space.restore.dialog.description')"
    :rows="rows"
    :pending="restore.isPending.value"
    :call-error="callError"
    @confirm="submit"
    @close="dismiss"
  />
</template>
