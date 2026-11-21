<script setup lang="ts">
import { computed } from 'vue';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import type { CardModel } from './usePlanModel';

// The Jira key as muted mono text after the title; hidden while the title is renamed so the input
// gets the room.
const props = defineProps<{ card: CardModel }>();

const ui = useAdeBoardUiStore();
const editing = computed(
  () => ui.rowEdit?.taskId === props.card.task.id && ui.rowEdit.field === 'title',
);
</script>

<template>
  <span
    v-if="card.issueKey && !editing"
    class="shrink-0 whitespace-nowrap font-data text-kira-sm text-subtle"
    data-testid="ade-task-key"
    >{{
      card.issueKey
    }}</span
  >
</template>
