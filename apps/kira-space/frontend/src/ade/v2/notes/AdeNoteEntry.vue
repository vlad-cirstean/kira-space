<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { formatRelative } from '@workbench/behaviour/format';
import { ref, watch } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeOriginSession from '../details/AdeOriginSession.vue';
import { useUpdateHandover } from '../queries';
import { useAdeStageDialogStore } from '../state/adeStageDialog';
import type { Handover } from '../wire';
import AdeNotesEditor from './AdeNotesEditor.vue';
import { entryTitle, noteAuthor, noteStage } from './noteChain';

// One note or handover of the task chain. It edits in place while `editing` is set; the chain keeps
// the one open edit.
const props = defineProps<{
  handover: Handover;
  taskId: string;
  readonly?: boolean;
  editing: boolean;
}>();
const emit = defineEmits<{ edit: []; done: []; openSession: [sessionId: string] }>();
const stage = useAdeStageDialogStore();
const update = useUpdateHandover();

const text = ref('');
watch(
  () => props.editing,
  (on) => {
    if (on) text.value = props.handover.body;
  },
  { immediate: true },
);
const saving = useAsyncAction(async () => {
  if (text.value !== props.handover.body) {
    await update.mutateAsync({ id: props.handover.id, body: text.value });
  }
  emit('done');
});

function time(h: Handover): string {
  return formatRelative(h.state === 'draft' ? h.updatedAt : h.createdAt);
}
</script>

<template>
  <Card
    class="flex flex-col gap-1 p-2 text-kira-md"
    data-kind="handover"
    :data-author="noteAuthor(handover)"
    :data-testid="
      handover.state === 'draft'
        ? 'ade-handover-draft'
        : handover.state === 'note'
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
            noteAuthor(handover) === 'user'
              ? 'space.ade.notes.author.user.aria'
              : 'space.ade.notes.author.agent.aria',
          )
        "
        data-testid="ade-note-author"
      >
        <CodiconIcon :name="noteAuthor(handover) === 'user' ? 'account' : 'hubot'" :size="12" />
        <AdeChip
          :label="
            noteAuthor(handover) === 'user'
              ? t('common.picker.you.label')
              : t('memory.common.origin.agent.label')
          "
          tone="grey"
        />
      </span>
      <span class="font-medium" data-testid="ade-handover-title">{{ entryTitle(handover) }}</span>
      <span v-if="noteStage(handover)" class="text-muted-foreground" data-testid="ade-note-stage">{{
        noteStage(handover)
      }}</span>
      <AdeChip v-if="handover.source === 'run'" :label="t('common.action.run.label')" tone="grey" />
      <AdeChip v-if="handover.state === 'draft'" :label="t('common.draft.status')" tone="amber" />
      <AdeChip
        v-if="handover.state === 'draft' && handover.finished"
        :label="t('space.ade.handover.finished.status')"
        tone="green"
      />
      <AdeChip
        v-if="handover.edited && handover.state !== 'note'"
        :label="t('llmPrompts.badge.edited.label')"
        tone="grey"
      />
      <span class="ml-auto text-subtle">{{ time(handover) }}</span>
      <AdeOriginSession
        :task-id="taskId"
        :session-id="handover.sessionId"
        :archived="readonly"
        @open="emit('openSession', $event)"
      />
      <TooltipIconButton
        v-if="!readonly && !editing"
        icon="edit"
        :label="t('space.ade.handover.edit.tooltip')"
        data-testid="ade-handover-edit"
        @click="emit('edit')"
      />
    </div>
    <template v-if="editing">
      <AdeNotesEditor
        :key="`edit-${handover.id}`"
        v-model="text"
        compact
        focus-on-mount
        :hard-breaks="noteAuthor(handover) === 'agent'"
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
          @click="emit('done')"
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
      :key="`view-${handover.id}`"
      :model-value="handover.body"
      readonly
      :hard-breaks="noteAuthor(handover) === 'agent'"
      data-testid="ade-handover-body"
    />
    <div v-if="handover.state === 'draft' && !readonly && !editing" class="flex justify-end">
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
</template>
