<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import AdeTip from '../AdeTip.vue';
import { TASK_ACTION_ICON } from '../plan/glyphs';
import type { CardModel } from '../plan/usePlanModel';
import { ACTION_CLASS } from '../tones';
import { useTaskAction } from './useTaskAction';

// The task's step action: `▶ Run`, `▶ <Step>`, `Resume <Step>`, `Take over`, `Approve`, `Retry`,
// `Next stage ›`, `Finish`, `Archive`; nothing otherwise. `icon` renders it as a neutral icon
// button (the Plan card header); the label moves to the tooltip.
const props = defineProps<{ card: CardModel; icon?: boolean }>();
const { action, perform, busy } = useTaskAction(() => props.card);
</script>

<template>
  <template v-if="action">
    <TooltipIconButton
      v-if="icon"
      :icon="TASK_ACTION_ICON[action.kind]"
      :label="action.label"
      :variant="action.tone === 'red' ? 'dialog-danger' : 'dialog'"
      size="kira-icon"
      class="shrink-0"
      :pending="busy"
      :data-testid="`ade-task-action-${action.kind}`"
      @click.stop="perform"
    >
      <div class="font-medium">{{ action.label }}</div>
      <div>{{ action.tip }}</div>
    </TooltipIconButton>
    <AdeTip v-else :text="action.tip">
      <Button
        variant="dialog"
        size="kira-lg"
        class="shrink-0"
        :class="ACTION_CLASS[action.tone]"
        :pending="busy"
        :data-testid="`ade-task-action-${action.kind}`"
        @click.stop="perform"
      >
        {{ action.label }}
      </Button>
    </AdeTip>
  </template>
</template>
