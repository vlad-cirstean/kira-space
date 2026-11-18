<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import { computed } from 'vue';
import AdeChip from '../AdeChip.vue';
import { useMarkTrackedSeen } from './queries';
import { useTaskUpdates } from './useTaskUpdates';

const props = withDefaults(defineProps<{ taskId: string; framed?: boolean }>(), { framed: true });

const { byTask, describe } = useTaskUpdates();
const markSeen = useMarkTrackedSeen();
const updates = computed(() => byTask.value.get(props.taskId) ?? []);

function seen(itemKeys: string[]): void {
  markSeen.mutate({ taskId: props.taskId, itemKeys });
}
</script>

<template>
  <div
    v-if="updates.length"
    class="flex flex-col gap-1.5 px-2 py-1.5"
    :class="{ 'border border-border': framed }"
    data-testid="ade-task-updates"
  >
    <div class="flex items-center gap-2">
      <AdeChip :label="t('space.ade.updates.strip.title')" tone="blue" />
      <Button
        variant="toolbar"
        size="kira"
        class="ml-auto"
        data-testid="ade-task-updates-seen-all"
        @click="seen([])"
        ><CodiconIcon name="check" :size="13" />
        {{ t('space.ade.updates.markAllSeen.label') }}</Button
      >
    </div>
    <div
      v-for="u in updates"
      :key="u.itemKey"
      class="flex items-center gap-2"
      :data-testid="`ade-task-update-${u.itemKey}`"
    >
      <span class="min-w-0 flex-1 truncate" :class="rowMeta">{{ describe(u) }}</span>
      <Button
        variant="toolbar"
        size="kira"
        :data-testid="`ade-task-update-seen-${u.itemKey}`"
        @click="seen([u.itemKey])"
        ><CodiconIcon name="check" :size="13" />{{ t('space.ade.updates.markSeen.label') }}</Button
      >
    </div>
  </div>
</template>
