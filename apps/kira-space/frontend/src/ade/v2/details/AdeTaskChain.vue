<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { formatRelative } from '@workbench/behaviour/format';
import { computed, ref } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeOriginSession from '../details/AdeOriginSession.vue';
import { useAddNote, useHandovers, useUpdateHandover } from '../queries';
import { useAdeNoteComposerStore } from '../state/adeNoteComposer';
import { useAdeStageDialogStore } from '../state/adeStageDialog';
import type { Handover } from '../wire';
import AdeNotesEditor from './AdeNotesEditor.vue';
import { entryTitle, noteAuthor, noteStage } from './noteChain';

// The task's notes chain: the user's notes and what each stage left for the next, one list, newest
// first. Every entry is Markdown; an entry edits in place unless the task is archived.
const props = defineProps<{ taskId: string; readonly?: boolean }>();
const emit = defineEmits<{ openSession: [sessionId: string] }>();
const stage = useAdeStageDialogStore();
const composer = useAdeNoteComposerStore();
const { data } = useHandovers(() => props.taskId);
const update = useUpdateHandover();
const add = useAddNote();

const entries = computed(() => data.value?.handovers ?? []);
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
const text = ref('');
function edit(h: Handover): void {
  editing.value = h.id;
  text.value = h.body;
}
const saving = useAsyncAction(async () => {
  const id = editing.value;
  if (!id) return;
  const body = entries.value.find((h) => h.id === id)?.body;
  if (text.value !== body) await update.mutateAsync({ id, body: text.value });
  editing.value = null;
});

function time(h: Handover): string {
  return formatRelative(h.state === 'draft' ? h.updatedAt : h.createdAt);
}
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="ade-notes-chain">
    <div class="flex items-center gap-2 text-kira-sm text-muted-foreground">
      <span>{{ t('space.common.notes.label') }}</span>
      <TooltipIconButton
        v-if="!readonly && !composing"
        class="ml-auto"
        icon="add"
        :label="t('space.ade.notes.add.tooltip')"
        data-testid="ade-note-add"
        @click="composer.show(taskId)"
      />
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
    <Card
      v-for="h in entries"
      :key="h.id"
      class="flex flex-col gap-1 p-2 text-kira-md"
      :data-author="noteAuthor(h)"
      :data-testid="
        h.state === 'draft'
          ? 'ade-handover-draft'
          : h.state === 'note'
            ? 'ade-note-row'
            : 'ade-handover-row'
      "
    >
      <div class="flex items-center gap-2 text-kira-sm">
        <span
          class="inline-flex items-center gap-1"
          role="img"
          :aria-label="
            t(
              noteAuthor(h) === 'user'
                ? 'space.ade.notes.author.user.aria'
                : 'space.ade.notes.author.agent.aria',
            )
          "
          data-testid="ade-note-author"
        >
          <CodiconIcon :name="noteAuthor(h) === 'user' ? 'account' : 'hubot'" :size="12" />
          <AdeChip
            :label="
              noteAuthor(h) === 'user'
                ? t('common.picker.you.label')
                : t('memory.common.origin.agent.label')
            "
            tone="grey"
          />
        </span>
        <span class="font-medium" data-testid="ade-handover-title">{{ entryTitle(h) }}</span>
        <span v-if="noteStage(h)" class="text-muted-foreground" data-testid="ade-note-stage">{{
          noteStage(h)
        }}</span>
        <AdeChip v-if="h.source === 'run'" :label="t('common.action.run.label')" tone="grey" />
        <AdeChip v-if="h.state === 'draft'" :label="t('common.draft.status')" tone="amber" />
        <AdeChip
          v-if="h.state === 'draft' && h.finished"
          :label="t('space.ade.handover.finished.status')"
          tone="green"
        />
        <AdeChip
          v-if="h.edited && h.state !== 'note'"
          :label="t('llmPrompts.badge.edited.label')"
          tone="grey"
        />
        <span class="ml-auto text-subtle">{{ time(h) }}</span>
        <AdeOriginSession
          :task-id="taskId"
          :session-id="h.sessionId"
          :archived="readonly"
          @open="emit('openSession', $event)"
        />
        <TooltipIconButton
          v-if="!readonly && editing !== h.id"
          icon="edit"
          :label="t('space.ade.handover.edit.tooltip')"
          data-testid="ade-handover-edit"
          @click="edit(h)"
        />
      </div>
      <template v-if="editing === h.id">
        <AdeNotesEditor
          :key="`edit-${h.id}`"
          v-model="text"
          compact
          focus-on-mount
          :hard-breaks="noteAuthor(h) === 'agent'"
          data-testid="ade-handover-text"
          @submit="saving.run"
        />
        <Alert v-if="saving.error.value" variant="error" data-testid="ade-handover-error"
          ><AlertDescription>{{ saving.error.value }}</AlertDescription></Alert
        >
        <div class="flex justify-end gap-2">
          <Button
            variant="dialog"
            size="kira"
            data-testid="ade-handover-cancel"
            @click="editing = null"
            >{{
              t('common.action.cancel.label')
            }}</Button
          >
          <Button
            variant="dialog-primary"
            size="kira"
            :pending="saving.pending.value"
            data-testid="ade-handover-save"
            @click="saving.run"
            >{{
              t('common.action.save.label')
            }}</Button
          >
        </div>
      </template>
      <AdeNotesEditor
        v-else
        :key="`view-${h.id}`"
        :model-value="h.body"
        readonly
        :hard-breaks="noteAuthor(h) === 'agent'"
        data-testid="ade-handover-body"
      />
      <div v-if="h.state === 'draft' && !readonly && editing !== h.id" class="flex justify-end">
        <Button
          variant="dialog"
          size="kira"
          data-testid="ade-handover-move-on"
          @click="stage.open({ taskId })"
          >{{
            t('space.ade.handover.moveOn.label')
          }}</Button
        >
      </div>
    </Card>
  </div>
</template>
