<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { SORTABLE_GHOST_CLASS } from '@workbench/behaviour/sortableReorder';
import { computed, ref } from 'vue';
import { useDraggable } from 'vue-draggable-plus';
import type { TimelineBand } from '../board/timeline';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import AdeTaskCard from './AdeTaskCard.vue';
import type { BranchRowModel, CardModel } from './usePlanModel';

// One day of the timeline: ruler, history rows, overdue and overflow strips, the day's cards and
// the continuation rows of multi-day tasks. Plan writes are the parent's: this only emits intent.
const ui = useAdeBoardUiStore();
const props = defineProps<{
  band: TimelineBand;
  cards: CardModel[];
  /** Continuation rows: task title plus the span facts. */
  spans: {
    taskId: string;
    title: string;
    color: string;
    note: string;
    merges: boolean;
    tip: string;
  }[];
  history: { key: string; merged: boolean; meta: string; title: string }[];
  overdueNote: string;
  overflowNote: string;
  overflowLabel: string;
  dragOver: boolean;
}>();
const emit = defineEmits<{
  select: [taskId: string];
  rollover: [];
  overflowMove: [];
  dayMenu: [ev: MouseEvent];
  forcePush: [row: BranchRowModel];
  dragStart: [taskId: string];
  dragEnd: [];
}>();

// SortableJS only drags direct children of its list, so each band's drop area is its own list.
// `sort: false` and no group pull/put mean it never moves Vue-managed DOM; the parent resolves the
// drop target from the pointer instead.
const dropEl = ref<HTMLElement | null>(null);
const ghostClass = SORTABLE_GHOST_CLASS;
useDraggable(dropEl, {
  draggable: '[data-ade-task]',
  sort: false,
  group: { name: 'ade-plan', pull: false, put: false },
  forceFallback: true,
  fallbackOnBody: true,
  fallbackTolerance: 4,
  ghostClass,
  onStart: (evt) => emit('dragStart', evt.item.dataset.taskId ?? ''),
  onEnd: () => emit('dragEnd'),
});

const greyed = computed(() => props.band.weekend || props.band.dayOff);
const empty = computed(
  () => props.cards.length === 0 && props.spans.length === 0 && props.history.length === 0,
);
const overdue = computed(() => props.band.overdueTaskIds.length > 0);
const over = computed(() => props.band.overflowTaskIds.length > 0 && !greyed.value);
const accepts = computed(() => !props.band.isPast && !props.band.dayOff);
const highlight = computed(() => props.dragOver && accepts.value);

const rulerBorderClass = computed(() => {
  if (props.band.isToday) return 'border-r-tone-amber-solid';
  return props.band.isPast || greyed.value ? 'border-r-border' : 'border-r-border-strong';
});
const dotClass = computed(() => {
  if (props.band.isToday) return 'border-tone-amber-solid bg-tone-amber-solid';
  return props.band.isPast || greyed.value
    ? 'border-border bg-chrome'
    : 'border-border-strong bg-chrome';
});
const labelClass = computed(() => {
  if (greyed.value) return 'text-disabled';
  if (overdue.value) return 'text-tone-amber';
  return empty.value || props.band.isPast ? 'text-subtle' : 'text-fg';
});
const subClass = computed(() => {
  if (greyed.value) return 'text-disabled';
  return props.band.overflowHours > 0 && !props.band.isLater
    ? 'text-tone-red'
    : 'text-muted-foreground';
});
const sub = computed(() => {
  if (props.band.dayOff) return t('common.off.status');
  return props.band.hours ? t('format.relative.unitHour', { count: props.band.hours }) : '';
});
const solidBorder = computed(() => props.band.isMonday || props.band.isLater || props.band.isToday);
</script>

