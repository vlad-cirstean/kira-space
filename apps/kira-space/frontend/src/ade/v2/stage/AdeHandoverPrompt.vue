<script setup lang="ts">
import type { RoutedPrompt } from '@shared/domain/prompts';
import { useTimeout } from '@vueuse/core';
import { computed, onBeforeUnmount, watch } from 'vue';
import { useHandovers, useTaskArtifacts } from '../queries';
import { useAdeStageDialogStore } from '../state/adeStageDialog';
import AdeStageDialog from './AdeStageDialog.vue';

// The routed `handover` popup: a session finished its stage and offers the move. `entry.ref` is the
// task id. Not now keeps the draft and withdraws the popup; Escape and Cancel only hide it here.
const props = defineProps<{ entry: RoutedPrompt; more: number }>();
const emit = defineEmits<{ hide: [] }>();
const store = useAdeStageDialogStore();

// The dialog opens once, with the text and link strip in its first frame. The board snapshot can
// trail the prompt, so the draft comes from the DB-only handovers read. Artifacts can be slow
// (issue tracker): wait for them only briefly, never fail on them.
const handovers = useHandovers(() => props.entry.ref);
const artifacts = useTaskArtifacts(() => props.entry.ref);
const ARTIFACTS_GRACE_MS = 400;
const { ready: graceOver } = useTimeout(ARTIFACTS_GRACE_MS, { controls: true });
const ready = computed(
  () =>
    handovers.isSuccess.value &&
    !handovers.isFetching.value &&
    (artifacts.isFetched.value || graceOver.value),
);

watch(
  ready,
  (r) => {
    if (!r) return;
    const seed = handovers.data.value?.handovers.find((h) => h.state === 'draft' && !h.dismissed);
    store.open({ taskId: props.entry.ref, routed: true, seed });
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  if (store.routed && store.taskId === props.entry.ref) store.close();
});

async function notNow(): Promise<void> {
  await store.dismiss();
  emit('hide');
}
</script>

<template>
  <AdeStageDialog v-if="ready" routed @close="emit('hide')" @not-now="notNow" />
</template>
