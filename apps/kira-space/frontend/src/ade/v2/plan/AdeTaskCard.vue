<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { Kbd } from '@theme/components/ui/kbd';
import VarText from '@theme/components/VarText.vue';
import { t } from '@theme/i18n';
import { formatPercent } from '@workbench/behaviour/format';
import { formatShortcut } from '@workbench/shortcuts/keys';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed, useTemplateRef } from 'vue';
import AdeOrchestratorBadge from '../AdeOrchestratorBadge.vue';
import AdeParentChip from '../AdeParentChip.vue';
import AdeTip from '../AdeTip.vue';
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
import AdeTaskSessions from './AdeTaskSessions.vue';
import AdeTaskStatusMenu from './AdeTaskStatusMenu.vue';
import AdeTaskTitle from './AdeTaskTitle.vue';
import { TASK_TAG_GLYPH } from './glyphs';
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

const draftRepoIds = computed(() =>
  props.card.rows.filter((r) => r.branch.name === '').map((r) => r.branch.codeRepoId),
);
const selected = computed(() => ui.selectedTaskId === props.card.task.id);
const inRipple = computed(() => rippleIds.value.tasks.has(props.card.task.id));
const headSelected = computed(() => selected.value && ui.selectedBranchId === null);
const p = computed(() => props.card.progress);
// The workflow tip, with the bar's tip and percent when the task shows one (the bar left the row).
const stepTip = computed(() =>
  p.value.showBar
    ? [p.value.workflowTip, p.value.barTip, formatPercent(p.value.percent)].join(' · ')
    : p.value.workflowTip,
);
const segClass = (state: string): string => {
  if (state === 'done') return 'bg-muted-foreground';
  if (state === 'todo') return 'bg-border';
  if (state === 'skipped') return 'bg-field';
  return p.value.bad ? 'bg-tone-red-solid' : 'bg-fg';
};
const boxClass = computed(() => {
  const c = props.card;
  return [
    c.parked ? 'bg-chrome' : '',
    'cursor-grab border border-l-3',
    selected.value ? 'border-focus' : 'border-transparent',
    inRipple.value && !selected.value ? 'outline-1 outline-border' : '',
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
      class="box-border flex min-w-0 flex-1 flex-col overflow-hidden"
      :class="boxClass"
      :style="{ borderLeftColor: card.color }"
      :aria-current="selected || undefined"
      data-testid="ade-card"
      @contextmenu="taskMenu.open"
    >
      <!-- biome-ignore lint/a11y/useSemanticElements: the header holds block content a button cannot. -->
      <div
        v-if="!card.review"
        ref="head"
        class="group/head box-border flex h-board-row w-full flex-col justify-center gap-0.5 px-2"
        :class="headSelected ? 'bg-select' : 'hover:bg-field'"
        role="button"
        tabindex="0"
        :aria-label="t('space.ade.plan.card.open.aria', { title: card.title })"
        data-testid="ade-card-head"
        @click="emit('select')"
        @keydown.enter.self="emit('select')"
        @keydown.space.self.prevent="emit('select')"
      >
        <div class="flex h-4.5 min-w-0 items-center gap-1.5">
          <span class="inline-flex min-w-4.5 shrink-0 justify-center" data-testid="ade-row-lead">
            <AdeTaskSessions
              v-if="card.sessions"
              :task-id="card.task.id"
              :sessions="card.sessions"
            />
          </span>
          <span class="flex min-w-0 items-center"><AdeTaskTitle :card="card" /></span>
          <span
            class="hidden shrink-0 items-center gap-0.5 group-hover/head:flex group-focus-visible/head:flex group-has-[:focus-visible]/head:flex has-[[data-state=open]]:flex has-[[data-pinned]]:flex"
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
                <DropdownMenuItem
                  v-for="c in baseChoices"
                  :key="c.branchId"
                  :disabled="c.disabled"
                  data-testid="ade-card-change-base-item"
                  @select="dialogs.act(c.act)"
                >
                  <VarText :parts="c.parts" />
                </DropdownMenuItem>
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
              <Kbd>{{ formatShortcut('ade.reviewCode') }}</Kbd>
            </TooltipIconButton>
            <TooltipIconButton
              icon="ellipsis"
              :label="t('common.moreActions.tooltip')"
              class="size-4.5 shrink-0"
              data-testid="ade-card-more"
              @click.stop="(ev: MouseEvent) => taskMenu.openAt(ev.currentTarget as HTMLElement)"
            />
          </span>
          <span class="flex-1" />
          <AdeTaskStatusMenu :card="card" />
          <AdeTaskActionButton :card="card" icon />
        </div>
        <div class="flex h-4.5 min-w-0 items-center gap-1.5 overflow-hidden pl-6">
          <template v-if="p.hasWorkflow && !card.parked">
            <AdeTip :text="stepTip">
              <span
                class="inline-flex w-12 shrink-0 items-center gap-0.5"
                data-testid="ade-step-segs"
              >
                <span
                  v-for="seg in p.segments"
                  :key="seg.id"
                  class="h-1.5 flex-1 rounded-kira-sm"
                  :class="segClass(seg.state)"
                />
              </span>
            </AdeTip>
            <AdeStepMenu :card="card" />
          </template>
          <AdeEstimateChip v-if="!card.review" :card="card" />
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
          <AdeAttention v-if="card.attention" :tip="card.attention" :item="card.attentionItem" />
          <span class="inline-flex @max-[380px]:hidden"
            ><AdeAutomationChip :task-id="card.task.id" /></span
          >
          <span class="inline-flex @max-[380px]:hidden"
            ><AdeLeaseChip :task-id="card.task.id" /></span
          >
          <AdeUpdatesPopover v-if="taskUpdates.length" :task-id="card.task.id" />
          <AdeParentChip
            v-if="card.parent"
            :parent="card.parent"
            icon-only
            @select="card.parent && ui.select(card.parent.id)"
          />
          <Button
            v-if="card.task.branchRequestId"
            type="button"
            variant="toolbar"
            size="inline"
            class="h-4.5 shrink-0 rounded-kira-sm px-1 text-kira-sm text-tone-amber"
            data-testid="ade-card-branch-request"
            @click.stop="approvalsStore.open(`branch.request:${card.task.branchRequestId}`)"
          >
            {{ t('space.ade.taskTab.branchRequest.label') }}
          </Button>
          <AdeTip :text="card.meta">
            <span
              class="min-w-0 flex-1 truncate text-kira-sm leading-3.5 text-subtle @max-[400px]:hidden"
              data-testid="ade-card-meta"
              >{{
                card.meta
              }}</span
            >
          </AdeTip>
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
        @task-menu="taskMenu.open"
      />
      <AdeTaskChildren v-if="card.orchestrator" :card="card" />
    </Card>
  </div>
</template>
