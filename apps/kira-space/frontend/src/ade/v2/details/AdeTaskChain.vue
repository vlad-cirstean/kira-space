<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { Toggle } from '@theme/components/ui/toggle';
import { t } from '@theme/i18n';
import { useMutationAction } from '@workbench/behaviour/asyncAction';
import { computed, ref } from 'vue';
import AdeNoteEntry from '../notes/AdeNoteEntry.vue';
import { AdeNotesEditor, prefetchNotesEditor } from '../notes/asyncEditor';
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
import AdeProofEntry from './AdeProofEntry.vue';
import AdeTimelineEntry from './AdeTimelineEntry.vue';
import { taskChain } from './taskChain';
import { type TimelineBranch, timelineEvents } from './taskTimeline';

// The task chain: the user's notes, what each stage left for the next and the proof sessions
// attached to the task, one list, newest first; the Timeline toggle adds task events and the Kira Studio
// toggle (on by default) the DB actions of its agent sessions. An entry edits
// in place unless the task is archived.
const props = withDefaults(
  defineProps<{
    task: Task;
    branches: readonly TimelineBranch[];
    readonly?: boolean;
    /** False while the tab is mounted but hidden: the artifacts query pauses. */
    active?: boolean;
  }>(),
  { active: true },
);
const emit = defineEmits<{ openSession: [sessionId: string] }>();
const taskId = computed(() => props.task.id);
const composer = useAdeNoteComposerStore();
const list = useAdeDetailsListStore();
const handovers = useHandovers(() => taskId.value);
const proofs = useTaskProofs(() => taskId.value);
const artifacts = useTaskArtifacts(
  () => taskId.value,
  () => props.active,
);
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

const adding = useMutationAction(add);
async function submit(): Promise<void> {
  if (!draft.value.trim()) return;
  if (await adding.run({ taskId: taskId.value, body: draft.value })) composer.clear(taskId.value);
}

const editing = ref<string | null>(null);
</script>

<template>
  <div
    class="flex flex-col gap-1 *:data-[slot=card]:gap-1 *:data-[slot=card]:px-2 *:data-[slot=card]:py-1.5"
    data-testid="ade-task-chain"
  >
    <SectionHeading flush :label="t('space.ade.proof.chain.title')">
      <TextTooltip :text="t('space.ade.details.timeline.toggle.tooltip')">
        <Toggle
          :model-value="list.showTimeline"
          size="kira"
          data-testid="ade-details-timeline-toggle"
          @update:model-value="list.setShowTimeline"
        >
          <CodiconIcon name="history" :size="13" />
          {{ t('common.timeline.label') }}
        </Toggle>
      </TextTooltip>
      <TextTooltip v-if="auditCount > 0" :text="t('space.ade.details.dbAudit.toggle.tooltip')"
        ><Toggle
          :model-value="list.showDbAudit"
          size="kira"
          data-testid="ade-details-dbaudit-toggle"
          @update:model-value="list.setShowDbAudit"
        >
          <CodiconIcon name="database" :size="13" />
          {{ t('space.ade.details.dbAudit.toggle.label', { count: auditCount }) }}
        </Toggle></TextTooltip
      >
      <template v-if="!readonly">
        <TooltipIconButton
          v-if="!composing"
          icon="add"
          :label="t('space.ade.notes.add.tooltip')"
          data-testid="ade-note-add"
          @pointerenter="prefetchNotesEditor"
          @click="composer.show(task.id)"
        />
      </template>
    </SectionHeading>
    <Card v-if="composing" data-testid="ade-note-composer">
      <AdeNotesEditor
        :key="`compose-${task.id}`"
        v-model="draft"
        compact
        focus-on-mount
        data-testid="ade-note-composer-text"
        @submit="submit"
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
          @click="submit"
          >{{
            t('common.action.save.label')
          }}</Button
        >
      </div>
    </Card>
    <InlineState v-if="!hasChain && !composing" kind="empty" data-testid="ade-notes-empty">
      <span>{{ t('space.ade.notes.empty.body') }}</span>
      <Button
        v-if="!readonly"
        variant="toolbar"
        size="kira"
        data-testid="ade-note-add-empty"
        @click="composer.show(task.id)"
        ><CodiconIcon name="add" :size="13" />{{ t('space.ade.notes.add.label') }}</Button
      >
    </InlineState>
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
