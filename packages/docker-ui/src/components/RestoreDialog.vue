<script setup lang="ts">
import { t } from '@theme/i18n';
import RestoreDialog from '@workbench/restore/RestoreDialog.vue';
import type { RestoreRow } from '@workbench/restore/types';
import { computed, ref } from 'vue';
import { useDismissRestore, useRestoreContainers } from '../queries';
import type { DockerRestoreState } from '../wire';

const props = defineProps<{ state: DockerRestoreState }>();
const emit = defineEmits<{ close: [] }>();

const restore = useRestoreContainers();
const dismiss = useDismissRestore();
const excluded = ref<string[]>([]);

const errors = computed(() => {
  const out = new Map<string, string>();
  for (const i of props.state.items) if (i.error) out.set(i.id, i.error);
  for (const r of restore.data.value?.items ?? []) if (!r.ok && r.error) out.set(r.id, r.error);
  return out;
});

// A Compose project is one row: its members start together, in dependency order.
const rows = computed<RestoreRow[]>(() => {
  const out: RestoreRow[] = [];
  const projects = new Map<string, RestoreRow>();
  for (const item of props.state.items) {
    if (item.status === 'back' || item.status === 'gone') {
      out.push({
        id: item.id,
        label: item.name,
        state: 'unavailable',
        status: item.status,
        reason: t(
          item.status === 'back'
            ? 'docker.restore.dialog.back.title'
            : 'docker.restore.dialog.gone.body',
        ),
      });
      continue;
    }
    const waiting = item.status === 'waiting';
    if (!item.composeProject) {
      out.push({
        id: item.id,
        label: item.name,
        detail: item.image,
        state: waiting ? 'waiting' : 'ready',
        status: item.status,
        reason: waiting ? t('docker.restore.dialog.waiting.label') : undefined,
        error: errors.value.get(item.id),
      });
      continue;
    }
    let row = projects.get(item.composeProject);
    if (!row) {
      row = {
        id: `project:${item.composeProject}`,
        label: t('docker.restore.dialog.project.label', { name: item.composeProject }),
        state: 'ready',
        status: 'restorable',
        members: [],
      };
      projects.set(item.composeProject, row);
      out.push(row);
    }
    row.members?.push({ id: item.id, label: item.name, error: errors.value.get(item.id) });
    if (waiting) {
      row.state = 'waiting';
      row.status = 'waiting';
      row.reason = t('docker.restore.dialog.waiting.label');
    }
  }
  return out;
});

const callError = computed(() => (restore.error.value ?? dismiss.error.value)?.message);

async function submit(ids: string[]): Promise<void> {
  dismiss.reset();
  const members = new Map(rows.value.map((r) => [r.id, r.members?.map((m) => m.id) ?? [r.id]]));
  try {
    const res = await restore.mutateAsync({
      engineId: props.state.engineId,
      ids: ids.flatMap((id) => members.get(id) ?? []),
    });
    if (res.items.every((i) => i.ok)) emit('close');
  } catch {
    // Surfaced through `callError`.
  }
}

async function dismissAll(): Promise<void> {
  restore.reset();
  try {
    await dismiss.mutateAsync(props.state.engineId);
    emit('close');
  } catch {
    // Surfaced through `callError`.
  }
}
</script>

<template>
  <RestoreDialog
    v-model:excluded="excluded"
    :title="t('docker.restore.dialog.title')"
    :description="t('docker.restore.dialog.description')"
    :rows="rows"
    :pending="restore.isPending.value"
    :dismissing="dismiss.isPending.value"
    :call-error="callError"
    @confirm="submit"
    @dismiss="dismissAll"
    @close="emit('close')"
  />
</template>
