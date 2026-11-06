<script setup lang="ts">
import type { Task } from '../wire';
import AdeLinkGroups from './AdeLinkGroups.vue';
import AdeTaskChain from './AdeTaskChain.vue';
import AdeTaskTimeline from './AdeTaskTimeline.vue';
import type { TimelineBranch } from './taskTimeline';

// Details tab: the links, the task chain (notes, handovers, proof) and the timeline. Archived tasks open it read-only.
const props = defineProps<{
  task: Task;
  branches: readonly TimelineBranch[];
  readonly?: boolean;
}>();

const emit = defineEmits<{ openSession: [sessionId: string] }>();
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-3" data-testid="ade-details-tab">
    <AdeLinkGroups
      :task="props.task"
      :readonly="readonly"
      @open-session="emit('openSession', $event)"
    />
    <AdeTaskChain
      :task-id="task.id"
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
