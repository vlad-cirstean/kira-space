<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { paneFrame } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import { useTaskAction } from '@workbench/behaviour/asyncAction';
import { useKeptScroll } from '@workbench/behaviour/viewState';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { computed, useTemplateRef, watch } from 'vue';
import { shelfIds } from '../board/shelf';
import { usePlanModel } from '../plan/usePlanModel';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import AdeShelfSection from './AdeShelfSection.vue';

// The Later tab: tasks taken off the Plan, in two sections. Later is next in line; Backlog holds
// work that may never be picked up. The task panel opens beside the lists.
const ui = useAdeBoardUiStore();
const { model } = usePlanModel();
const surface = useTaskAction();
const error = surface.error;

function cardsOf(shelf: 'later' | 'backlog') {
  const m = model.value;
  if (!m) return [];
  return shelfIds(m.board, shelf).flatMap((id) => m.cardFor(id) ?? []);
}
const later = computed(() => cardsOf('later'));
const backlog = computed(() => cardsOf('backlog'));

// A selection that is not on a shelf (a Plan task) gives way to the first shelf task.
watch(
  [later, backlog],
  ([l, b]) => {
    const all = [...l, ...b];
    if (all.length && !all.some((c) => c.task.id === ui.selectedTaskId))
      ui.select(all[0]?.task.id ?? null);
  },
  { immediate: true },
);

const bodyEl = useTemplateRef<HTMLElement>('bodyEl');
useKeptScroll('ade:later', bodyEl);
</script>

<template>
  <div :class="cn(paneFrame, 'flex min-w-0 flex-1 flex-col')" data-testid="ade-later">
    <PanelHeader>{{ t('common.later.label') }}</PanelHeader>
    <div class="flex shrink-0 flex-col gap-2 p-3">
      <FieldDescription>{{ t('space.ade.later.intro.hint') }}</FieldDescription>
      <Alert v-if="error" variant="error" data-testid="ade-later-error"
        ><AlertDescription>{{ error }}</AlertDescription></Alert
      >
    </div>
    <div ref="bodyEl" class="flex min-h-0 flex-1 flex-col overflow-auto pb-3">
      <AdeShelfSection shelf="later" :cards="later" @run="surface.run" />
      <AdeShelfSection shelf="backlog" :cards="backlog" @run="surface.run" />
    </div>
  </div>
</template>
