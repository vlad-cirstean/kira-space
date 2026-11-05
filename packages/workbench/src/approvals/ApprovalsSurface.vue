<script setup lang="ts">
import { compareNatural } from '@workbench/behaviour/match';
import { windowKey } from '@workbench/lib/window';
import {
  usePrompts,
  usePromptsControl,
  usePromptVisibility,
} from '@workbench/prompts/promptsQueries';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed, onScopeDispose, watch } from 'vue';
import ApprovalsDialog from './ApprovalsDialog.vue';
import ApprovalsToast from './ApprovalsToast.vue';
import { useApprovals } from './approvalsQueries';

// One dialog for every agent approval routed to this window. Closing it minimises to a toast and
// never answers; a prompt (quit, update, credential) on screen turns the dialog into the toast so
// two modals never stack.
const PREFIX = 'approval:';

const store = useApprovalsStore();
const { data: items } = useApprovals();
const { data: prompts } = usePrompts();
const { shown: promptShown } = usePromptVisibility();
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

watch(
  () => (items.value ?? []).map((i) => i.id),
  (ids) => store.prune(ids),
);
</script>

<template>
  <template v-if="mine.length > 0">
    <ApprovalsDialog v-if="!store.minimised && !promptShown" :items="mine" />
    <ApprovalsToast v-else :items="mine" />
  </template>
</template>
