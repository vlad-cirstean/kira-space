<script setup lang="ts">
import { t } from '@theme/i18n';
import ConfirmDialogView from '@workbench/components/ConfirmDialogView.vue';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
import { useConfirmRun } from '../useConfirmRun';

const takeOver = useAdeTakeOverStore();
const { busy, error, confirm } = useConfirmRun(
  () => takeOver.confirm !== null,
  takeOver.confirmYes,
  () => takeOver.cancel(),
);
</script>

<template>
  <ConfirmDialogView
    :open="takeOver.confirm !== null"
    :title="t('space.common.takeOverConfirm.title')"
    :message="takeOver.confirm?.text ?? ''"
    :confirm-label="t('common.action.takeOver.label')"
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
    @cancel="takeOver.cancel"
  />
</template>
