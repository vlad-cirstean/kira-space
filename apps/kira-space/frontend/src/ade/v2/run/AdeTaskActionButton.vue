<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { computed } from 'vue';
import AdeTip from '../AdeTip.vue';
import type { CardModel } from '../plan/usePlanModel';
import { ACTION_CLASS } from '../tones';
import { useTaskAction } from './useTaskAction';

// `quiet` is the Plan card's neutral look. The task's step action: `▶ Run`, `▶ <Step>`, `Resume <Step>`, `Take over`, `Approve`, `Retry`, `Next stage ›`, `Finish`,
// `Archive`; nothing otherwise.
const props = defineProps<{ card: CardModel; quiet?: boolean }>();
const { action, perform, busy } = useTaskAction(() => props.card);
const variant = computed(() =>
  props.quiet && action.value?.tone === 'red' ? 'dialog-danger' : 'dialog',
);
</script>

<template>
  <AdeTip v-if="action" :text="action.tip">
    <Button
      :variant="variant"
      size="kira-lg"
      class="shrink-0"
      :class="quiet ? '' : ACTION_CLASS[action.tone]"
      :pending="busy"
      :data-testid="`ade-task-action-${action.kind}`"
      @click.stop="perform"
    >
      {{ action.label }}
    </Button>
  </AdeTip>
</template>
