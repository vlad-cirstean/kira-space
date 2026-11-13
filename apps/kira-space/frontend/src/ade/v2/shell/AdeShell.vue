<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import { Tabs, TabsList, TabsTrigger, tabChipVariants } from '@theme/components/ui/tabs';
import { t } from '@theme/i18n';
import { computed, defineAsyncComponent, onMounted, onScopeDispose } from 'vue';
import { control } from '../../../bridge/control';
import AdeAddPopover from '../AdeAddPopover.vue';
import { shelfOf } from '../board/shelf';
import { prefetchNotesEditor } from '../notes/asyncEditor';
import AdePanel from '../panel/AdePanel.vue';
import AdePlanView from '../plan/AdePlanView.vue';
import { usePlanModel } from '../plan/usePlanModel';
import { useReviewCode } from '../review/useReviewCode';
import { type AdeView, useAdeBoardUiStore } from '../state/adeBoardUi';
import { switchAdeView } from '../state/adeView';
import AdeCaptureBox from './AdeCaptureBox.vue';

// Every page but the Plan and every dialog loads as its own chunk: the entry bundle skips them. Dialog
// state lives in stores, so one mounted before its chunk lands opens as soon as it arrives.
const AdeAbortRebaseDialog = defineAsyncComponent(() => import('../AdeAbortRebaseDialog.vue'));
const AdeAgentsPage = defineAsyncComponent(() => import('../agents/AdeAgentsPage.vue'));
const AdeAgentDialog = defineAsyncComponent(() => import('../dialog/AdeAgentDialog.vue'));
const AdeLaterPage = defineAsyncComponent(() => import('../later/AdeLaterPage.vue'));
const AdeNeedsPage = defineAsyncComponent(() => import('../needs/AdeNeedsPage.vue'));
const AdeReposPage = defineAsyncComponent(() => import('../repos/AdeReposPage.vue'));
const AdeRepoGateDialog = defineAsyncComponent(() => import('../run/AdeRepoGateDialog.vue'));
const AdeRunDialog = defineAsyncComponent(() => import('../run/AdeRunDialog.vue'));
const AdeTakeOverDialog = defineAsyncComponent(() => import('../sessions/AdeTakeOverDialog.vue'));
const AdeStageDialog = defineAsyncComponent(() => import('../stage/AdeStageDialog.vue'));
const AdeWorkflowsPage = defineAsyncComponent(() => import('../workflows/AdeWorkflowsPage.vue'));

// Top bar (tabs, capture box, Add) over the active tab; the Plan opens the panel beside it.
const ui = useAdeBoardUiStore();

function onView(view: unknown): void {
  void switchAdeView(view as AdeView);
}
// Reads the needs and the board, not the cards: the full model never builds on a hidden Plan.
const { model, needs, boardQuery } = usePlanModel();
/** The panel on the Later tab opens only for a task that sits on a shelf. */
const shelfSelected = computed(() => {
  const id = ui.selectedTaskId;
  const board = boardQuery.data.value;
  return id !== null && board !== undefined && shelfOf(board, id) !== 'plan';
});
const needsCount = computed(() => needs.value?.badge ?? 0);

// Review code on the focused branch or card, else the selected one.
const reviewCode = useReviewCode();
function reviewTarget(): { taskId?: string; branchId?: string; el?: HTMLElement } {
  const active = document.activeElement;
  const t = active instanceof Element ? active : null;
  const rowEl = t?.closest<HTMLElement>(
    ':is([data-testid="ade-branch-row"],[data-testid="ade-task-branch"])[data-branch-id]',
  );
  if (rowEl) return { branchId: rowEl.dataset.branchId, el: rowEl };
  const cardEl = t?.closest<HTMLElement>('[data-testid="ade-card"][data-task-id]');
  if (cardEl) {
    const head = cardEl.querySelector<HTMLElement>('[data-testid="ade-card-head"]');
    const inside = t instanceof HTMLElement && cardEl.contains(t) ? t : null;
    return { taskId: cardEl.dataset.taskId, el: inside ?? head ?? cardEl };
  }
  if (ui.selectedBranchId) return { branchId: ui.selectedBranchId };
  return { taskId: ui.selectedTaskId ?? undefined };
}
function onReviewCode(): void {
  if (ui.view !== 'plan') return;
  const m = model.value;
  if (!m) return;
  const target = reviewTarget();
  if (target.branchId) {
    const branch = m.board.branches.find((b) => b.id === target.branchId);
    const card = branch ? m.cardFor(branch.taskId) : null;
    const row = card?.rows.find((r) => r.id === target.branchId);
    const choice = row ? reviewCode.choiceOf(row) : null;
    const anchor =
      target.el ??
      document.querySelector<HTMLElement>(
        `[data-testid="ade-branch-row"][data-branch-id="${target.branchId}"]`,
      );
    if (card && choice && !choice.disabled) reviewCode.open(card.task.id, target.branchId, anchor);
    return;
  }
  const card = target.taskId ? m.cardFor(target.taskId) : null;
  if (!card) return;
  const head =
    target.el ??
    document.querySelector<HTMLElement>(
      `[data-testid="ade-card"][data-task-id="${card.task.id}"] [data-testid="ade-card-head"]`,
    );
  reviewCode.openTask(card, head);
}
let offReviewCode: (() => void) | undefined;
onMounted(() => {
  // Off the critical path but early: the stage dialog and notes open on events, not on a hover, and
  // must find the editor chunk loaded for their first frame.
  prefetchNotesEditor();
  offReviewCode = control.onReviewCode(onReviewCode);
});
onScopeDispose(() => offReviewCode?.());
</script>

