<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import AdeActivityIcon from '../AdeActivityIcon.vue';
import AdeTip from '../AdeTip.vue';
import type { TaskSessions } from '../board/taskSessions';
import { useAdeBoardUiStore } from '../state/adeBoardUi';

// Card indicator of the task's agent sessions: needs input, working, finished not viewed, or all
// finished. Click opens
// the task's Sessions tab.
const props = withDefaults(
  defineProps<{ taskId: string; sessions: TaskSessions; size?: 'md' | 'lg' }>(),
  { size: 'md' },
);
// `lg`: the card head's two-line lead column.
const SIZE = {
  md: { button: 'h-4.5 min-w-4.5 px-0.5', icon: 12, count: 'text-kira-sm' },
  lg: { button: 'min-h-6 min-w-6 px-1', icon: 16, count: 'text-kira-md' },
} as const;
const sz = computed(() => SIZE[props.size]);
const ui = useAdeBoardUiStore();

function tip(): string {
  const { state, running } = props.sessions;
  if (state === 'input') return t('space.ade.plan.card.sessionsInput.tooltip');
  if (state === 'unseen') return t('space.ade.plan.card.sessionsUnseen.tooltip');
  if (state === 'running')
    return t('space.ade.plan.card.sessionsRunning.tooltip', { count: running });
  return t('space.ade.plan.card.sessionsFinished.tooltip');
}
</script>

<template>
  <AdeTip :text="tip()">
    <Button
      size="inline"
      type="button"
      variant="toolbar"
      :aria-label="tip()"
      class="shrink-0 gap-0.5 rounded-full border-0"
      :class="sz.button"
      :data-state="sessions.state"
      data-testid="ade-card-sessions"
      @click.stop="ui.openSessions(taskId)"
    >
      <AdeActivityIcon v-if="sessions.state === 'input'" kind="input" :size="sz.icon" bare />
      <AdeActivityIcon v-else-if="sessions.state === 'unseen'" kind="unseen" :size="sz.icon" bare />
      <template v-else-if="sessions.state === 'running'">
        <AdeActivityIcon kind="working" :size="sz.icon" bare />
        <span v-if="sessions.running > 1" class="tabular-nums" :class="sz.count">{{
          sessions.running
        }}</span>
      </template>
      <CodiconIcon v-else name="check" :size="sz.icon" class="text-tone-green" />
    </Button>
  </AdeTip>
</template>
