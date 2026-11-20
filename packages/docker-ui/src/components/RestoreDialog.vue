<script setup lang="ts">
import { t } from '@theme/i18n';
import RestoreDialog from '@workbench/restore/RestoreDialog.vue';
import type { RestoreConfirm, RestoreRow } from '@workbench/restore/types';
import { computed, ref } from 'vue';
import { useDismissRestore, useRestoreContainers } from '../queries';
import { dockerRestoreIds, dockerRestoreRows } from '../restoreRows';
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

const rows = computed<RestoreRow[]>(() => dockerRestoreRows(props.state, errors.value));

const callError = computed(() => (restore.error.value ?? dismiss.error.value)?.message);

async function submit({ ids }: RestoreConfirm): Promise<void> {
  dismiss.reset();
  try {
    const res = await restore.mutateAsync({
      engineId: props.state.engineId,
      ids: dockerRestoreIds(rows.value, ids),
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
    :dismiss-label="t('common.action.dontRestore.label')"
    :call-error="callError"
    @confirm="submit"
    @dismiss="dismissAll"
    @close="emit('close')"
  />
</template>
