<script setup lang="ts">
import TruncatedText from '@theme/components/TruncatedText.vue';
import { errorText } from '@workbench/bridge/codedError';
import InlineEditInput from '@workbench/components/InlineEditInput.vue';
import { computed } from 'vue';
import { taskPatch } from '../board/panelFacts';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import type { CardModel } from './usePlanModel';
import { useTaskWrites } from './useTaskWrites';

// The task title on its row; double-click renames in place (the task menu's Rename sets the same
// flag). A blank name cancels: clearing a Jira task's title stays a panel action.
const props = defineProps<{ card: CardModel }>();

const ui = useAdeBoardUiStore();
const { update } = useTaskWrites();
const editing = computed(
  () => ui.rowEdit?.taskId === props.card.task.id && ui.rowEdit.field === 'title',
);

function cancel(): void {
  ui.rowEdit = null;
}

async function commit(title: string): Promise<void> {
  const taskId = props.card.task.id;
  ui.rowEdit = null;
  delete ui.actionError[taskId];
  try {
    await update.mutateAsync({ taskId, patch: taskPatch({ title }) });
  } catch (err) {
    ui.select(taskId);
    ui.actionError[taskId] = errorText(err);
  }
}
</script>

<template>
  <InlineEditInput
    v-if="editing"
    :name="card.task.title"
    class="h-4.5 text-kira-lg font-medium"
    data-testid="ade-card-title-input"
    @commit="commit"
    @cancel="cancel"
  />
  <!-- biome-ignore lint/a11y/noStaticElementInteractions: double-click is the mouse shortcut; the task menu's Rename is the keyboard path. -->
  <span
    v-else
    class="flex min-w-0 text-kira-lg font-medium"
    :class="card.parked ? 'text-muted-foreground' : 'text-fg'"
    data-testid="ade-card-title"
    @dblclick.stop="ui.rowEdit = { taskId: card.task.id, field: 'title' }"
  >
    <TruncatedText :text="card.title" />
  </span>
</template>
