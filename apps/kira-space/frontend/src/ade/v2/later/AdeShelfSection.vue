<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { t } from '@theme/i18n';
import { moveId, useSortableReorder } from '@workbench/behaviour/sortableReorder';
import { computed, useTemplateRef } from 'vue';
import type { CardModel } from '../plan/usePlanModel';
import { useShelveTask } from '../queries';
import type { Shelf } from '../wire';
import AdeShelfRow from './AdeShelfRow.vue';

// One shelf of the Later tab: a heading and a drag list. Both shelves share a Sortable group, so a
// drop from the other shelf lands here.
const props = defineProps<{ shelf: Shelf; cards: readonly CardModel[] }>();
// The parent runs the write on its error surface.
const emit = defineEmits<{ run: [task: () => Promise<unknown>] }>();

const shelve = useShelveTask();
const list = useTemplateRef<HTMLElement>('list');
const ids = computed(() => props.cards.map((c) => c.task.id));

function put(taskId: string, beforeId: string): void {
  emit('run', () => shelve.mutateAsync({ taskId, shelf: props.shelf, beforeId }));
}

useSortableReorder(
  list,
  () => ids.value,
  (fromId, toId) => {
    const next = moveId(ids.value, fromId, toId);
    put(fromId, next[next.indexOf(fromId) + 1] ?? '');
  },
  {
    draggable: '[data-shelf-row]',
    filter: 'button',
    direction: 'vertical',
    keyboard: true,
    group: 'ade-shelf',
    itemId: (el) => el.dataset.taskId ?? null,
    onAdd: (id, beforeId) => put(id, beforeId ?? ''),
  },
);
</script>

<template>
  <section class="flex min-h-0 flex-col" :data-testid="`ade-shelf-${shelf}`">
    <div class="shrink-0 px-1.5 pt-3">
      <SectionHeading
        :label="shelf === 'later' ? t('common.later.label') : t('space.common.backlog.label')"
      />
    </div>
    <div ref="list" class="flex min-h-16 flex-col" data-testid="ade-shelf-list">
      <AdeShelfRow v-for="card in cards" :key="card.task.id" :card="card" />
      <InlineState v-if="!cards.length" kind="empty">{{
        shelf === 'later'
          ? t('space.ade.later.laterEmpty.empty')
          : t('space.common.backlogEmpty.empty')
      }}</InlineState>
    </div>
  </section>
</template>
