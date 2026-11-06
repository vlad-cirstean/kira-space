<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { Toggle } from '@theme/components/ui/toggle';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed, ref } from 'vue';
import AdeNoteEntry from '../notes/AdeNoteEntry.vue';
import AdeNotesEditor from '../notes/AdeNotesEditor.vue';
import {
  useAddNote,
  useHandovers,
  useTaskArtifacts,
  useTaskDbAudit,
  useTaskProofs,
} from '../queries';
import { useAdeDetailsListStore } from '../state/adeDetailsList';
import { useAdeNoteComposerStore } from '../state/adeNoteComposer';
import type { Task } from '../wire';
import AdeAuditEntry from './AdeAuditEntry.vue';
import AdeProofAttachMenu from './AdeProofAttachMenu.vue';
import AdeProofEntry from './AdeProofEntry.vue';
import AdeTimelineEntry from './AdeTimelineEntry.vue';
import { taskChain } from './taskChain';
import { type TimelineBranch, timelineEvents } from './taskTimeline';

// The task chain: the user's notes, what each stage left for the next and the proof sessions
// attached to the task, one list, newest first; the Timeline toggle adds task events and the Kira DB
// toggle (on by default) the DB actions of its agent sessions. An entry edits
// in place unless the task is archived.
const props = defineProps<{
  task: Task;
  branches: readonly TimelineBranch[];
  readonly?: boolean;
}>();
const emit = defineEmits<{ openSession: [sessionId: string] }>();
const taskId = computed(() => props.task.id);
const composer = useAdeNoteComposerStore();
const list = useAdeDetailsListStore();
const handovers = useHandovers(() => taskId.value);
const proofs = useTaskProofs(() => taskId.value);
const artifacts = useTaskArtifacts(() => taskId.value);
const dbAudit = useTaskDbAudit(() => taskId.value);
const auditCount = computed(() => dbAudit.data.value?.entries.length ?? 0);
const add = useAddNote();

const entries = computed(() =>
  taskChain({
    handovers: handovers.data.value?.handovers ?? [],
    proofs: proofs.data.value?.proofs ?? [],
    events: list.showTimeline
      ? timelineEvents(props.task, props.branches, artifacts.data.value?.artifacts ?? [])
      : [],
    audit: list.showDbAudit ? (dbAudit.data.value?.entries ?? []) : [],
  }),
);
const hasChain = computed(() => entries.value.some((e) => e.layer === 'chain'));
const composing = computed(() => !props.readonly && composer.isOpen(taskId.value));
const draft = computed({
  get: () => composer.draftOf(taskId.value),
  set: (v: string) => composer.setDraft(taskId.value, v),
});

const adding = useAsyncAction(async () => {
  if (!draft.value.trim()) return;
  await add.mutateAsync({ taskId: taskId.value, body: draft.value });
  composer.clear(taskId.value);
});

const editing = ref<string | null>(null);
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="ade-task-chain">
    <div class="flex items-center gap-1 text-kira-sm text-muted-foreground">
      <span class="flex-1">{{ t('space.ade.proof.chain.title') }}</span>
      <Tooltip>
        <TooltipTrigger as-child>
          <Toggle
            :model-value="list.showTimeline"
            size="kira"
            data-testid="ade-details-timeline-toggle"
            @update:model-value="list.setShowTimeline"
          >
            <CodiconIcon name="history" :size="13" />
            {{ t('common.timeline.label') }}
          </Toggle>
        </TooltipTrigger>
        <TooltipContent>{{ t('space.ade.details.timeline.toggle.tooltip') }}</TooltipContent>
      </Tooltip>
      <Tooltip v-if="auditCount > 0">
        <TooltipTrigger as-child>
          <Toggle
            :model-value="list.showDbAudit"
            size="kira"
            data-testid="ade-details-dbaudit-toggle"
            @update:model-value="list.setShowDbAudit"
          >
            <CodiconIcon name="database" :size="13" />
            {{ t('space.ade.details.dbAudit.toggle.label', { count: auditCount }) }}
          </Toggle>
        </TooltipTrigger>
        <TooltipContent>{{ t('space.ade.details.dbAudit.toggle.tooltip') }}</TooltipContent>
      </Tooltip>
      <template v-if="!readonly">
        <TooltipIconButton
          v-if="!composing"
          icon="add"
          :label="t('space.ade.notes.add.tooltip')"
          data-testid="ade-note-add"
          @click="composer.show(task.id)"
        />
        <AdeProofAttachMenu :task-id="task.id" />
      </template>
    </div>
    <Card v-if="composing" class="flex flex-col gap-2 p-2" data-testid="ade-note-composer">
      <AdeNotesEditor
        :key="`compose-${task.id}`"
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
          @click="composer.clear(task.id)"
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
      v-if="!hasChain && !composing"
      class="flex items-center gap-2 text-kira-sm text-subtle"
      data-testid="ade-notes-empty"
    >
      <span>{{ t('space.ade.notes.empty.body') }}</span>
      <Button
        v-if="!readonly"
        variant="dialog"
        size="kira"
        data-testid="ade-note-add-empty"
        @click="composer.show(task.id)"
        >{{
          t('space.ade.notes.add.label')
        }}</Button
      >
    </div>
    <template v-for="e in entries" :key="e.key">
      <AdeNoteEntry
        v-if="e.kind === 'handover'"
        :handover="e.handover"
        :task-id="task.id"
        :readonly="readonly"
        :editing="editing === e.handover.id"
        @edit="editing = e.handover.id"
        @done="editing = null"
        @open-session="emit('openSession', $event)"
      />
      <AdeProofEntry
        v-else-if="e.kind === 'proof'"
        :proof="e.proof"
        :task-id="task.id"
        :readonly="readonly"
        @open-session="emit('openSession', $event)"
      />
      <AdeAuditEntry
        v-else-if="e.kind === 'audit'"
        :entry="e.audit"
        :task-id="task.id"
        :archived="readonly"
        @open-session="emit('openSession', $event)"
      />
      <AdeTimelineEntry
        v-else
        :event="e.event"
        :task-id="task.id"
        :archived="readonly"
        @open-session="emit('openSession', $event)"
      />
    </template>
  </div>
</template>
