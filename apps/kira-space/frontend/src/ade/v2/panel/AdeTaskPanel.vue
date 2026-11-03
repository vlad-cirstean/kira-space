<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { TooltipDisabledTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { useInlineEdit } from '@workbench/behaviour/inlineEdit';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeTip from '../AdeTip.vue';
import AdeArtifactList from '../artifacts/AdeArtifactList.vue';
import AdeAutomationsBlock from '../automation/AdeAutomationsBlock.vue';
import { archiveTip } from '../board/actions';
import { dayLabel, LATER } from '../board/calendar';
import { taskFacts, taskPatch } from '../board/panelFacts';
import { taskReviewTip } from '../board/reviewCode';
import AdeHandoverList from '../notes/AdeHandoverList.vue';
import AdeNotesEditor from '../notes/AdeNotesEditor.vue';
import type { CardModel, PlanModel } from '../plan/usePlanModel';
import { useTaskMenu } from '../plan/useTaskMenu';
import { useShelveTask, useUpdateTask } from '../queries';
import { useReviewCode } from '../review/useReviewCode';
import AdeTaskActionButton from '../run/AdeTaskActionButton.vue';
import AdeSessionsTab from '../sessions/AdeSessionsTab.vue';
import { useSessionViews } from '../sessions/useSessionViews';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import AdePanelFrame from './AdePanelFrame.vue';
import AdeTaskTab from './AdeTaskTab.vue';

// Task mode: header facts, the Task, Notes, Artifacts and Sessions tabs.
const props = defineProps<{ card: CardModel; model: PlanModel }>();

const ui = useAdeBoardUiStore();
const tabs = useAdePanelTabsStore();
const tab = computed({
  get: () => tabs.taskTab(props.card.task.id),
  set: (v) => tabs.setTaskTab(props.card.task.id, v),
});
const dialogs = useAdeDialogsStore();
const { sessions } = useSessionViews();
const sessionCount = computed(
  () =>
    sessions.value.filter((x) => x.taskId === props.card.task.id && x.state === 'running').length,
);
const update = useUpdateTask();
const shelve = useShelveTask();
const taskMenu = useTaskMenu(() => props.card);
function openMore(ev: MouseEvent): void {
  if (ev.currentTarget instanceof Element) taskMenu.openAt(ev.currentTarget);
}
const reviewCode = useReviewCode();
const choices = computed(() => reviewCode.choicesOf(props.card));
const reviewTip = computed(() => taskReviewTip(choices.value));
const reviewDisabled = computed(
  () => reviewCode.pending.value || choices.value.every((c) => c.disabled),
);
const notesError = ref('');

const titleEdit = useInlineEdit({
  source: () => props.card.task.title,
  commit: async (title) => {
    await update.mutateAsync({ taskId: props.card.task.id, patch: taskPatch({ title }) });
  },
  // A task without a Jira key needs its title.
  get allowEmpty() {
    return Boolean(props.card.task.jira);
  },
});

/** A finished task shows Archive as its step action; any other task gets the quiet one. */
const quietArchive = computed(
  () => props.card.task.kind !== 'review' && props.card.action?.kind !== 'archive',
);

/** Backlog tasks go back to Later and any other task to Backlog; a review item stays on the Plan. */
const shelfTarget = computed(() => {
  if (props.card.review) return null;
  return props.card.shelf === 'backlog' ? 'later' : 'backlog';
});
async function moveShelf(): Promise<void> {
  const to = shelfTarget.value;
  if (!to) return;
  delete ui.actionError[props.card.task.id];
  try {
    await shelve.mutateAsync({ taskId: props.card.task.id, shelf: to, beforeId: '' });
  } catch (err) {
    ui.actionError[props.card.task.id] = errorText(err);
  }
}

const chip = computed(() =>
  props.card.parked ? { tone: 'grey' as const } : { color: props.card.status.color },
);
const label = computed(() =>
  props.card.parked ? t('space.ade.board.notMerging.label') : props.card.status.name,
);

const facts = computed(() => {
  const e = props.card.entry;
  const cal = props.model.cal;
  const shelf = props.card.shelf;
  const span =
    shelf === 'backlog'
      ? t('space.common.backlog.label')
      : e.day === LATER
        ? t('common.later.label')
        : dayLabel(cal, e.day) + (e.days.length > 1 ? `–${dayLabel(cal, e.end)}` : '');
  const repos: string[] = [];
  for (const row of props.card.rows) if (!repos.includes(row.repo)) repos.push(row.repo);
  const p = props.card.progress;
  return taskFacts({
    span,
    branches: props.card.rows.length,
    repos,
    doneSteps: p.doneSteps,
    steps: p.totalSteps,
  });
});

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
  <AdePanelFrame
    v-model="tab"
    :tabs="[
      { value: 'task', label: t('common.task.label') },
      { value: 'notes', label: t('space.common.notes.label') },
      { value: 'artifacts', label: t('space.ade.artifacts.title.title') },
      { value: 'sessions', label: t('space.common.sessions.label'), count: sessionCount },
    ]"
  >
    <template #actions>
      <AdeTaskActionButton :card="card" />
      <AdeTip v-if="quietArchive" :text="archiveTip()">
        <Button
          variant="dialog"
          size="kira-lg"
          :pending="dialogs.pending.has(`archive:${card.task.id}`)"
          data-testid="ade-panel-archive"
          @click="dialogs.archive(card.task.id)"
        >
          {{ t('space.common.archive.label') }}
        </Button>
      </AdeTip>
      <Button
        v-if="shelfTarget"
        variant="dialog"
        size="kira-lg"
        :pending="shelve.isPending.value"
        data-testid="ade-panel-shelve"
        @click="moveShelf"
      >
        {{ shelfTarget === 'backlog' ? t('space.common.backlog.label') : t('common.later.label') }}
      </Button>
      <AdeTip v-if="choices.length" :parts="reviewTip">
        <TooltipDisabledTrigger :disabled="reviewDisabled">
          <Button
            variant="dialog"
            size="kira-lg"
            :disabled="reviewDisabled"
            data-testid="ade-panel-review"
            @click="(ev: MouseEvent) => reviewCode.openTask(card, ev.currentTarget as HTMLElement)"
          >
            <CodiconIcon name="git-compare" :size="13" />
            {{ t('space.ade.board.taskMenu.reviewCode.label') }}
          </Button>
        </TooltipDisabledTrigger>
      </AdeTip>
      <TooltipIconButton
        icon="ellipsis"
        :label="t('common.moreActions.tooltip')"
        class="shrink-0"
        data-testid="ade-panel-more"
        @click="openMore"
      />
    </template>
    <template #header>
      <div class="flex items-start gap-2">
        <span
          class="mt-1 box-border size-3 shrink-0 rounded-kira-xs"
          :class="card.parked ? 'border-2' : ''"
          :style="card.parked ? { borderColor: card.color } : { background: card.color }"
          data-testid="ade-panel-dot"
        />
        <AdeChip :label="label" v-bind="chip" />
        <AdeChip
          v-if="card.blocked"
          :label="t('space.common.status.blocked.status')"
          tone="red"
          data-testid="ade-panel-blocked"
        />
        <Input
          :model-value="titleEdit.text.value"
          :placeholder="card.defaultTitle"
          :aria-label="t('common.field.name.label')"
          appearance="inline"
          class="min-w-0 flex-1 text-kira-lg font-medium"
          data-testid="ade-panel-title"
          @update:model-value="titleEdit.onInput"
          @blur="titleEdit.onCommit"
          @keydown="titleEdit.onKeydown"
        />
      </div>
      <Alert v-if="titleEdit.error.value" variant="error" data-testid="ade-title-error"
        ><AlertDescription>{{ titleEdit.error.value }}</AlertDescription></Alert
      >
      <div class="truncate text-kira-sm text-muted-foreground" data-testid="ade-panel-facts">{{
        facts
      }}</div>
      <Alert
        v-if="ui.actionError[card.task.id]"
        variant="error"
        data-testid="ade-panel-action-error"
        ><AlertDescription>{{ ui.actionError[card.task.id] }}</AlertDescription></Alert
      >
      <AdeAutomationsBlock :task-id="card.task.id" />
      <Alert v-if="card.review" variant="info" data-testid="ade-panel-review-note"
        ><AlertDescription>{{
          t('space.ade.taskPanel.reviewNote.body', {
            owner: card.task.owner || t('space.ade.taskPanel.someone.label'),
          })
        }}</AlertDescription></Alert
      >
    </template>
    <AdeTaskTab v-if="tab === 'task'" :card="card" />
    <AdeSessionsTab v-else-if="tab === 'sessions'" :task-id="card.task.id" />
    <div
      v-else-if="tab === 'artifacts'"
      class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-3"
      data-testid="ade-artifacts-tab"
    >
      <AdeArtifactList :task-id="card.task.id" />
    </div>
    <div
      v-else
      class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-3"
      data-testid="ade-notes-tab"
    >
      <AdeNotesEditor :notes="card.task.notes" :item-id="card.task.id" @save="saveNotes" />
      <Alert v-if="notesError" variant="error" data-testid="ade-notes-error"
        ><AlertDescription>{{ notesError }}</AlertDescription></Alert
      >
      <AdeHandoverList :task-id="card.task.id" />
    </div>
  </AdePanelFrame>
</template>
