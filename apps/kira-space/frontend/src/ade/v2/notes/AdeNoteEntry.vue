<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { formatRelative } from '@workbench/behaviour/format';
import CopyIconButton from '@workbench/components/CopyIconButton.vue';
import { computed, ref, watch } from 'vue';
import AdeOriginSession from '../details/AdeOriginSession.vue';
import { useUpdateHandover } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeStageDialogStore } from '../state/adeStageDialog';
import type { Handover } from '../wire';
import AdeNoteBody from './AdeNoteBody.vue';
import { AdeNotesEditor, prefetchNotesEditor } from './asyncEditor';
import {
  AUTHOR_ICON,
  authorAria,
  authorLabel,
  childOf,
  entryTitle,
  isReadOnlyEntry,
  noteAuthor,
  noteStage,
} from './noteChain';

// One note or handover of the task chain. It edits in place while `editing` is set; the chain keeps
// the one open edit. The body selects and copies.
const props = defineProps<{
  handover: Handover;
  taskId: string;
  readonly?: boolean;
  editing: boolean;
}>();
const emit = defineEmits<{ edit: []; done: []; openSession: [sessionId: string] }>();
const stage = useAdeStageDialogStore();
const update = useUpdateHandover();
const boardUi = useAdeBoardUiStore();
const child = computed(() => childOf(props.handover));
const author = computed(() => noteAuthor(props.handover));

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
    data-kind="handover"
    :data-author="author"
    :data-source="handover.source"
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
        :aria-label="authorAria(author)"
        data-testid="ade-note-author"
      >
        <CodiconIcon :name="AUTHOR_ICON[author]" :size="12" />
        <Badge variant="default">{{ authorLabel(author) }}</Badge>
      </span>
      <Button
        v-if="child"
        type="button"
        variant="link"
        size="inline"
        class="font-medium"
        data-testid="ade-handover-child-link"
        :data-task-id="child.id"
        @click="boardUi.select(child.id)"
        >{{
          entryTitle(handover)
        }}</Button
      >
      <span v-else class="font-medium" data-testid="ade-handover-title">{{
        entryTitle(handover)
      }}</span>
      <span v-if="noteStage(handover)" class="text-muted-foreground" data-testid="ade-note-stage">{{
        noteStage(handover)
      }}</span>
      <Badge v-if="handover.source === 'run'" variant="default">{{
        t('common.action.run.label')
      }}</Badge>
      <Badge v-if="handover.state === 'draft'" variant="default">{{
        t('common.draft.status')
      }}</Badge>
      <Badge v-if="handover.state === 'draft' && handover.finished" variant="ok">{{
        t('space.ade.handover.finished.status')
      }}</Badge>
      <Badge v-if="handover.edited && handover.state !== 'note'" variant="default">{{
        t('llmPrompts.badge.edited.label')
      }}</Badge>
      <span class="ml-auto text-subtle">{{ time(handover) }}</span>
      <AdeOriginSession
        :task-id="taskId"
        :session-id="handover.sessionId"
        :archived="readonly"
        @open="emit('openSession', $event)"
      />
      <CopyIconButton
        v-if="!editing && handover.body.trim()"
        :text="handover.body"
        :label="t('space.ade.handover.copy.tooltip')"
        data-testid="ade-handover-copy"
      />
      <TooltipIconButton
        v-if="!readonly && !editing && !isReadOnlyEntry(handover)"
        icon="edit"
        :label="t('space.ade.handover.edit.tooltip')"
        data-testid="ade-handover-edit"
        @pointerenter="prefetchNotesEditor"
        @click="emit('edit')"
      />
    </div>
    <template v-if="editing">
      <AdeNotesEditor
        :key="`edit-${handover.id}`"
        v-model="text"
        compact
        focus-on-mount
        :hard-breaks="author !== 'user'"
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
    <span
      v-else-if="isReadOnlyEntry(handover) && !handover.body.trim()"
      class="text-muted-foreground"
      data-testid="ade-handover-child-empty"
      >{{
        t('space.ade.notes.childFinishedEmpty.label')
      }}</span
    >
    <AdeNoteBody
      v-else
      :key="`view-${handover.id}`"
      :body="handover.body"
      :hard-breaks="author !== 'user'"
      data-testid="ade-handover-body"
    />
    <div v-if="handover.state === 'draft' && !readonly && !editing" class="flex justify-end">
      <Button
        variant="toolbar"
        size="kira"
        data-testid="ade-handover-move-on"
        @click="stage.open({ taskId })"
        ><CodiconIcon name="arrow-right" :size="13" />
        {{ t('space.ade.handover.moveOn.label') }}</Button
      >
    </div>
  </Card>
</template>