<template>
  <Tabs
    :model-value="ui.view"
    class="@container flex min-h-0 min-w-0 flex-1 flex-col gap-0"
    data-testid="ade-shell"
    @update:model-value="onView"
  >
    <nav
      class="flex min-h-tabbar shrink-0 flex-wrap items-center justify-between gap-0.5 bg-chrome px-1 py-0.5 @5xl:grid @5xl:h-tabbar @5xl:grid-cols-[1fr_auto_1fr] @5xl:flex-nowrap @5xl:py-0"
      data-testid="ade-shell-bar"
    >
      <TabsList class="justify-self-start">
        <TabsTrigger
          value="needs"
          :class="tabChipVariants({ active: ui.view === 'needs', size: 'wide' })"
          data-testid="ade-tab-needs"
        >
          {{ t('common.needsYou.status') }}
          <Badge v-if="needsCount > 0" variant="count" data-testid="ade-needs-count">{{
            needsCount
          }}</Badge>
        </TabsTrigger>
        <TabsTrigger
          value="later"
          :class="tabChipVariants({ active: ui.view === 'later', size: 'wide' })"
          data-testid="ade-tab-later"
        >
          {{ t('common.later.label') }}
        </TabsTrigger>
        <TabsTrigger
          value="plan"
          :class="tabChipVariants({ active: ui.view === 'plan', size: 'wide' })"
          data-testid="ade-tab-plan"
        >
          {{ t('space.common.plan.label') }}
        </TabsTrigger>
      </TabsList>
      <div
        class="mx-2 flex min-w-0 flex-1 basis-56 items-center justify-center gap-2 @5xl:flex-none"
      >
        <AdeCaptureBox />
        <AdeAddPopover />
      </div>
      <TabsList class="justify-self-end">
        <TabsTrigger
          value="repos"
          :class="tabChipVariants({ active: ui.view === 'repos', size: 'wide' })"
          data-testid="ade-tab-repos"
        >
          {{ t('common.module.repositories.label') }}
        </TabsTrigger>
        <TabsTrigger
          value="agents"
          :class="tabChipVariants({ active: ui.view === 'agents', size: 'wide' })"
          data-testid="ade-tab-agents"
        >
          {{ t('space.ade.agents.tab.label') }}
        </TabsTrigger>
        <TabsTrigger
          value="workflows"
          :class="tabChipVariants({ active: ui.view === 'workflows', size: 'wide' })"
          data-testid="ade-tab-workflows"
        >
          {{ t('space.common.workflows.label') }}
        </TabsTrigger>
      </TabsList>
    </nav>
    <div class="flex min-h-0 flex-1 gap-0.5">
      <template v-if="ui.view === 'later'">
        <AdeLaterPage />
        <AdePanel v-if="shelfSelected" />
      </template>
      <AdeNeedsPage v-else-if="ui.view === 'needs'" />
      <AdeAgentsPage v-else-if="ui.view === 'agents'" />
      <AdeReposPage v-else-if="ui.view === 'repos'" />
      <AdeWorkflowsPage v-else-if="ui.view === 'workflows'" />
      <template v-else>
        <AdePlanView />
        <AdePanel v-if="ui.selectedTaskId" />
      </template>
    </div>
    <AdeRunDialog />
    <AdeRepoGateDialog />
    <AdeAgentDialog />
    <AdeAbortRebaseDialog />
    <AdeTakeOverDialog />
    <AdeStageDialog :routed="false" />
  </Tabs>
</template>
