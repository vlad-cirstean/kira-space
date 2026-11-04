<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { ResizeHandle } from '@theme/components/ui/resizable';
import { useLiveSize } from '@theme/components/ui/resizable/useLiveSize';
import { t } from '@theme/i18n';
import { useElementSize } from '@vueuse/core';
import { usePersistedState } from '@workbench/behaviour/persisted';
import MainView from '@workbench/components/MainView.vue';
import TabStrip from '@workbench/components/TabStrip.vue';
import { computed, useTemplateRef } from 'vue';
import { useReviewStep } from '../queries';
import { useAdeReviewWindowStore } from '../state/adeReviewWindow';
import AdeReviewAgentPanel from './AdeReviewAgentPanel.vue';
import AdeReviewFiles from './AdeReviewFiles.vue';
import AdeReviewHeader from './AdeReviewHeader.vue';
import AdeReviewRemoved from './AdeReviewRemoved.vue';
import AdeWorkflowReviewPanel from './AdeWorkflowReviewPanel.vue';
import { useReviewContext } from './useReviewContext';
import { useReviewWorkingSync } from './useReviewWorking';

// Three panes for one branch: files to review (left), this window's diff tabs (centre), and the
// questions panel (right).
const store = useAdeReviewWindowStore();
const target = computed(() => store.target);
const { repoLabel } = useReviewContext(() => store.target as NonNullable<typeof store.target>);

useReviewWorkingSync(() => store.target);
const files = useTemplateRef('files');

// The Workflow review tab shows while the branch has a review step run; it opens first while that
// run waits, and the user's own pick wins afterwards.
const reviewStep = useReviewStep(() => store.target?.branchId ?? '');
const hasWorkflowReview = computed(() => reviewStep.data.value?.run != null);
const rightTab = computed<'workflow' | 'ask'>({
  get: () => {
    if (!hasWorkflowReview.value) return 'ask';
    if (store.rightTab) return store.rightTab;
    return reviewStep.data.value?.run?.state === 'review' ? 'workflow' : 'ask';
  },
  set: (v) => {
    store.rightTab = v;
  },
});
const RIGHT_TABS = computed(() => [
  {
    value: 'workflow',
    label: t('space.ade.reviewStep.tab.workflow.label'),
    testid: 'ade-review-tab-workflow',
  },
  { value: 'ask', label: t('space.ade.reviewStep.tab.ask.label'), testid: 'ade-review-tab-ask' },
]);

// Pane bounds come from the row, not fixed caps: the right pane takes priority, the left fits
// around it, and the diffs keep CENTRE_MIN while the window is wide enough. The clamp never writes
// back, so a stored width survives a window that is briefly too narrow.
const LEFT_MIN = 180;
const RIGHT_MIN = 280;
const CENTRE_MIN = 320;
// Row layout in px: `gap-0.5` between children, `w-0.5` per resize handle. The row's content width
// excludes its own padding.
const GAP = 2;
const HANDLE = 2;
const row = useTemplateRef<HTMLElement>('row');
const { width: rowWidth } = useElementSize(row);
const chrome = computed(() => (store.agentPanelVisible ? 2 : 1) * (HANDLE + 2 * GAP));
// Before the first layout the row has no width: only the minimums apply.
const measured = computed(() => rowWidth.value > 0);

const rightMax = computed(() =>
  measured.value
    ? Math.max(RIGHT_MIN, rowWidth.value - LEFT_MIN - CENTRE_MIN - chrome.value)
    : undefined,
);
const storedRight = usePersistedState('kira.ade.review.rightWidth', 380);
const right = useLiveSize(
  () => Math.min(rightMax.value ?? Infinity, Math.max(RIGHT_MIN, storedRight.value)),
  (w) => {
    storedRight.value = w;
  },
);

const leftMax = computed(() =>
  measured.value
    ? Math.max(
        LEFT_MIN,
        rowWidth.value -
          CENTRE_MIN -
          chrome.value -
          (store.agentPanelVisible ? right.size.value : 0),
      )
    : undefined,
);
const storedLeft = usePersistedState('kira.ade.review.leftWidth', 260);
const left = useLiveSize(
  () => Math.min(leftMax.value ?? Infinity, Math.max(LEFT_MIN, storedLeft.value)),
  (w) => {
    storedLeft.value = w;
  },
);
</script>

<template>
  <div v-if="target" class="flex h-full flex-col" data-testid="ade-review-window">
    <AdeReviewHeader :target="target" />
    <div ref="row" class="flex min-h-0 flex-1 gap-0.5 bg-chrome px-1.5 pb-0.5">
      <section
        class="flex min-h-0 flex-none flex-col overflow-hidden rounded-kira bg-bg"
        :style="{ width: `${left.size.value}px` }"
        :aria-label="t('space.ade.review.window.files.aria')"
        data-testid="ade-review-left"
      >
        <div class="min-h-0 flex-1">
          <AdeReviewFiles ref="files" :target="target" />
        </div>
        <div class="flex max-h-[40%] shrink-0 flex-col overflow-auto empty:hidden">
          <AdeReviewRemoved :target="target" />
        </div>
      </section>
      <ResizeHandle
        :label="t('space.ade.panel.resize.label')"
        data-testid="ade-panel-resize-handle"
        :min="LEFT_MIN"
        :max="leftMax"
        v-bind="left.bind"
      />
      <section
        class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-kira bg-bg"
        :aria-label="t('space.ade.review.window.diffs.aria')"
        data-testid="ade-review-centre"
      >
        <div
          class="h-tabbar min-h-0 shrink-0 overflow-hidden bg-chrome"
          data-testid="ade-review-tabs"
        >
          <TabStrip />
        </div>
        <div class="min-h-0 flex-1">
          <MainView>
            <template #empty>
              <p class="m-0 p-4 text-muted-foreground" data-testid="ade-review-empty">{{
                t('space.ade.review.window.emptyHint.emptyHint')
              }}</p>
            </template>
          </MainView>
        </div>
      </section>
      <ResizeHandle
        v-if="store.agentPanelVisible"
        :label="t('space.ade.panel.resize.label')"
        data-testid="ade-panel-resize-handle"
        direction="reverse"
        :min="RIGHT_MIN"
        :max="rightMax"
        v-bind="right.bind"
      />
      <section
        v-if="store.agentPanelVisible"
        class="flex min-h-0 flex-none flex-col overflow-hidden rounded-kira bg-bg"
        :style="{ width: `${right.size.value}px` }"
        :aria-label="t('space.ade.review.agent.label')"
        data-testid="ade-review-right"
      >
        <SecondaryTabs
          v-if="hasWorkflowReview"
          v-model="rightTab"
          :items="RIGHT_TABS"
          class="shrink-0"
          data-testid="ade-review-right-tabs"
        />
        <div class="min-h-0 flex-1">
          <AdeWorkflowReviewPanel
            v-if="rightTab === 'workflow'"
            :target="target"
            @open-file="(path, origin) => files?.selectFile(path, origin)"
          />
          <AdeReviewAgentPanel v-else :target="target" :repo-label="repoLabel" />
        </div>
      </section>
    </div>
  </div>
</template>
