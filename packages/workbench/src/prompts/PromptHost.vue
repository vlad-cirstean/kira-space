<script setup lang="ts">
import type { PromptKind, RoutedPrompt } from '@shared/domain/prompts';
import { compareNatural } from '@workbench/behaviour/match';
import { windowKey } from '@workbench/lib/window';
import { type Component, computed, onScopeDispose, watch, watchEffect } from 'vue';
import { usePrompts, usePromptsControl, usePromptVisibility } from './promptsQueries';

// Shows the popup the router targeted at this window: the oldest, or the one a notification click
// asked for. Each kind's component answers through its own owner; this host only picks which one.
const props = defineProps<{ kinds: Partial<Record<PromptKind, Component>> }>();

const control = usePromptsControl();
const { data } = usePrompts();
const { hidden, revealed, shown, hide, reveal, prune } = usePromptVisibility();
onScopeDispose(control.onPromptsReveal(({ id }) => reveal(id)));

watch(
  () => (data.value ?? []).map((p) => p.id),
  (ids) => prune(ids),
);

const visible = computed<RoutedPrompt[]>(() =>
  (data.value ?? [])
    .filter((p) => p.target === windowKey && props.kinds[p.kind] && !hidden.value.includes(p.id))
    .sort((a, b) => a.createdAt - b.createdAt || compareNatural(a.id, b.id)),
);
const head = computed(() => visible.value.find((p) => p.id === revealed.value) ?? visible.value[0]);
watchEffect(() => {
  shown.value = !!head.value;
});
onScopeDispose(() => {
  shown.value = false;
});
const more = computed(() => Math.max(0, visible.value.length - 1));
</script>

<template>
  <component
    :is="kinds[head.kind]"
    v-if="head"
    :key="head.id"
    :entry="head"
    :more="more"
    @hide="hide(head.id)"
  />
</template>
