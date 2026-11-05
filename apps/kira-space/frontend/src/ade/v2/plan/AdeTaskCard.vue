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
import { computed, useTemplateRef } from 'vue';
import AdeTip from '../AdeTip.vue';
import AdeAutomationChip from '../automation/AdeAutomationChip.vue';
import { changeBaseChoices } from '../board/changeBase';
import { taskReviewTip } from '../board/reviewCode';
import { useReviewCode } from '../review/useReviewCode';
import AdeTaskActionButton from '../run/AdeTaskActionButton.vue';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import AdeUpdatesDot from '../updates/AdeUpdatesDot.vue';
import { useTaskUpdates } from '../updates/useTaskUpdates';
import AdeAttention from './AdeAttention.vue';
import AdeBranchRow from './AdeBranchRow.vue';
import AdeTagIcon from './AdeTagIcon.vue';
import AdeTaskSessions from './AdeTaskSessions.vue';
import { TASK_TAG_GLYPH } from './glyphs';
import { type BranchRowModel, type CardModel, usePlanModel } from './usePlanModel';
import { useTaskMenu } from './useTaskMenu';

const props = defineProps<{ card: CardModel }>();
const emit = defineEmits<{ select: []; forcePush: [row: BranchRowModel] }>();

const ui = useAdeBoardUiStore();
const dialogs = useAdeDialogsStore();
const { model } = usePlanModel();
const { byTask, describe } = useTaskUpdates();
const taskUpdates = computed(() => byTask.value.get(props.card.task.id) ?? []);
const taskMenu = useTaskMenu(() => props.card);
const reviewCode = useReviewCode();
const choices = computed(() => reviewCode.choicesOf(props.card));
const reviewTip = computed(() => taskReviewTip(choices.value));

const head = useTemplateRef<HTMLElement>('head');
useShortcut(head, ['menu.open'], () => {
  if (head.value) taskMenu.openAt(head.value);
});

// One branch opens Change base at once; several ask which.
const baseChoices = computed(() => {
  const m = model.value;
  return m
    ? changeBaseChoices(props.card, { graph: m.view.graph, after: m.view.after, board: m.board })
    : [];
});

const headSelected = computed(() => props.card.selected && ui.selectedBranchId === null);
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
    c.parked ? 'bg-chrome' : '',
    'cursor-grab border border-l-3',
    c.selected ? 'border-focus' : 'border-transparent',
    c.ripple && !c.selected ? 'outline-1 outline-border' : '',
  ];
});
</script>

<template>
  <div
    class="flex items-start gap-2"
    data-ade-task=""
    data-testid="ade-task"
    :data-task-id="card.task.id"
    :data-selected="card.selected || undefined"
  >
    <Card
      variant="section"
      :data-task-id="card.task.id"
      class="box-border flex min-w-0 flex-1 flex-col overflow-hidden"
      :class="boxClass"
      :style="{ borderLeftColor: card.color }"
      :aria-current="card.selected || undefined"
      data-testid="ade-card"
      @contextmenu="taskMenu.open"
    >
      <!-- biome-ignore lint/a11y/useSemanticElements: the header holds block content a button cannot. -->
      <div
        v-if="!card.review"
        ref="head"
        class="box-border flex h-17 w-full flex-col justify-center gap-1 px-2 py-1.5"
        :class="headSelected ? 'bg-select' : 'hover:bg-field'"
        role="button"
        tabindex="0"
        data-testid="ade-card-head"
        @click="emit('select')"
        @keydown.enter.self="emit('select')"
        @keydown.space.self.prevent="emit('select')"
      >
        <div class="flex h-4.5 min-w-0 items-center gap-1.5">
          <span
            class="box-border size-3 shrink-0 rounded-kira-sm"
            :class="card.parked ? 'border-2' : ''"
            :style="card.parked ? { borderColor: card.color } : { background: card.color }"
            data-testid="ade-card-dot"
          />
          <AdeTagIcon
            v-if="card.tag"
            :glyph="TASK_TAG_GLYPH[card.tag.kind]"
            :kind="card.tag.kind"
            :label="card.tag.label"
            :tip="card.tag.tip"
            :tone="card.tag.tone"
            :color="card.tag.color"
          />
          <template v-if="p.hasWorkflow && !card.parked">
            <AdeTip :text="p.workflowTip">
              <span
                class="inline-flex w-20 shrink-0 items-center gap-0.5"
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
            <span
              class="max-w-36 shrink-0 truncate text-kira-sm"
              :class="p.bad ? 'text-tone-red' : 'text-muted-foreground'"
              data-testid="ade-step-label"
              >{{
                p.label
              }}</span
            >
            <AdeTip v-if="p.showBar" :text="p.barTip">
              <span class="inline-flex shrink-0 items-center gap-1">
                <span class="inline-block h-1.25 w-11.5 overflow-hidden rounded-kira-sm bg-border">
                  <span
                    class="block h-full"
                    :class="p.bad ? 'bg-tone-red-solid' : 'bg-muted-foreground'"
                    :style="{ width: `${p.percent}%` }"
                  />
                </span>
                <span class="text-kira-sm text-muted-foreground">{{
                  formatPercent(p.percent)
                }}</span>
              </span>
            </AdeTip>
          </template>
          <AdeTaskSessions v-if="card.sessions" :task-id="card.task.id" :sessions="card.sessions" />
          <AdeAttention v-if="card.attention" :tip="card.attention" :item="card.attentionItem" />
          <AdeAutomationChip :task-id="card.task.id" />
          <AdeUpdatesDot
            v-if="taskUpdates.length"
            :tip="taskUpdates.map(describe).join('; ')"
            testid="ade-task-updated"
          />
          <AdeTip :text="card.meta">
            <span
              class="min-w-0 flex-1 truncate text-kira-sm leading-3.5 text-subtle"
              data-testid="ade-card-meta"
              >{{
                card.meta
              }}</span
            >
          </AdeTip>
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
            :disabled="reviewCode.pending.value || choices.every((c) => c.disabled)"
            data-testid="ade-card-review"
            @click.stop="
              (ev: MouseEvent) => reviewCode.openTask(card, ev.currentTarget as HTMLElement)
            "
          >
            <VarText :parts="reviewTip" />
            <Kbd>{{ formatShortcut('ade.reviewCode') }}</Kbd>
          </TooltipIconButton>
          <AdeTaskActionButton :card="card" icon />
        </div>
        <AdeTip :text="card.title">
          <Button
            type="button"
            variant="link"
            size="inline"
            class="line-clamp-2 block justify-start whitespace-normal break-words rounded-none border-0 text-left text-kira-lg font-medium leading-4"
            :class="card.parked ? 'text-muted-foreground' : 'text-fg'"
            :aria-label="t('space.ade.plan.card.open.aria', { title: card.title })"
            data-testid="ade-card-title"
            @click.stop="emit('select')"
          >
            {{ card.title }}
          </Button>
        </AdeTip>
      </div>
      <AdeBranchRow
        v-for="row in card.rows"
        :key="row.id"
        :row="row"
        :color="card.color"
        :task-row="card.review"
        @force-push="emit('forcePush', row)"
        @task-menu="taskMenu.open"
      />
    </Card>
  </div>
</template>