<template>
  <!-- biome-ignore lint/a11y/noStaticElementInteractions: right-click only; the band holds nested interactive rows. -->
  <div
    class="flex border-t"
    :class="solidBorder ? 'border-t-border-strong' : 'border-t-border'"
    data-testid="ade-day-band"
    data-ade-band
    :data-weekend="band.weekend || undefined"
    :data-ade-day="band.key"
    @contextmenu.prevent="emit('dayMenu', $event)"
  >
    <div
      class="relative box-border w-15 shrink-0 border-r-2 text-right"
      :class="['py-1.5 pl-0 pr-2', rulerBorderClass]"
    >
      <span
        class="absolute box-border rounded-full border-2"
        :class="[empty ? '-right-1 top-1.5 size-1.5' : '-right-1.5 top-2.5 size-2.5', dotClass]"
      />
      <div
        class="whitespace-nowrap"
        :class="['text-kira-md font-medium', band.dayOff ? 'line-through' : '', labelClass]"
        data-testid="ade-band-label"
      >
        {{ band.label }}
      </div>
      <div class="text-kira-sm" :class="subClass">{{ sub }}</div>
    </div>
    <div
      class="box-border flex min-w-0 flex-1 flex-col gap-2 rounded-kira"
      :class="[
        empty ? (greyed ? 'min-h-4' : 'min-h-control') : 'pb-2 pl-2 pt-2',
        highlight ? 'outline outline-1 outline-focus' : '',
        band.weekend
          ? 'bg-[repeating-linear-gradient(135deg,var(--color-border)_0_1px,transparent_1px_6px)]'
          : '',
      ]"
      ref="dropEl"
      data-testid="ade-band-drop"
    >
      <Button
        size="kira"
        v-for="h in history"
        :key="h.key"
        variant="toolbar"
        class="h-row w-auto justify-start gap-2 px-2 font-normal data-[selected]:bg-select"
        :data-selected="ui.selectedTaskId === h.key || undefined"
        :data-task-id="h.key"
        data-testid="ade-history-row"
        @click="ui.select(h.key)"
      >
        <CodiconIcon
          :name="h.merged ? 'check' : 'archive'"
          :size="12"
          class="shrink-0 text-muted-foreground"
        />
        <span class="min-w-0 flex-1 truncate text-left font-medium text-fg">{{ h.title }}</span>
        <span class="shrink-0 whitespace-nowrap text-kira-sm text-muted-foreground">{{
          h.meta
        }}</span>
      </Button>
      <div v-if="overdue" class="flex items-center gap-2 text-kira-sm text-tone-amber">
        <span>{{ overdueNote }}</span>
        <Button
          variant="dialog"
          size="kira"
          data-testid="ade-band-rollover"
          @click="emit('rollover')"
        >
          {{ t('space.ade.board.taskMenu.moveToday.label') }}
        </Button>
      </div>
      <div v-if="over" class="flex items-center gap-2 text-kira-sm text-tone-red">
        <span>{{ overflowNote }}</span>
        <Button
          variant="dialog-danger"
          size="kira"
          class="max-w-90 truncate"
          data-testid="ade-band-overflow-move"
          @click="emit('overflowMove')"
        >
          {{ overflowLabel }}
        </Button>
      </div>
      <template v-for="(card, i) in cards" :key="card.task.id">
        <AdeTaskCard
          :class="i > 0 ? 'mt-2' : undefined"
          :card="card"
          @select="emit('select', card.task.id)"
          @force-push="(row) => emit('forcePush', row)"
        />
      </template>
      <Button
        size="kira"
        v-for="s in spans"
        :key="s.taskId"
        variant="toolbar"
        class="h-row w-auto justify-start gap-2 border border-border px-2 font-normal data-[selected]:bg-select"
        :data-task-id="s.taskId"
        :data-selected="ui.selectedTaskId === s.taskId || undefined"
        data-testid="ade-span-row"
        @click="emit('select', s.taskId)"
      >
        <CodiconIcon name="indent" :size="12" class="shrink-0 text-subtle" />
        <span class="size-2 shrink-0 rounded-kira-xs" :style="{ background: s.color }" />
        <span class="shrink-0 whitespace-nowrap text-kira-sm text-subtle">{{ s.note }}</span>
        <span class="min-w-0 truncate font-medium text-fg">{{ s.title }}</span>
      </Button>
    </div>
  </div>
</template>
