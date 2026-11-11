<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import AdeActivityIcon from '../AdeActivityIcon.vue';
import AdeTip from '../AdeTip.vue';
import type { TaskSessions } from '../board/taskSessions';
import { useAdeBoardUiStore } from '../state/adeBoardUi';

// Card indicator of the task's agent sessions: needs input, working, finished not viewed, or all
// finished. Click opens
// the task's Sessions tab.
const props = defineProps<{ taskId: string; sessions: TaskSessions }>();
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
      class="h-4.5 min-w-4.5 shrink-0 gap-0.5 rounded-full border-0 px-0.5"
      :data-state="sessions.state"
      data-testid="ade-card-sessions"
      @click.stop="ui.openSessions(taskId)"
    >
      <AdeActivityIcon v-if="sessions.state === 'input'" kind="input" bare />
      <AdeActivityIcon v-else-if="sessions.state === 'unseen'" kind="unseen" bare />
      <template v-else-if="sessions.state === 'running'">
        <AdeActivityIcon kind="working" bare />
        <span v-if="sessions.running > 1" class="text-kira-sm tabular-nums">{{
          sessions.running
        }}</span>
      </template>
      <CodiconIcon v-else name="check" :size="12" class="text-tone-green" />
    </Button>
  </AdeTip>
</template>
