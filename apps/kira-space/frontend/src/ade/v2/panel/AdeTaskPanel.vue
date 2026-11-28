<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import DetailHeader from '@theme/components/DetailHeader.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { TooltipDisabledTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { useInlineEdit } from '@workbench/behaviour/inlineEdit';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref, watch } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeOrchestratorBadge from '../AdeOrchestratorBadge.vue';
import AdeAutomationsBlock from '../automation/AdeAutomationsBlock.vue';
import { taskPatch } from '../board/panelFacts';
import { taskReviewTip } from '../board/reviewCode';
import AdeDetailsTab from '../details/AdeDetailsTab.vue';
import type { CardModel, PlanModel } from '../plan/usePlanModel';
import { useTaskMenu } from '../plan/useTaskMenu';
import { useOpenFilesWindow, useUpdateTask } from '../queries';
import { useReviewCode } from '../review/useReviewCode';
import AdeTaskActionButton from '../run/AdeTaskActionButton.vue';
import AdeSessionsTab from '../sessions/AdeSessionsTab.vue';
import { useSessionViews } from '../sessions/useSessionViews';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import AdeMainTab from './AdeMainTab.vue';
import AdePanelForwardBar from './AdePanelForwardBar.vue';
import AdePanelFrame from './AdePanelFrame.vue';

// Task mode: header, then the Main, Details and Sessions tabs.
const props = defineProps<{ card: CardModel; model: PlanModel }>();

const ui = useAdeBoardUiStore();
const tabs = useAdePanelTabsStore();
const tab = computed({
  get: () => tabs.taskTab(props.card.task.id),
  set: (v) => {
    if (v === 'sessions') tabs.requestAgentFocus(props.card.task.id);
    tabs.setTaskTab(props.card.task.id, v);
  },
});
// Details mounts on its first visit and then stays mounted (hidden) while the panel shows this task,
// so a return neither refetches its links nor re-renders the chain. Panel-local: no shared state.
const detailsSeenFor = ref<string | null>(null);
watch(
  [tab, () => props.card.task.id],
  ([t, id]) => {
    if (t === 'details') detailsSeenFor.value = id;
  },
  { immediate: true },
);
const detailsMounted = computed(() => detailsSeenFor.value === props.card.task.id);
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
  () =>
    reviewCode.pendingFor(choices.value.map((c) => c.branchId)) ||
    choices.value.every((c) => c.disabled),
);

// The task's Files window: a quiet icon, below the labelled Review button.
const openFiles = useOpenFilesWindow();
const hasRepo = computed(() => props.card.rows.length > 0);
const filesDisabled = computed(() => openFiles.isPending.value);
function openFilesWindow(ev: MouseEvent): void {
  const taskId = props.card.task.id;
  const returnTo = ev.currentTarget as HTMLElement | null;
  delete ui.actionError[taskId];
  openFiles.mutate(
    { taskId },
    {
      onError: (err) => {
        ui.actionError[taskId] = errorText(err);
      },
      onSettled: () => {
        if (returnTo?.isConnected) returnTo.focus({ preventScroll: true });
      },
    },
  );
}

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

const label = computed(() =>
  props.card.parked ? t('space.ade.board.notMerging.label') : props.card.status.name,
);
</script>

<template>
  <AdePanelFrame
    v-model="tab"
    :color="card.color"
    :tabs="[
      { value: 'main', label: t('common.main.label') },
      { value: 'details', label: t('common.details.title') },
      { value: 'sessions', label: t('space.common.sessions.label'), count: sessionCount },
    ]"
  >
    <template #actions>
      <AdeTaskActionButton :card="card" />
      <TextTooltip v-if="choices.length" :parts="reviewTip">
        <TooltipDisabledTrigger :disabled="reviewDisabled">
          <Button
            variant="toolbar"
            size="kira"
            :disabled="reviewDisabled"
            data-testid="ade-panel-review"
            @click="(ev: MouseEvent) => reviewCode.openTask(card, ev.currentTarget as HTMLElement)"
          >
            <CodiconIcon name="git-compare" :size="13" />
            {{ t('space.ade.board.taskMenu.reviewCode.label') }}
          </Button>
        </TooltipDisabledTrigger>
      </TextTooltip>
      <TooltipIconButton
        v-if="hasRepo"
        icon="files"
        :label="t('space.ade.panel.files.tooltip')"
        disabled-trigger
        :disabled="filesDisabled"
        class="shrink-0"
        data-testid="ade-panel-files"
        @click="openFilesWindow"
      />
      <TooltipIconButton
        icon="ellipsis"
        :label="t('common.moreActions.tooltip')"
        class="shrink-0"
        data-testid="ade-panel-more"
        @click="openMore"
      />
    </template>
    <template #header>
      <DetailHeader flush class="px-3 pt-2 pb-1" @contextmenu="taskMenu.open">
        <template #leading>
          <Badge v-if="card.parked" variant="default">{{ label }}</Badge>
          <AdeChip v-else :label="label" :color="card.status.color" />
          <AdeOrchestratorBadge v-if="card.orchestrator" />
          <Badge v-if="card.blocked" variant="error" data-testid="ade-panel-blocked">{{
            t('space.common.status.blocked.status')
          }}</Badge>
        </template>
        <template #control>
          <Input
            :model-value="titleEdit.text.value"
            :placeholder="card.defaultTitle"
            :aria-label="t('common.field.name.label')"
            appearance="inline"
            size="fluid"
            class="min-w-0 flex-1 text-kira-lg font-medium text-ellipsis"
            data-testid="ade-panel-title"
            @update:model-value="titleEdit.onInput"
            @blur="titleEdit.onCommit"
            @keydown="titleEdit.onKeydown"
          />
        </template>
        <Alert v-if="titleEdit.error.value" variant="error" data-testid="ade-title-error"
          ><AlertDescription>{{ titleEdit.error.value }}</AlertDescription></Alert
        >
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
      </DetailHeader>
    </template>
    <AdeMainTab v-if="tab === 'main'" :card="card" />
    <AdeSessionsTab v-else-if="tab === 'sessions'" :task-id="card.task.id" />
    <AdeDetailsTab
      v-if="detailsMounted"
      v-show="tab === 'details'"
      :task="card.task"
      :branches="card.rows.map((r) => r.branch)"
      :active="tab === 'details'"
    />
  </AdePanelFrame>
</template>
