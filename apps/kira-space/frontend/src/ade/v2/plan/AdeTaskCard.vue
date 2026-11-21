<script setup lang="ts">
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import { plainText } from '@theme/varText';
import { formatPercent } from '@workbench/behaviour/format';
import MenuItems from '@workbench/components/MenuItems.vue';
import { formatShortcut } from '@workbench/shortcuts/keys';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { useApprovalsStore } from '@workbench/state/approvals';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed, useTemplateRef } from 'vue';
import AdeOrchestratorBadge from '../AdeOrchestratorBadge.vue';
import AdeParentChip from '../AdeParentChip.vue';
import AdeAutomationChip from '../automation/AdeAutomationChip.vue';
import { changeBaseChoices } from '../board/changeBase';
import { taskReviewTip } from '../board/reviewCode';
import { useReviewCode } from '../review/useReviewCode';
import AdeTaskActionButton from '../run/AdeTaskActionButton.vue';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import AdeUpdatesPopover from '../updates/AdeUpdatesPopover.vue';
import { useTaskUpdates } from '../updates/useTaskUpdates';
import AdeAttention from './AdeAttention.vue';
import AdeBranchRow from './AdeBranchRow.vue';
import AdeEstimateChip from './AdeEstimateChip.vue';
import AdeLeaseChip from './AdeLeaseChip.vue';
import AdeOpStatus from './AdeOpStatus.vue';
import AdeStepMenu from './AdeStepMenu.vue';
import AdeTagIcon from './AdeTagIcon.vue';
import AdeTaskAddMenu from './AdeTaskAddMenu.vue';
import AdeTaskChildren from './AdeTaskChildren.vue';
import AdeTaskKey from './AdeTaskKey.vue';
import AdeTaskSessions from './AdeTaskSessions.vue';
import AdeTaskStatusMenu from './AdeTaskStatusMenu.vue';
import AdeTaskTitle from './AdeTaskTitle.vue';
import { TASK_TAG_GLYPH } from './glyphs';
import {
  GROUND_CARD,
  GROUND_ROW,
  GROUND_SELECTED,
  HOVER_OVERLAY,
  TASK_ACTION_CELL,
  TASK_EDGE,
  TASK_LEAD,
  TASK_STATUS_CELL,
  TASK_STEP_CELL,
  TASK_TIME_CELL,
} from './taskGrid';
import { type BranchRowModel, type CardModel, usePlanModel } from './usePlanModel';
import { useTaskMenu } from './useTaskMenu';

const props = defineProps<{ card: CardModel }>();
const emit = defineEmits<{ select: []; forcePush: [row: BranchRowModel] }>();

const ui = useAdeBoardUiStore();
const dialogs = useAdeDialogsStore();
const { view, boardQuery, rippleIds } = usePlanModel();
const approvalsStore = useApprovalsStore();
const { byTask } = useTaskUpdates();
const taskUpdates = computed(() => byTask.value.get(props.card.task.id) ?? []);
const taskMenu = useTaskMenu(() => props.card);
const reviewCode = useReviewCode();
const choices = computed(() => reviewCode.choicesOf(props.card));
const reviewTip = computed(() => taskReviewTip(choices.value));

// The stored status shows as text; "Pass done" has no colour and keeps its glyph.
const tagGlyph = computed(() =>
  props.card.tag && !(props.card.tag.kind === 'status' && props.card.tag.color)
    ? props.card.tag
    : null,
);

function onRowMenu(at: MouseEvent | HTMLElement): void {
  if (at instanceof HTMLElement) taskMenu.openAt(at);
  else taskMenu.open(at);
}

const head = useTemplateRef<HTMLElement>('head');
useShortcut(head, ['menu.open'], () => {
  if (head.value) taskMenu.openAt(head.value);
});

// One branch opens Change base at once; several ask which.
const baseChoices = computed(() => {
  const v = view.value;
  const board = boardQuery.data.value;
  return v && board ? changeBaseChoices(props.card, { graph: v.graph, after: v.after, board }) : [];
});

const baseItems = computed<MenuItem[]>(() =>
  baseChoices.value.map((c) => ({
    type: 'item',
    id: c.branchId,
    testId: 'ade-card-change-base-item',
    label: plainText(c.parts),
    parts: c.parts,
    disabled: c.disabled,
    run: () => dialogs.act(c.act),
  })),
);

