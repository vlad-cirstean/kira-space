<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { errorText } from '@workbench/bridge/codedError';
import { ref } from 'vue';
import { taskPatch } from '../board/panelFacts';
import AdeHandoverList from '../notes/AdeHandoverList.vue';
import AdeNotesEditor from '../notes/AdeNotesEditor.vue';
import { useUpdateTask } from '../queries';
import type { Task } from '../wire';
import AdeLinkGroups from './AdeLinkGroups.vue';
import AdeTaskTimeline from './AdeTaskTimeline.vue';
import type { TimelineBranch } from './taskTimeline';

// Details tab: notes, links, handovers and the timeline. Archived tasks open it read-only.
const props = defineProps<{
  task: Task;
  branches: readonly TimelineBranch[];
  readonly?: boolean;
}>();

const update = useUpdateTask();
const notesError = ref('');

async function saveNotes(taskId: string, value: string): Promise<void> {
  notesError.value = '';
  try {
    await update.mutateAsync({ taskId, patch: taskPatch({ notes: value }) });
  } catch (err) {
    notesError.value = errorText(err);
  }
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-3" data-testid="ade-details-tab">
    <AdeNotesEditor :notes="task.notes" :item-id="task.id" :readonly="readonly" @save="saveNotes" />
    <Alert v-if="notesError" variant="error" data-testid="ade-notes-error"
      ><AlertDescription>{{ notesError }}</AlertDescription></Alert
    >
    <AdeLinkGroups :task="props.task" :readonly="readonly" />
    <AdeHandoverList :task-id="task.id" :readonly="readonly" />
    <AdeTaskTimeline :task="task" :branches="branches" />
  </div>
</template>
