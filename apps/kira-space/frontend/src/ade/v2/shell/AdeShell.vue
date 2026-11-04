<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import { Tabs, TabsList, TabsTrigger, tabChipVariants } from '@theme/components/ui/tabs';
import { t } from '@theme/i18n';
import { computed, onMounted, onScopeDispose } from 'vue';
import { control } from '../../../bridge/control';
import AdeAbortRebaseDialog from '../AdeAbortRebaseDialog.vue';
import AdeAddPopover from '../AdeAddPopover.vue';
import AdeAgentsPage from '../agents/AdeAgentsPage.vue';
import AdeAgentDialog from '../dialog/AdeAgentDialog.vue';
import AdeLaterPage from '../later/AdeLaterPage.vue';
import AdeNeedsPage from '../needs/AdeNeedsPage.vue';
import AdePanel from '../panel/AdePanel.vue';
import AdePlanView from '../plan/AdePlanView.vue';
import { usePlanModel } from '../plan/usePlanModel';
import AdeBranchRequestDialog from '../request/AdeBranchRequestDialog.vue';
import { useReviewCode } from '../review/useReviewCode';
import AdeRunDialog from '../run/AdeRunDialog.vue';
import AdeTakeOverDialog from '../sessions/AdeTakeOverDialog.vue';
import AdeStageDialog from '../stage/AdeStageDialog.vue';
import { useAdeAgentsUiStore } from '../state/adeAgentsUi';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeWorkflowsUiStore } from '../state/adeWorkflowsUi';
import AdeWorkflowsPage from '../workflows/AdeWorkflowsPage.vue';
import AdeCaptureBox from './AdeCaptureBox.vue';

// Top bar (tabs, capture box, Add) over the active tab; the Plan opens the panel beside it.
const ui = useAdeBoardUiStore();
const wfUi = useAdeWorkflowsUiStore();
const agentsUi = useAdeAgentsUiStore();

async function onView(view: unknown): Promise<void> {
  if (view === ui.view) return;
  if (ui.view === 'workflows' && !(await wfUi.leave())) return;
  if (ui.view === 'agents' && !(await agentsUi.leave())) return;
  ui.view = view as typeof ui.view;
}
const { model } = usePlanModel();
/** The panel on the Later tab opens only for a task that sits on a shelf. */
const shelfSelected = computed(() => {
  const id = ui.selectedTaskId;
  return id !== null && (model.value?.cardFor(id)?.shelf ?? 'plan') !== 'plan';
});
const needsCount = computed(() => model.value?.needs.badge ?? 0);

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
  offReviewCode = control.onReviewCode(onReviewCode);
});
onScopeDispose(() => offReviewCode?.());
</script>

<template>
  <Tabs
    :model-value="ui.view"
    class="flex min-h-0 flex-1 flex-col gap-0"
    data-testid="ade-shell"
    @update:model-value="onView"
  >
    <nav
      class="grid h-tabbar shrink-0 grid-cols-[1fr_auto_1fr] items-center gap-0.5 bg-chrome px-1"
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
          value="plan"
          :class="tabChipVariants({ active: ui.view === 'plan', size: 'wide' })"
          data-testid="ade-tab-plan"
        >
          {{ t('space.common.plan.label') }}
        </TabsTrigger>
        <TabsTrigger
          value="later"
          :class="tabChipVariants({ active: ui.view === 'later', size: 'wide' })"
          data-testid="ade-tab-later"
        >
          {{ t('common.later.label') }}
        </TabsTrigger>
      </TabsList>
      <div class="mx-2 flex items-center gap-2">
        <AdeCaptureBox />
        <AdeAddPopover />
      </div>
      <TabsList class="justify-self-end">
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
      <AdeWorkflowsPage v-else-if="ui.view === 'workflows'" />
      <template v-else>
        <AdePlanView />
        <AdePanel v-if="ui.selectedTaskId" />
      </template>
    </div>
    <AdeRunDialog />
    <AdeAgentDialog />
    <AdeAbortRebaseDialog />
    <AdeTakeOverDialog />
    <AdeStageDialog :routed="false" />
    <AdeBranchRequestDialog :routed="false" />
  </Tabs>
</template>