const draftRepoIds = computed(() =>
  props.card.rows.filter((r) => r.branch.name === '').map((r) => r.branch.codeRepoId),
);
const selected = computed(() => ui.selectedTaskId === props.card.task.id);
const inRipple = computed(() => rippleIds.value.tasks.has(props.card.task.id));
const headSelected = computed(() => selected.value && ui.selectedBranchId === null);
const p = computed(() => props.card.progress);
const segClass = (state: string): string => {
  if (state === 'done') return 'bg-muted-foreground';
  if (state === 'todo') return 'bg-border';
  if (state === 'skipped') return 'bg-field';
  return p.value.bad ? 'bg-tone-red-solid' : 'bg-fg';
};
const boxClass = computed(() => {
  const c = props.card;
  return [
    'bg-chrome cursor-grab border',
    TASK_EDGE,
    GROUND_CARD,
    selected.value ? 'border-select-edge' : inRipple.value ? 'border-subtle/50' : 'border-border',
  ];
});
</script>

<template>
  <div
    class="flex items-start gap-2"
    data-ade-task=""
    data-testid="ade-task"
    :data-task-id="card.task.id"
    :data-selected="selected || undefined"
  >
    <Card
      variant="section"
      :data-task-id="card.task.id"
      class="box-border flex min-w-0 flex-1 flex-col overflow-hidden in-[.sortable-fallback]:max-h-board-row in-[.sortable-fallback]:max-w-64 in-[.sortable-fallback]:shadow-kira"
      :class="boxClass"
      :style="{ borderTopColor: card.color }"
      :aria-current="selected || undefined"
      data-testid="ade-card"
      @contextmenu="taskMenu.open"
    >
      <!-- biome-ignore lint/a11y/useSemanticElements: the header holds block content a button cannot. -->
      <div
        v-if="!card.review"
        ref="head"
        class="group/head box-border flex h-board-row w-full items-center gap-1.5 px-2"
        :class="headSelected ? GROUND_SELECTED : GROUND_ROW"
        role="button"
        tabindex="0"
        :aria-label="t('space.ade.plan.card.open.aria', { title: card.title })"
        data-testid="ade-card-head"
        @click="emit('select')"
        @keydown.enter.self="emit('select')"
        @keydown.space.self.prevent="emit('select')"
      >
        <span :class="TASK_LEAD" data-testid="ade-row-lead">
          <AdeTaskSessions
            v-if="card.sessions"
            :task-id="card.task.id"
            :sessions="card.sessions"
            size="lg"
          />
        </span>
        <div class="flex min-w-0 flex-1 flex-col justify-center gap-0.5">
          <div class="flex h-4.5 min-w-0 items-center gap-1.5">
            <span class="relative flex min-w-0 flex-1 items-center gap-1.5">
              <AdeTaskTitle :card="card" />
              <AdeTaskKey :card="card" />
              <span
                class="hidden group-hover/head:flex group-focus-visible/head:flex group-has-[:focus-visible]/head:flex has-[[data-state=open]]:flex has-[[data-pinned]]:flex"
                :class="HOVER_OVERLAY"
                data-testid="ade-card-actions"
              >
                <AdeTaskAddMenu v-if="!card.review" :card="card" />
                <DropdownMenu v-if="baseChoices.length > 1">
                  <DropdownMenuTrigger as-child>
                    <TooltipIconButton
                      icon="git-branch"
                      :label="t('space.ade.plan.card.changeBase.tooltip')"
                      :aria-label="t('space.ade.rebase.changeBase.label')"
                      class="size-4.5 shrink-0"
                      data-testid="ade-card-change-base"
                      @click.stop
                    />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" data-testid="ade-card-change-base-menu">
                    <MenuItems :items="baseItems" />
                  </DropdownMenuContent>
                </DropdownMenu>
                <TooltipIconButton
                  v-else-if="baseChoices.length === 1"
                  icon="git-branch"
                  :label="t('space.ade.rebase.changeBase.label')"
                  class="size-4.5 shrink-0"
                  disabled-trigger
                  :disabled="baseChoices[0].disabled"
                  data-testid="ade-card-change-base"
                  @click.stop="dialogs.act(baseChoices[0].act)"
                >
                  <VarText :parts="baseChoices[0].tip" />
                </TooltipIconButton>
                <TooltipIconButton
                  v-if="choices.length"
                  icon="git-compare"
                  :label="t('space.ade.board.taskMenu.reviewCode.label')"
                  :kbd="formatShortcut('ade.reviewCode')"
                  class="size-4.5 shrink-0"
                  disabled-trigger
                  :pending="reviewCode.pendingFor(choices.map((c) => c.branchId))"
                  :disabled="choices.every((c) => c.disabled)"
                  data-testid="ade-card-review"
                  @click.stop="
                    (ev: MouseEvent) => reviewCode.openTask(card, ev.currentTarget as HTMLElement)
                  "
                >
                  <VarText :parts="reviewTip" />
                </TooltipIconButton>
              </span>
            </span>
            <span :class="TASK_ACTION_CELL"><AdeTaskActionButton :card="card" icon /></span>
          </div>
          <div class="flex h-4.5 min-w-0 items-center gap-1.5">
            <span :class="TASK_STATUS_CELL"><AdeTaskStatusMenu :card="card" /></span>
            <span :class="TASK_STEP_CELL"
              ><template v-if="p.hasWorkflow && !card.parked">
                <TextTooltip :text="p.workflowTip">
                  <span
                    class="inline-flex w-8 shrink-0 items-center gap-0.5 @lg:w-10"
                    data-testid="ade-step-segs"
                  >
                    <span
                      v-for="seg in p.segments"
                      :key="seg.id"
                      class="h-1.5 flex-1 rounded-kira-sm"
                      :class="segClass(seg.state)"
                    />
                  </span>
                </TextTooltip>
                <AdeStepMenu :card="card" />
                <TextTooltip v-if="p.showBar" :text="p.barTip">
                  <span
                    class="shrink-0 text-kira-sm @max-lg:hidden text-muted-foreground"
                    data-testid="ade-step-percent"
                    >{{
                      formatPercent(p.percent)
                    }}</span
                  >
                </TextTooltip>
              </template></span
            >
            <span
              class="min-w-0 flex-1 text-kira-sm leading-3.5 text-subtle"
              data-testid="ade-card-meta"
              ><TruncatedText :text="card.meta" /></span
            >
            <span class="flex shrink-0 items-center gap-1">
              <AdeTagIcon
                v-if="tagGlyph"
                :glyph="TASK_TAG_GLYPH[tagGlyph.kind]"
                :kind="tagGlyph.kind"
                :label="tagGlyph.label"
                :tip="tagGlyph.tip"
                :tone="tagGlyph.tone"
                :color="tagGlyph.color"
              />
              <AdeOrchestratorBadge v-if="card.orchestrator" icon-only />
              <AdeOpStatus :task-id="card.task.id" />
              <AdeAttention
                v-if="card.attention"
                :tip="card.attention"
                :item="card.attentionItem"
              />
              <AdeAutomationChip :task-id="card.task.id" />
              <AdeLeaseChip :task-id="card.task.id" />
              <AdeUpdatesPopover v-if="taskUpdates.length" :task-id="card.task.id" />
              <span v-if="card.parent" class="inline-flex min-w-0 max-w-24">
                <AdeParentChip
                  :parent="card.parent"
                  @select="card.parent && ui.select(card.parent.id)"
                />
              </span>
              <Button
                v-if="card.task.branchRequestId"
                type="button"
                variant="toolbar"
                size="inline"
                class="h-4.5 min-w-0 max-w-24 truncate rounded-kira-sm px-1 text-kira-sm text-tone-amber"
                data-testid="ade-card-branch-request"
                @click.stop="approvalsStore.open(`branch.request:${card.task.branchRequestId}`)"
              >
                {{ t('space.ade.taskTab.branchRequest.label') }}
              </Button>
            </span>
            <span :class="TASK_TIME_CELL"
              ><AdeEstimateChip v-if="!card.review" :card="card" /></span
            >
          </div>
        </div>
      </div>
      <AdeBranchRow
        v-for="row in card.rows"
        :key="row.id"
        :row="row"
        :color="card.color"
        :task-row="card.review"
        :draft-repo-ids="draftRepoIds"
        :editable="!card.review"
        @force-push="emit('forcePush', row)"
        @task-menu="onRowMenu"
      />
      <AdeTaskChildren v-if="card.orchestrator" :card="card" />
    </Card>
  </div>
</template>
