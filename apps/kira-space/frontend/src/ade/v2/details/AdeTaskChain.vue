<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed, ref } from 'vue';
import AdeNoteEntry from '../notes/AdeNoteEntry.vue';
import AdeNotesEditor from '../notes/AdeNotesEditor.vue';
import { useAddNote, useHandovers, useTaskProofs } from '../queries';
import { useAdeNoteComposerStore } from '../state/adeNoteComposer';
import AdeProofAttachMenu from './AdeProofAttachMenu.vue';
import AdeProofEntry from './AdeProofEntry.vue';
import { taskChain } from './taskChain';

// The task chain: the user's notes, what each stage left for the next and the proof sessions
// attached to the task, one list, newest first. An entry edits in place unless the task is archived.
const props = defineProps<{ taskId: string; readonly?: boolean }>();
const emit = defineEmits<{ openSession: [sessionId: string] }>();
const composer = useAdeNoteComposerStore();
const handovers = useHandovers(() => props.taskId);
const proofs = useTaskProofs(() => props.taskId);
const add = useAddNote();

const entries = computed(() =>
  taskChain(handovers.data.value?.handovers ?? [], proofs.data.value?.proofs ?? []),
);
const composing = computed(() => !props.readonly && composer.isOpen(props.taskId));
const draft = computed({
  get: () => composer.draftOf(props.taskId),
  set: (v: string) => composer.setDraft(props.taskId, v),
});

const adding = useAsyncAction(async () => {
  if (!draft.value.trim()) return;
  await add.mutateAsync({ taskId: props.taskId, body: draft.value });
  composer.clear(props.taskId);
});

const editing = ref<string | null>(null);
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="ade-task-chain">
    <div class="flex items-center gap-1 text-kira-sm text-muted-foreground">
      <span class="flex-1">{{ t('space.ade.proof.chain.title') }}</span>
      <template v-if="!readonly">
        <TooltipIconButton
          v-if="!composing"
          icon="add"
          :label="t('space.ade.notes.add.tooltip')"
          data-testid="ade-note-add"
          @click="composer.show(taskId)"
        />
        <AdeProofAttachMenu :task-id="taskId" />
      </template>
    </div>
    <Card v-if="composing" class="flex flex-col gap-2 p-2" data-testid="ade-note-composer">
      <AdeNotesEditor
        :key="`compose-${taskId}`"
        v-model="draft"
        compact
        focus-on-mount
        data-testid="ade-note-composer-text"
        @submit="adding.run"
      />
      <Alert v-if="adding.error.value" variant="error" data-testid="ade-note-error"
        ><AlertDescription>{{ adding.error.value }}</AlertDescription></Alert
      >
      <div class="flex justify-end gap-2">
        <Button
          variant="dialog"
          size="kira"
          data-testid="ade-note-cancel"
          @click="composer.clear(taskId)"
          >{{
            t('common.action.cancel.label')
          }}</Button
        >
        <Button
          variant="dialog-primary"
          size="kira"
          :pending="adding.pending.value"
          :disabled="!draft.trim()"
          data-testid="ade-note-save"
          @click="adding.run"
          >{{
            t('common.action.save.label')
          }}</Button
        >
      </div>
    </Card>
    <div
      v-if="!entries.length && !composing"
      class="flex items-center gap-2 text-kira-sm text-subtle"
      data-testid="ade-notes-empty"
    >
      <span>{{ t('space.ade.notes.empty.body') }}</span>
      <Button
        v-if="!readonly"
        variant="dialog"
        size="kira"
        data-testid="ade-note-add-empty"
        @click="composer.show(taskId)"
        >{{
          t('space.ade.notes.add.label')
        }}</Button
      >
    </div>
    <template v-for="e in entries" :key="e.key">
      <AdeNoteEntry
        v-if="e.kind === 'handover'"
        :handover="e.handover"
        :task-id="taskId"
        :readonly="readonly"
        :editing="editing === e.handover.id"
        @edit="editing = e.handover.id"
        @done="editing = null"
        @open-session="emit('openSession', $event)"
      />
      <AdeProofEntry
        v-else
        :proof="e.proof"
        :task-id="taskId"
        :readonly="readonly"
        @open-session="emit('openSession', $event)"
      />
    </template>
  </div>
</template>
