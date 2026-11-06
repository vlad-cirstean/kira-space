<script setup lang="ts">
import AdeNotesChain from '../notes/AdeNotesChain.vue';
import type { Task } from '../wire';
import AdeLinkGroups from './AdeLinkGroups.vue';
import AdeTaskTimeline from './AdeTaskTimeline.vue';
import type { TimelineBranch } from './taskTimeline';

// Details tab: the notes chain, links and the timeline. Archived tasks open it read-only.
const props = defineProps<{
  task: Task;
  branches: readonly TimelineBranch[];
  readonly?: boolean;
}>();

const emit = defineEmits<{ openSession: [sessionId: string] }>();
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-3" data-testid="ade-details-tab">
    <AdeNotesChain
      :task-id="task.id"
      :readonly="readonly"
      @open-session="emit('openSession', $event)"
    />
    <AdeLinkGroups
      :task="props.task"
      :readonly="readonly"
      @open-session="emit('openSession', $event)"
    />
    <AdeTaskTimeline
      :task="task"
      :branches="branches"
      :archived="readonly"
      @open-session="emit('openSession', $event)"
    />
  </div>
</template>
