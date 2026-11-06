<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { formatRelative } from '@workbench/behaviour/format';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { control } from '../../../bridge/control';
import AdeRow from '../panel/AdeRow.vue';
import AdeOriginSession from './AdeOriginSession.vue';
import type { TimelineEvent } from './taskTimeline';

// One task event in the Details list: slimmer than a note or proof card. Read-only.
const props = defineProps<{ event: TimelineEvent; taskId: string; archived?: boolean }>();
const emit = defineEmits<{ openSession: [sessionId: string] }>();

const ICONS = { created: 'add', branch: 'git-branch', artifact: 'link' } as const;

function openLink(e: MouseEvent): void {
  e.preventDefault();
  fireAndForget(control.openExternal(props.event.url), 'ade.timeline');
}
</script>

<template>
  <AdeRow
    as="div"
    :label="formatRelative(event.at)"
    data-testid="ade-details-timeline-entry"
    data-kind="event"
    :data-event="event.type"
  >
    <template #lead><CodiconIcon :name="ICONS[event.type]" :size="13" /></template>
    <a v-if="event.url" :href="event.url" class="min-w-0 truncate text-info" @click="openLink">{{
      event.label
    }}</a>
    <span v-else class="min-w-0 truncate">{{ event.label }}</span>
    <template #actions>
      <AdeOriginSession
        :task-id="taskId"
        :session-id="event.sessionId"
        :archived="archived"
        @open="emit('openSession', $event)"
      />
    </template>
  </AdeRow>
</template>
