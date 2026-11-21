<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { rowIndent, rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import RelativeTime from '@workbench/components/RelativeTime.vue';
import { computed } from 'vue';
import AdeEstimateChip from '../plan/AdeEstimateChip.vue';
import AdeTaskKey from '../plan/AdeTaskKey.vue';
import AdeTaskLine from '../plan/AdeTaskLine.vue';
import AdeTaskSessions from '../plan/AdeTaskSessions.vue';
import AdeTaskStatusMenu from '../plan/AdeTaskStatusMenu.vue';
import AdeTaskTitle from '../plan/AdeTaskTitle.vue';
import type { CardModel } from '../plan/usePlanModel';
import { useTaskMenu } from '../plan/useTaskMenu';
import { useAdeBoardUiStore } from '../state/adeBoardUi';

// One task on the Later or Backlog shelf. Drag it to reorder or to the other shelf; the menu
// moves it back to the Plan.
const props = defineProps<{ card: CardModel }>();

const ui = useAdeBoardUiStore();
const selected = computed(() => ui.selectedTaskId === props.card.task.id);
const menu = useTaskMenu(() => props.card);
function openMore(ev: MouseEvent): void {
  if (ev.currentTarget instanceof Element) menu.openAt(ev.currentTarget);
}
const facts = computed(() =>
  [
    props.card.meta,
    props.card.repos.join(' · '),
    props.card.task.notes ? t('space.common.notes.label') : '',
  ]
    .filter(Boolean)
    .join(' · '),
);
</script>

<template>
  <!-- biome-ignore lint/a11y/useSemanticElements: the row holds buttons a button cannot. -->
  <div
    :class="
      cn(rowVariants({ layout: 'tree', selected }), 'group/head box-border cursor-grab gap-2 py-1')
    "
    :style="rowIndent(0)"
    role="button"
    tabindex="0"
    :data-selected="selected || undefined"
    data-shelf-row=""
    data-testid="ade-shelf-row"
    :data-task-id="card.task.id"
    @click="ui.select(card.task.id)"
    @keydown.self.enter="ui.select(card.task.id)"
    @keydown.self.space.prevent="ui.select(card.task.id)"
    @contextmenu="menu.open"
  >
    <span class="inline-flex min-w-4.5 shrink-0 justify-center" data-testid="ade-row-lead">
      <AdeTaskSessions v-if="card.sessions" :task-id="card.task.id" :sessions="card.sessions" />
    </span>
    <AdeTaskLine variant="mark" :color="card.color" />
    <span class="flex min-w-0 flex-1 items-center gap-1.5" data-testid="ade-shelf-title">
      <AdeTaskTitle :card="card" />
      <AdeTaskKey :card="card" />
    </span>
    <AdeEstimateChip :card="card" />
    <AdeTaskStatusMenu :card="card" />
    <span
      v-if="facts"
      class="max-w-1/4 shrink-0 truncate text-kira-sm text-subtle"
      data-testid="ade-shelf-facts"
      >{{
        facts
      }}</span
    >
    <span :class="[rowMeta, 'w-16 shrink-0']" data-testid="ade-shelf-ago"
      ><RelativeTime :at="card.task.createdAt" /></span
    >
    <TooltipIconButton
      icon="ellipsis"
      :label="t('common.moreActions.tooltip')"
      class="shrink-0"
      data-testid="ade-shelf-more"
      @click.stop="openMore"
    />
  </div>
</template>
