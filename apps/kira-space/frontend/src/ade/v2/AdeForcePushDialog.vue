<script setup lang="ts">
import { t } from '@theme/i18n';
import ConfirmDialogView from '@workbench/components/ConfirmDialogView.vue';
import { type ForcePushTarget, throwOnRemoteError } from './board/panelFacts';
import { useForcePush } from './queries';
import { useAdeOpsStore } from './state/adeOps';

// Force push with lease for one branch. Confirm closes the dialog; the push runs as the branch's op
// (chip, toast past 2 s, error toast with the backend's refusal such as a stale lease).
const props = defineProps<{ target: ForcePushTarget | null }>();
const emit = defineEmits<{ close: [] }>();

const forcePush = useForcePush();
const ops = useAdeOpsStore();

function confirm(): void {
  const target = props.target;
  if (!target) return;
  emit('close');
  void ops.run(
    {
      id: `forcePush:${target.branchId}`,
      kind: 'forcePush',
      taskId: target.taskId || undefined,
      branchId: target.branchId,
      name: target.branchName,
    },
    async () => throwOnRemoteError(await forcePush.mutateAsync({ branchId: target.branchId })),
  );
}
</script>

<template>
  <ConfirmDialogView
    :open="target !== null"
    :title="t('space.ade.forcePushConfirm.title')"
    :message="
      t('space.ade.forcePushConfirm.body', {
        branch: target?.branchName ?? '',
        repo: target?.repo ?? '',
      })
    "
    :confirm-label="t('common.forcePush.label')"
    tone="danger"
    :testids="{
      root: 'ade-confirm-dialog',
      cancel: 'ade-confirm-no',
      confirm: 'ade-confirm-yes',
      error: 'ade-confirm-error',
    }"
    @confirm="confirm"
    @cancel="emit('close')"
  />
</template>
