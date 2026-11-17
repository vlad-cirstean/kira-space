<script setup lang="ts">
import { compareNatural } from '@workbench/behaviour/match';
import { loadMonaco } from '@workbench/editor/monaco';
import { windowKey } from '@workbench/lib/window';
import {
  usePrompts,
  usePromptsControl,
  usePromptVisibility,
} from '@workbench/prompts/promptsQueries';
import { useApprovalsStore } from '@workbench/state/approvals';
import { useGatedOpen } from '@workbench/state/promptGate';
import { computed, onScopeDispose, watch } from 'vue';
import ApprovalsDialog from './ApprovalsDialog.vue';
import ApprovalsToast from './ApprovalsToast.vue';
import { useApprovals } from './approvalsQueries';
import { useApprovalKinds } from './registry';

// One dialog for every agent approval routed to this window. Closing it minimises to a toast and
// never answers; a prompt (quit, update, credential) on screen turns the dialog into the toast so
// two modals never stack.
const PREFIX = 'approval:';

const store = useApprovalsStore();
const { data: items } = useApprovals();
const { data: prompts } = usePrompts();
const { shown: promptShown } = usePromptVisibility();
const kinds = useApprovalKinds();
onScopeDispose(
  usePromptsControl().onPromptsReveal(({ id }) => {
    if (id.startsWith(PREFIX)) store.open(id.slice(PREFIX.length));
  }),
);

const mine = computed(() => {
  const ids = new Set(
    (prompts.value ?? [])
      .filter((p) => p.kind === 'approval' && p.target === windowKey)
      .map((p) => p.ref),
  );
  return (items.value ?? [])
    .filter((i) => ids.has(i.id))
    .sort((a, b) => a.createdAt - b.createdAt || compareNatural(a.id, b.id));
});

// The dialog waits for the user to stop typing; the toast stands in meanwhile.
const wantDialog = computed(() => mine.value.length > 0 && !store.minimised && !promptShown.value);
const gate = useGatedOpen(wantDialog);
const dialogOpen = computed(() => wantDialog.value && gate.ready.value);
const deferred = computed(() => wantDialog.value && !gate.ready.value);

function show(): void {
  store.open();
  gate.force();
}

// Statements and bodies of some kinds render in Monaco: load it while the dialog opens, not after.
// Only for those kinds: Monaco and its worker cost tens of MB for the app's life.
watch(
  () => mine.value.some((i) => kinds[i.kind]?.monaco),
  (needs) => needs && void loadMonaco(),
  { immediate: true },
);

watch(
  () => (items.value ?? []).map((i) => i.id),
  (ids) => store.prune(ids),
);
</script>

<template>
  <template v-if="mine.length > 0">
    <ApprovalsDialog v-if="dialogOpen" :items="mine" />
    <ApprovalsToast v-else :items="mine" :deferred="deferred" @show="show" />
  </template>
</template>
