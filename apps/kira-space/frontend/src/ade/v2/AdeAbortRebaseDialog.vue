<script setup lang="ts">
import { t } from '@theme/i18n';
import ConfirmDialogView from '@workbench/components/ConfirmDialogView.vue';
import { useAdeDialogsStore } from './state/adeDialogs';
import { useConfirmRun } from './useConfirmRun';

// Abort rebase asks first: it throws away the half-done rebase in the worktree.
const dialogs = useAdeDialogsStore();
const { busy, error, confirm } = useConfirmRun(
  () => dialogs.abortTarget !== null,
  dialogs.confirmAbort,
  () => {
    dialogs.abortTarget = null;
  },
);
</script>

<template>
  <ConfirmDialogView
    :open="dialogs.abortTarget !== null"
    :title="t('space.ade.abortConfirm.title')"
    :message="t('space.ade.abortConfirm.body', { branch: dialogs.abortTarget?.name ?? '' })"
    :confirm-label="t('common.abort.label')"
    tone="danger"
    :pending="busy"
    :error="error ?? ''"
    :testids="{
      root: 'ade-confirm-dialog',
      close: 'ade-confirm-close',
      confirm: 'ade-confirm-yes',
      error: 'ade-confirm-error',
    }"
    @confirm="confirm"
    @cancel="dialogs.abortTarget = null"
  />
</template>
