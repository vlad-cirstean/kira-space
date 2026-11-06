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
import OperationsPanel from './panels/OperationsPanel.vue';
import StatusBar from './StatusBar.vue';

// Kira Studio's own WorkbenchShell.vue, now a thin composition over the shared
// grid/splitters/TabStrip/MainView package components — this file keeps exactly the per-app
// content those components' slots need: the mode-scoped left-panel lookup and the Operations dock.
// The Automations module's own "+" menu (`terminalModuleMenuItems`/`onNewTab`) moved into the
// module itself (packages/workbench/src/terminal/AutomationsNewTab.vue) —
// `#new-tab` now renders whichever mode's own `ModeDef.newTab` is set, with no per-module branch
// here at all.
const modeStore = useModeStore();
const layoutStore = useLayoutStore();
const sizes = computed(() => layoutStore.sizesFor(modeStore.active));

// P1 D6/C6: the left panel mounts whichever mode is active's own self-contained panel component.
const activeModePanel = computed(() => MODES[modeStore.active].panel);
// P1 D6/C6: MainView.vue's fallback when the active mode has no active tab.
const modeStart = computed(() => MODES[modeStore.active].start);
// P91 §8/P128 §2.3: the tab strip's own "+", when the active mode has one.
const modeNewTab = computed(() => MODES[modeStore.active].newTab);
</script>

<template>
  <WorkbenchShellBase
    :project-visible="layoutStore.panel.project.visible"
    :project-width="sizes.project.width"
    :tab-strip-visible="MODES[modeStore.active].tabStrip !== false"
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
      <MainView>
        <template #empty>
          <component :is="modeStart" />
        </template>
      </MainView>
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
