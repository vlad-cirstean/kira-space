<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import AdeChip from '../AdeChip.vue';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { type CardModel, usePlanModel } from './usePlanModel';

// An orchestrator's children: one row per live child, then one muted line for the archived ones.
// A click selects the child; archived children open from a short list.
const props = withDefaults(defineProps<{ card: CardModel; testid?: string }>(), {
  testid: 'ade-card-children',
});
const ui = useAdeBoardUiStore();
const { model } = usePlanModel();

const archived = computed(() =>
  props.card.archivedChildren.map((id) => ({
    id,
    title: model.value?.board.history.find((h) => h.taskId === id)?.title ?? id,
  })),
);
</script>

<template>
  <div
    v-if="card.children.length || archived.length"
    class="flex flex-col border-t border-border"
    :data-testid="testid"
  >
    <Button
      v-for="child in card.children"
      :key="child.id"
      type="button"
      variant="toolbar"
      size="status"
      class="w-full justify-start gap-2 rounded-none text-left"
      :aria-label="t('space.ade.plan.card.open.aria', { title: child.title })"
      data-testid="ade-card-child"
      :data-task-id="child.id"
      :data-finished="child.finished || undefined"
      @click.stop="ui.select(child.id)"
    >
      <CodiconIcon
        :name="child.finished ? 'pass-filled' : 'circle-outline'"
        :size="12"
        :class="child.finished ? 'text-tone-green' : 'text-muted-foreground'"
        data-testid="ade-card-child-mark"
      />
      <span class="min-w-0 flex-1 truncate text-fg" data-testid="ade-card-child-title">{{
        child.title
      }}</span>
      <AdeChip :label="child.status.name" :color="child.status.color" />
      <span v-if="child.step" class="max-w-28 shrink-0 truncate text-muted-foreground">{{
        child.step
      }}</span>
    </Button>
    <Popover v-if="archived.length">
      <PopoverTrigger as-child>
        <Button
          type="button"
          variant="toolbar"
          size="status"
          class="w-full justify-start gap-2 rounded-none text-left text-subtle"
          data-testid="ade-card-archived-children"
          @click.stop
        >
          <CodiconIcon name="archive" :size="12" />
          {{ t('space.ade.orchestrator.archivedChildren.label', { count: archived.length }) }}
        </Button>
      </PopoverTrigger>
      <PopoverContent class="w-80 gap-0 p-1" align="start">
        <Button
          v-for="a in archived"
          :key="a.id"
          type="button"
          variant="toolbar"
          size="status"
          class="w-full justify-start rounded-none text-left"
          data-testid="ade-card-archived-child"
          :data-task-id="a.id"
          @click.stop="ui.select(a.id)"
        >
          <span class="truncate">{{ a.title }}</span>
        </Button>
      </PopoverContent>
    </Popover>
  </div>
</template>
