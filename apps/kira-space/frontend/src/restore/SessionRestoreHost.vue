<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { hasReadyItem, useClaimSessionRestore, useSessionRestoreState } from './queries';
import SessionRestoreDialog from './SessionRestoreDialog.vue';

// Opens the restart dialog once per process, in the window that wins the claim. Nothing ready, no
// claim: another window or a later Docker start can still win while the offer window is open.
const state = useSessionRestoreState();
const claim = useClaimSessionRestore();
const shown = ref(false);
const claimed = ref(false);

const offer = computed(() => {
  const s = state.data.value;
  return s?.open && hasReadyItem(s) ? s : null;
});

watch(
  offer,
  async (o) => {
    if (!o || claimed.value) return;
    claimed.value = true;
    try {
      shown.value = await claim.mutateAsync();
    } catch {
      // A failed claim shows nothing; every item stays on its board or banner.
    }
  },
  { immediate: true },
);
</script>

<template>
  <SessionRestoreDialog
    v-if="shown && state.data.value"
    :state="state.data.value"
    @close="shown = false"
  />
</template>
