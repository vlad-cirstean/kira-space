<script setup lang="ts">
import type { ScriptRunRestoreItem } from '@shared/domain/scriptRuns';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import RestoreDialog from '@workbench/restore/RestoreDialog.vue';
import type { RestoreConfirm, RestoreRow } from '@workbench/restore/types';
import { appUpdateStoreKey } from '@workbench/state/createAppUpdateStore';
import { computed, inject, ref } from 'vue';
import { useDismissRunsRestore, useRestoreRuns } from './restoreQueries';
import { runRestoreRows, useOpenRestoredRun } from './rows';

const props = defineProps<{ items: readonly ScriptRunRestoreItem[] }>();
const emit = defineEmits<{ close: [] }>();

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

const rows = computed<RestoreRow[]>(() => runRestoreRows(props.items, errors.value));
const openStarted = useOpenRestoredRun();

const callError = computed(() => {
  const err = restore.error.value ?? dismiss.error.value;
  return err ? errorText(err) : undefined;
});

async function submit({ ids }: RestoreConfirm): Promise<void> {
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
    :dismiss-label="t('common.action.dontRestore.label')"
    :call-error="callError"
    @confirm="submit"
    @dismiss="dismissAll"
    @close="emit('close')"
  />
</template>
