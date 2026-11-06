<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { TooltipDisabledTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { useInlineEdit } from '@workbench/behaviour/inlineEdit';
import { computed } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeTip from '../AdeTip.vue';
import AdeAutomationsBlock from '../automation/AdeAutomationsBlock.vue';
import { dayLabel, LATER } from '../board/calendar';
import { taskFacts, taskPatch } from '../board/panelFacts';
import { taskReviewTip } from '../board/reviewCode';
import AdeDetailsTab from '../details/AdeDetailsTab.vue';
import type { CardModel, PlanModel } from '../plan/usePlanModel';
import { useTaskMenu } from '../plan/useTaskMenu';
import { useUpdateTask } from '../queries';
import { useReviewCode } from '../review/useReviewCode';
import AdeTaskActionButton from '../run/AdeTaskActionButton.vue';
import AdeSessionsTab from '../sessions/AdeSessionsTab.vue';
import { useSessionViews } from '../sessions/useSessionViews';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import AdeMainTab from './AdeMainTab.vue';
import AdePanelForwardBar from './AdePanelForwardBar.vue';
import AdePanelFrame from './AdePanelFrame.vue';

// Task mode: header facts, then the Main, Details and Sessions tabs.
const props = defineProps<{ card: CardModel; model: PlanModel }>();

const ui = useAdeBoardUiStore();
const tabs = useAdePanelTabsStore();
const tab = computed({
  get: () => tabs.taskTab(props.card.task.id),
  set: (v) => tabs.setTaskTab(props.card.task.id, v),
});
const { sessions } = useSessionViews();
const sessionCount = computed(
  () =>
    sessions.value.filter((x) => x.taskId === props.card.task.id && x.state === 'running').length,
);
const update = useUpdateTask();
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
</script>

<template>
  <AdePanelFrame
    v-model="tab"
    :tabs="[
      { value: 'main', label: t('common.main.label') },
      { value: 'details', label: t('common.details.title') },
      { value: 'sessions', label: t('space.common.sessions.label'), count: sessionCount },
    ]"
  >
    <template #actions>
      <AdeTaskActionButton :card="card" />
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
      <!-- biome-ignore lint/a11y/noStaticElementInteractions: right-click only; the keyboard opens the same menu from More. -->
      <div class="flex items-start gap-2" @contextmenu="taskMenu.open">
        <span
          class="mt-1 box-border size-3 shrink-0 rounded-kira-sm"
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
      <AdePanelForwardBar v-if="!card.review" :task-id="card.task.id" />
      <AdeAutomationsBlock :task-id="card.task.id" />
      <Alert v-if="card.review" variant="info" data-testid="ade-panel-review-note"
        ><AlertDescription>{{
          t('space.ade.taskPanel.reviewNote.body', {
            owner: card.task.owner || t('space.ade.taskPanel.someone.label'),
          })
        }}</AlertDescription></Alert
      >
    </template>
    <AdeMainTab v-if="tab === 'main'" :card="card" />
    <AdeSessionsTab v-else-if="tab === 'sessions'" :task-id="card.task.id" />
    <AdeDetailsTab v-else :task="card.task" :branches="card.rows.map((r) => r.branch)" />
  </AdePanelFrame>
</template>
