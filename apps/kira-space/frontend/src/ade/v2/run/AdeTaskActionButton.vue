<script setup lang="ts">
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { TASK_ACTION_ICON } from '../plan/glyphs';
import type { CardModel } from '../plan/usePlanModel';
import { usePrefetchRunPreview } from './runPreview';
import { useTaskAction } from './useTaskAction';

// The task's step action: `▶ Run`, `▶ <Step>`, `Resume <Step>`, `Take over`, `Approve`, `Retry`,
// `Next stage ›`, `Finish`, `Archive`; nothing otherwise. `icon` renders it as an icon button
// (the Plan card header); the label moves to the tooltip. Both looks take the action's tone outline
// (Button `outline`).
const props = defineProps<{ card: CardModel; icon?: boolean }>();
const { action, perform, busy } = useTaskAction(() => props.card);
const prefetch = usePrefetchRunPreview();
// Hovering an agent step's Run warms the dialog's previews, so its message is there on open.
function warm(): void {
  if (action.value?.kind === 'run' && props.card.progress.step?.kind === 'agent')
    prefetch(props.card);
}
</script>

<template>
  <template v-if="action">
    <TooltipIconButton
      v-if="icon"
      :icon="TASK_ACTION_ICON[action.kind]"
      :label="action.label"
      variant="dialog"
      :outline="action.tone"
      size="kira-icon"
      class="shrink-0"
      :pending="busy"
      :data-testid="`ade-task-action-${action.kind}`"
      @pointerenter="warm"
      @click.stop="perform"
    >
      <div class="font-medium">{{ action.label }}</div>
      <div>{{ action.tip }}</div>
    </TooltipIconButton>
    <TextTooltip v-else :text="action.tip">
      <Button
        variant="dialog"
        :outline="action.tone"
        size="kira"
        class="shrink-0"
        :pending="busy"
        :data-testid="`ade-task-action-${action.kind}`"
        @pointerenter="warm"
        @click.stop="perform"
      >
        {{ action.label }}
      </Button>
    </TextTooltip>
  </template>
</template>
