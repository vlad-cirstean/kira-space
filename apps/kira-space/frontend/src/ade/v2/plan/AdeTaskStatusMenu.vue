<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { computed } from 'vue';
import AdeChip from '../AdeChip.vue';
import { taskPatch } from '../board/panelFacts';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { STATUS_TEXT_CLASS } from '../statusColors';
import { type CardModel, usePlanModel } from './usePlanModel';
import { useTaskWrites } from './useTaskWrites';

// The task's manual status as small text on its row; click opens the status set. No tooltip: a
// Tooltip between DropdownMenu and its trigger steals the popper anchor; aria-label names it.
const props = defineProps<{ card: CardModel }>();

const ui = useAdeBoardUiStore();
const { model } = usePlanModel();
const { update } = useTaskWrites();
const statuses = computed(() => model.value?.board.statuses ?? []);

async function pick(value: unknown): Promise<void> {
  const statusId = String(value);
  if (statusId === props.card.status.id) return;
  const taskId = props.card.task.id;
  delete ui.actionError[taskId];
  try {
    await update.mutateAsync({ taskId, patch: taskPatch({ statusId }) });
  } catch (err) {
    ui.select(taskId);
    ui.actionError[taskId] = errorText(err);
  }
}
</script>

<template>
  <DropdownMenu :modal="false">
    <DropdownMenuTrigger as-child>
      <Button
        type="button"
        variant="toolbar"
        size="inline"
        class="h-4.5 max-w-24 shrink-0 justify-start truncate rounded-kira-sm border-0 px-1 text-kira-sm font-normal"
        :class="STATUS_TEXT_CLASS[card.status.color]"
        :aria-label="t('space.ade.plan.card.status.aria', { status: card.status.name })"
        :data-status-id="card.status.id"
        data-testid="ade-row-status"
        @click.stop
      >
        {{ card.status.name }}
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="start" data-testid="ade-row-status-menu" @click.stop>
      <DropdownMenuLabel>{{ t('common.field.status.label') }}</DropdownMenuLabel>
      <DropdownMenuRadioGroup :model-value="card.status.id" @update:model-value="pick">
        <DropdownMenuRadioItem
          v-for="s in statuses"
          :key="s.id"
          :value="s.id"
          :data-testid="`ade-row-status-option-${s.id}`"
        >
          <AdeChip :label="s.name" :color="s.color" />
          <template #indicator-icon><CodiconIcon name="check" :size="13" /></template>
        </DropdownMenuRadioItem>
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
