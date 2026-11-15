<script setup lang="ts">
import { watch } from 'vue';
import { useFocusReturn } from '../behaviour/focus';
import { useConfirmDialogStore } from '../state/confirmDialog';
import ConfirmDialogView from './ConfirmDialogView.vue';

defineProps<{ touch?: boolean }>();

const confirmDialogStore = useConfirmDialogStore();

// Focus goes back to whatever held it when the dialog opened (a row, a toolbar button).
const focusReturn = useFocusReturn();
watch(
  () => confirmDialogStore.open,
  (open) => {
    if (open) focusReturn.capture();
    else focusReturn.restore();
  },
  { flush: 'sync' },
);
</script>

<template>
  <!-- v-if, not just :open — reka's DialogPortal inserts each open dialog's node at the *end* of
       body at open time, so DOM (and paint) order tracks open order. Bound only by :open, this
       component's Teleport target would instead claim a fixed, early DOM position from app boot
       (ConfirmDialog is always mounted, unlike whatever dialog it confirms over), permanently
       under any dialog opened later — exactly the DbMcpApprovalDialog.vue precedent this mirrors.
       Losing the closing fade (v-if unmounts immediately, no exit transition) is the accepted
       trade-off for correct stacking when nested under another open dialog. -->
  <ConfirmDialogView
    v-if="confirmDialogStore.open"
    :open="true"
    :title="confirmDialogStore.title"
    :message="confirmDialogStore.message"
    :confirm-label="confirmDialogStore.confirmLabel"
    :secondary-label="confirmDialogStore.secondaryLabel"
    :tone="confirmDialogStore.tone"
    :pending="confirmDialogStore.busy"
    :error="confirmDialogStore.error"
    :touch="touch"
    @confirm="confirmDialogStore.acceptConfirm()"
    @secondary="confirmDialogStore.acceptSecondary()"
    @cancel="confirmDialogStore.cancelConfirm()"
  />
</template>
