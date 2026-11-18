<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useKeptScroll } from '@workbench/behaviour/viewState';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { computed, useTemplateRef } from 'vue';
import { usePlanModel } from '../plan/usePlanModel';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import AdeAllSessions from './AdeAllSessions.vue';
import AdeNeedsRow from './AdeNeedsRow.vue';

// Needs you: one list across all tasks (SPEC2 section 11), over a footer with the run counts and the
// All sessions toggle.
const { model } = usePlanModel();
const needs = computed(() => model.value?.needs ?? null);
const ui = useAdeBoardUiStore();

function taskOf(taskId: string): { title: string; color: string } {
  const card = model.value?.cardFor(taskId);
  return { title: card?.title ?? taskId, color: card?.color ?? 'var(--kira-fg-subtle)' };
}

const bodyEl = useTemplateRef<HTMLElement>('bodyEl');
useKeptScroll('ade:needs', bodyEl);
</script>

<template>
  <div
    class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-kira bg-bg"
    data-testid="ade-needs-page"
  >
    <PanelHeader>
      {{ t('common.needsYou.status') }}
      <template v-if="needs && !needs.empty" #actions>
        <span class="normal-case tracking-normal" data-testid="ade-needs-summary">{{
          t('space.ade.needsPage.summary.label', { count: needs.items.length })
        }}</span>
      </template>
    </PanelHeader>
    <div ref="bodyEl" class="flex min-h-0 flex-1 flex-col overflow-auto">
      <template v-if="needs">
        <AdeNeedsRow
          v-for="n in needs.items"
          :key="n.id"
          :item="n"
          :task-title="taskOf(n.taskId).title"
          :task-color="taskOf(n.taskId).color"
        />
        <EmptyState
          v-if="needs.empty"
          icon="check"
          data-testid="ade-needs-empty"
          :title="t('space.ade.needsPage.empty.emptyHint')"
        />
        <div class="flex items-center gap-3 px-3 py-2 text-kira-md text-muted-foreground">
          <span class="flex-1" data-testid="ade-needs-footer">{{ needs.footer }}</span>
          <Button
            variant="toolbar"
            size="kira"
            data-testid="ade-needs-all"
            @click="ui.showAllSessions = !ui.showAllSessions"
            ><CodiconIcon name="eye" :size="13" />
            {{
              ui.showAllSessions
                ? t('space.ade.needsPage.hideAll.label')
                : t('space.ade.needsPage.showAll.label')
            }}
          </Button>
        </div>
      </template>
      <AdeAllSessions v-if="ui.showAllSessions" />
    </div>
  </div>
</template>
