<script setup lang="ts">
import RunScriptDialogHost from '@workbench/automations/run/RunScriptDialogHost.vue';
import ScheduleConfirmHost from '@workbench/automations/schedule/ScheduleConfirmHost.vue';
import MainView from '@workbench/components/MainView.vue';
import TabStrip from '@workbench/components/TabStrip.vue';
import WorkbenchShellBase from '@workbench/components/WorkbenchShell.vue';
import { computed } from 'vue';
import { useLayoutStore } from '../state/layout';
import { useModeStore } from '../state/mode';
import { MODES } from './modes';
import OperationsPanel from './OperationsPanel.vue';
import StatusBar from './StatusBar.vue';

// Kira Studio's own WorkbenchShell.vue, trimmed. The Operations
// dock is mounted here too, gated only by `panel.operations.visible` (also in full layout, where
// `ade` writes ops). `GitPanel`/`GitStart`, once this whole app's own
// left panel, are now one entry (`git`) in this app's own module registry (workbench/modes.ts),
// alongside `terminal` and `ade` — the per-module "+" branch this file used to hold moved into the
// module itself (repo/GitNewTab.vue), the same shape Kira Studio's own automations module took.
// This file keeps only this app's own per-app content: the mode-scoped left-panel
// lookup.
const layoutStore = useLayoutStore();
const modeStore = useModeStore();
const sizes = computed(() => layoutStore.sizesFor(modeStore.active));

// P129 Part 3 §0.12/§0.13/§2.1: a `layout: 'full'` module (`ade`) has no panel/start/newTab at all
// — `def`'s own `layout` discriminant is what every computed below narrows on, so a `FullModeDef`
// never has its (non-existent) `.panel`/`.start`/`.newTab` accessed.
const def = computed(() => MODES[modeStore.active]);
const isFull = computed(() => def.value.layout === 'full');
const hasTabStrip = computed(() => def.value.layout !== 'full' && def.value.tabStrip !== false);
// P1 D6/C6 (Kira Studio's own pattern): the left panel mounts whichever module is active's own
// self-contained panel component.
const activeModePanel = computed(() => (def.value.layout === 'full' ? undefined : def.value.panel));
// MainView.vue's own fallback when the active module has no active tab.
const modeStart = computed(() => (def.value.layout === 'full' ? undefined : def.value.start));
// The tab strip's own "+", when the active module has one.
const modeNewTab = computed(() => (def.value.layout === 'full' ? undefined : def.value.newTab));
// Rendered in `#main` instead of the tab-scoped MainView, in place of it, for a full-layout module.
const fullView = computed(() => (def.value.layout === 'full' ? def.value.view : undefined));
</script>

<template>
  <WorkbenchShellBase
    :project-visible="!isFull && layoutStore.panel.project.visible"
    :project-width="sizes.project.width"
    :tab-strip-visible="hasTabStrip"
    :main-framed="!isFull"
    :ops-visible="layoutStore.panel.operations.visible"
    :ops-height="sizes.operations.height"
    @resize-project="(width: number) => layoutStore.setProjectWidth(modeStore.active, width)"
    @resize-ops="(height: number) => layoutStore.setOperationsHeight(modeStore.active, height)"
  >
    <template #panel>
      <component :is="activeModePanel" />
    </template>
    <template #tab-strip>
      <TabStrip>
        <template #new-tab>
          <component :is="modeNewTab" v-if="modeNewTab" />
        </template>
      </TabStrip>
    </template>
    <template #main>
      <component :is="fullView" v-if="isFull" />
      <!-- Stays mounted behind a full module so kept graphs survive (MainView `suspended`). -->
      <div v-show="!isFull" class="contents">
        <MainView :suspended="isFull">
          <template #empty>
            <component :is="modeStart" />
          </template>
        </MainView>
      </div>
    </template>
    <template #dock>
      <OperationsPanel />
    </template>
    <template #status>
      <StatusBar />
      <RunScriptDialogHost />
      <ScheduleConfirmHost />
    </template>
  </WorkbenchShellBase>
</template>
