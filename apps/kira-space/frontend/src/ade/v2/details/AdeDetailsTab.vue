<script setup lang="ts">
import AdeLinkStrip from '../links/AdeLinkStrip.vue';
import type { Task } from '../wire';
import AdeTaskChain from './AdeTaskChain.vue';
import type { TimelineBranch } from './taskTimeline';

// Details tab: the links, one list of notes, handovers and proof (task events with the Timeline toggle). Archived tasks open it read-only.
const props = defineProps<{
  task: Task;
  branches: readonly TimelineBranch[];
  readonly?: boolean;
}>();

const emit = defineEmits<{ openSession: [sessionId: string] }>();
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-3" data-testid="ade-details-tab">
    <AdeLinkStrip
      :task="props.task"
      :mode="readonly ? 'view' : 'edit'"
      :archived="readonly"
      @open-session="emit('openSession', $event)"
    />
    <AdeTaskChain
      :task="task"
      :branches="branches"
      :readonly="readonly"
      @open-session="emit('openSession', $event)"
    />
  </div>
</template>
