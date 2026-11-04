<script setup lang="ts">
// P264 §2.8: the project panel and the Operations dock resize through ResizeHandle (VueUse
// `useDraggable`), plain flex layout. The dock stays a full-width flex sibling below the row, never
// nested in a split group (P132 Part 1: nested reka groups hung the render process, and a px panel
// re-ran reka's layout on every ResizeObserver tick beside a virtualized SlickGrid). The handles
// emit the size on release; a drag previews through local state, so the parent store sees one write.
import { ResizeHandle } from '@theme/components/ui/resizable';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';
import MainView from './MainView.vue';
import TabStrip from './TabStrip.vue';

interface Props {
  projectVisible: boolean;
  projectWidth: number;
  opsVisible: boolean;
  opsHeight: number;
  /** P129 Part 3 §0.13: hides the tab-strip row for a `layout: 'full'` module (Kira Space's own
   *  `ade`, which renders its own view in `#main` instead of tab-scoped `MainView`). Default
   *  `true`: Kira Studio passes nothing, so its own geometry is unchanged. */
  tabStripVisible?: boolean;
  /** Default `true`. `false` lets `#main` lay out its own framed panes straight on the chrome (a
   *  full-layout module), so no pane sits inside a second frame. */
  mainFramed?: boolean;
}
const props = withDefaults(defineProps<Props>(), { tabStripVisible: true, mainFramed: true });
const emit = defineEmits<{
  resizeProject: [size: number];
  resizeOps: [size: number];
}>();

const PROJECT_MIN_PX = 180;
const PROJECT_MAX_PX = 480;
const OPS_MIN_PX = 100;
const OPS_MAX_PX = 500;

const clamp = (value: number, min: number, max: number): number =>
  Math.min(max, Math.max(min, value));

const liveProject = ref<number | null>(null);
const liveOps = ref<number | null>(null);
const projectSize = computed(() =>
  clamp(liveProject.value ?? props.projectWidth, PROJECT_MIN_PX, PROJECT_MAX_PX),
);
const dockHeight = computed(() => clamp(liveOps.value ?? props.opsHeight, OPS_MIN_PX, OPS_MAX_PX));

function commitProject(size: number): void {
  liveProject.value = null;
  emit('resizeProject', size);
}
function commitOps(size: number): void {
  liveOps.value = null;
  emit('resizeOps', size);
}
</script>

<template>
  <!-- flex-1 (was dropped converting the old grid's `flex: 1` to Tailwind classes, collapsing the
       whole shell to content height) and gap-0.5 (the old grid's row-gap, 2px, between rows) both
       reproduce byte-identical geometry to the previous CSS grid. The dock's own 6px gaps above and
       below (main-to-dock, dock-to-status) come from this same gap-0.5 plus the handle's own h-0.5
       and the status bar's own mt-1 below — no separate spacing rule for the dock. -->
  <div class="workbench-shell flex flex-1 flex-col min-h-0 gap-0.5 px-1.5 pb-0.5 bg-chrome">
    <div class="flex flex-1 min-h-0 gap-0.5">
      <div
        v-if="projectVisible"
        class="overflow-hidden min-w-0 min-h-0 rounded-kira bg-bg"
        data-testid="project-panel"
        :style="{ flex: `0 0 ${projectSize}px` }"
      >
        <slot name="panel" />
      </div>
      <ResizeHandle
        v-if="projectVisible"
        :label="t('workbench.shell.resizeProject.label')"
        :size="projectSize"
        :min="PROJECT_MIN_PX"
        :max="PROJECT_MAX_PX"
        @live="liveProject = $event"
        @commit="commitProject"
        @cancel="liveProject = null"
      />

      <div class="min-w-0 flex-1" data-testid="main-panel">
        <div
          class="h-full flex flex-col min-w-0 min-h-0 overflow-hidden"
          :class="{ 'rounded-kira bg-bg': mainFramed }"
        >
          <!-- Taller than a tab (--kira-h-md, 26px) by design (h-tabbar) — the extra height is
               the tab's own breathing room within the chrome row, not a margin tacked
               on after it. -->
          <div
            v-if="tabStripVisible"
            class="h-tabbar min-h-0 overflow-hidden shrink-0 bg-chrome"
            data-testid="tab-strip"
          >
            <slot name="tab-strip"><TabStrip /></slot>
          </div>
          <div class="flex-1 min-h-0" data-testid="main-view">
            <slot name="main"><MainView /></slot>
          </div>
        </div>
      </div>
    </div>

    <template v-if="opsVisible">
      <ResizeHandle
        :label="t('workbench.shell.resizeOps.label')"
        axis="y"
        direction="reverse"
        :size="dockHeight"
        :min="OPS_MIN_PX"
        :max="OPS_MAX_PX"
        @live="liveOps = $event"
        @commit="commitOps"
        @cancel="liveOps = null"
      />
      <div
        class="shrink-0 overflow-hidden min-w-0 min-h-0 rounded-kira bg-bg"
        data-testid="operations-panel"
        :style="{ height: `${dockHeight}px` }"
      >
        <slot name="dock" />
      </div>
    </template>

    <!-- The old grid template stayed four rows (main/splitops/ops/status) whether or not ops was
         open — even a collapsed splitops/ops row consumed its own row-gap. mt-1 (4px, two more
         2px row-gaps) reproduces that reserved space exactly; gap-0.5 above already accounts for
         one. Both apps mount a dock, so it is unconditional. -->
    <div class="shrink-0 h-statusbar mt-1" data-testid="status-bar">
      <slot name="status" />
    </div>
  </div>
</template>
