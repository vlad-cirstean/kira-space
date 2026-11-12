<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { rowIndent, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { useRelativeTime } from '@workbench/behaviour/format';
import { computed } from 'vue';
import AdeTaskSessions from '../plan/AdeTaskSessions.vue';
import AdeTaskStatusMenu from '../plan/AdeTaskStatusMenu.vue';
import { CARD_LEAD, CARD_LEAD_AGENTS, CARD_LEAD_ATTENTION, CARD_STATUS } from '../plan/cardColumns';
import type { CardModel } from '../plan/usePlanModel';
import { useTaskMenu } from '../plan/useTaskMenu';
import { useAdeBoardUiStore } from '../state/adeBoardUi';

// One task on the Later or Backlog shelf. Drag it to reorder or to the other shelf; the menu
// moves it back to the Plan.
const props = defineProps<{ card: CardModel }>();

const ui = useAdeBoardUiStore();
const menu = useTaskMenu(() => props.card);
const ago = useRelativeTime(() => props.card.task.createdAt);
function openMore(ev: MouseEvent): void {
  if (ev.currentTarget instanceof Element) menu.openAt(ev.currentTarget);
}
const facts = computed(() =>
  [props.card.meta, props.card.task.notes ? t('space.common.notes.label') : '']
    .filter(Boolean)
    .join(' · '),
);
</script>

<template>
  <!-- biome-ignore lint/a11y/useSemanticElements: the row holds buttons a button cannot. -->
  <div
    :class="
      cn(
        rowVariants({ layout: 'tree', selected: card.selected }),
        'box-border cursor-grab gap-2 border-l-3 py-1',
      )
    "
    :style="{ ...rowIndent(0), borderLeftColor: card.color }"
    role="button"
    tabindex="0"
    :data-selected="card.selected || undefined"
    data-shelf-row=""
    data-testid="ade-shelf-row"
    :data-task-id="card.task.id"
    @click="ui.select(card.task.id)"
    @keydown.self.enter="ui.select(card.task.id)"
    @keydown.self.space.prevent="ui.select(card.task.id)"
    @contextmenu="menu.open"
  >
    <span :class="CARD_LEAD" data-testid="ade-row-lead">
      <span :class="CARD_LEAD_ATTENTION">
        <AdeTaskSessions
          v-if="card.sessions?.state === 'input'"
          :task-id="card.task.id"
          :sessions="card.sessions"
        />
      </span>
      <span :class="CARD_LEAD_AGENTS">
        <AdeTaskSessions
          v-if="card.sessions && card.sessions.state !== 'input'"
          :task-id="card.task.id"
          :sessions="card.sessions"
        />
      </span>
    </span>
    <span
      class="min-w-0 flex-1 truncate"
      :class="card.parked ? 'text-muted-foreground' : ''"
      data-testid="ade-shelf-title"
      >{{
        card.title
      }}</span
    >
    <span :class="CARD_STATUS"><AdeTaskStatusMenu :card="card" /></span>
    <span
      v-if="facts"
      class="max-w-2/5 shrink-0 truncate text-kira-sm text-subtle"
      data-testid="ade-shelf-facts"
      >{{
        facts
      }}</span
    >
    <span class="w-16 shrink-0 text-kira-sm text-subtle" data-testid="ade-shelf-ago">{{
      ago
    }}</span>
    <TooltipIconButton
      icon="ellipsis"
      :label="t('common.moreActions.tooltip')"
      class="shrink-0"
      data-testid="ade-shelf-more"
      @click.stop="openMore"
    />
  </div>
</template>
