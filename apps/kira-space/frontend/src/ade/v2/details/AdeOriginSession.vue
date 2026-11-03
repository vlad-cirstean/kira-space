<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useSessions } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';

// Opens the session that reported an item. Hidden for user adds and sessions the board no longer lists.
// An archived task has no board selection to move, so it hands the id up instead.
const props = defineProps<{ taskId: string; sessionId: string; archived?: boolean }>();
const emit = defineEmits<{ open: [sessionId: string] }>();

const ui = useAdeBoardUiStore();
const sessions = useSessions();
const known = computed(
  () =>
    props.sessionId !== '' &&
    (sessions.data.value?.sessions ?? []).some((s) => s.id === props.sessionId),
);

function open(): void {
  if (props.archived) emit('open', props.sessionId);
  else ui.openSession({ taskId: props.taskId, branchId: '', sessionId: props.sessionId });
}
</script>

<template>
  <TooltipIconButton
    v-if="known"
    icon="terminal"
    :label="t('space.ade.details.session.tooltip')"
    class="shrink-0"
    data-testid="ade-origin-session"
    @click="open"
  />
</template>
