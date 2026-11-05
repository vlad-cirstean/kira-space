<script setup lang="ts">
// P104 §6.1: TooltipProvider is the one place call sites reach tooltip timing
// (delayDuration/skipDelayDuration) through.
import { TooltipProvider } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { agentControlKey, useAgentMessageDefaults } from '@workbench/agent/agentInfo';
import ApprovalsSurface from '@workbench/approvals/ApprovalsSurface.vue';
import { approvalsControlKey } from '@workbench/approvals/approvalsQueries';
import { provideApprovalKinds } from '@workbench/approvals/registry';
import { automationsModuleKey } from '@workbench/automations/module';
import ScheduleConfirmPrompt from '@workbench/automations/schedule/ScheduleConfirmPrompt.vue';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import AppLiveRegion from '@workbench/components/AppLiveRegion.vue';
import ConfirmDialog from '@workbench/components/ConfirmDialog.vue';
import ContextMenu from '@workbench/components/ContextMenu.vue';
import UpdateDialog from '@workbench/components/UpdateDialog.vue';
import WorkbenchTitleBar from '@workbench/components/WorkbenchTitleBar.vue';
import { workbenchHostKey } from '@workbench/host';
import { llmPromptsControlKey } from '@workbench/llmPrompts/queries';
import PromptHost from '@workbench/prompts/PromptHost.vue';
import { promptsControlKey } from '@workbench/prompts/promptsQueries';
import QuitPrompt from '@workbench/prompts/QuitPrompt.vue';
import UpdatePrompt from '@workbench/prompts/UpdatePrompt.vue';
import CommandPalette from '@workbench/shortcuts/CommandPalette.vue';
import { runCommand } from '@workbench/shortcuts/commands';
import { providePaletteCommands, usePaletteStore } from '@workbench/shortcuts/palette';
import { appUpdateStoreKey } from '@workbench/state/createAppUpdateStore';
import { getCurrentInstance, markRaw, onMounted, onUnmounted, provide } from 'vue';
import ApiDialogs from './api/ApiDialogs.vue';
import { useCollectionsStore } from './api/state/collections';
import { openApiRequestTab } from './api/tabs';
import { studioApprovalKinds } from './approvals/kinds';
import { control } from './bridge/control';
import ConnectionFileDialog from './project/ConnectionFileDialog.vue';
import DataGripImportDialog from './project/DataGripImportDialog.vue';
import MaskingReviewPrompt from './project/MaskingReviewPrompt.vue';
import NewConnectionDialog from './project/NewConnectionDialog.vue';
import ProofReadyPrompt from './proof/ProofReadyPrompt.vue';
import { closeTab, nextTab, prevTab, studioPaletteCommands } from './shortcuts/studioCommands';
import { useAppUpdateStore } from './state/appUpdate';
import { useConnectionDialogStore } from './state/connectionDialogs';
import { useDatagripImportStore } from './state/datagripImport';
import { useFakeDataStore } from './state/fakeData';
import { useKeepAwakeStore } from './state/keepAwake';
import { useLayoutStore } from './state/layout';
import { useModeStore } from './state/mode';
import { useObjectStoreStore } from './state/objectStore';
import { useSettingsStore } from './state/settings';
import { createAutomationsModule } from './workbench/automationsModule';
import GenerateDataDialog from './workbench/GenerateDataDialog.vue';
import { createWorkbenchHost } from './workbench/host';
import { MODE_ORDER, MODES } from './workbench/modes';
import SettingsDialog from './workbench/SettingsDialog.vue';
import UploadObjectDialog from './workbench/UploadObjectDialog.vue';
import WorkbenchShell from './workbench/WorkbenchShell.vue';

// P103 Part 2 (§5.4): provided once, here, for MainView/TabStrip/WorkbenchShell (via their own
// per-app workbench/*.vue wrappers) to inject through packages/workbench/src/host.ts.
provide(workbenchHostKey, createWorkbenchHost());
// P128 §2.4: the automations module's own context, for AutomationsPanel.vue/AutomationsStart.vue/
// AutomationsNewTab.vue/TerminalTabView.vue (all shared with Kira Space) to inject.
provide(automationsModuleKey, createAutomationsModule());
provide(promptsControlKey, control);
provide(approvalsControlKey, control);
provideApprovalKinds(studioApprovalKinds());
provide(llmPromptsControlKey, control);
// App level, not component level: Pinia stores and composables outside the tree inject it too.
getCurrentInstance()?.appContext.app.provide(agentControlKey, control);

const appUpdateStore = useAppUpdateStore();
provide(appUpdateStoreKey, appUpdateStore);
// P246: one dialog per routed popup kind; PromptHost shows the one the router targeted here.
const promptKinds = {
  schedule: markRaw(ScheduleConfirmPrompt),
  update: markRaw(UpdatePrompt),
  quit: markRaw(QuitPrompt),
  proof: markRaw(ProofReadyPrompt),
  masking: markRaw(MaskingReviewPrompt),
};
const paletteStore = usePaletteStore();
providePaletteCommands(studioPaletteCommands());
const modeStore = useModeStore();
const datagripImportStore = useDatagripImportStore();
const fakeDataStore = useFakeDataStore();
const objectStoreStore = useObjectStoreStore();
const layoutStore = useLayoutStore();
const collectionsStore = useCollectionsStore();
const connectionDialogStore = useConnectionDialogStore();
const settingsStore = useSettingsStore();
useAgentMessageDefaults(() => settingsStore.agent.provider, control);
const keepAwakeStore = useKeepAwakeStore();

let unsubscribe: Array<() => void> = [];

// A rejection (e.g. a `-tags server` build) logs, no UI. The button shows only the manual source.
function onNewWindow(): void {
  fireAndForget(control.windowsOpenNew(), 'newWindow');
}

function onToggleKeepAwake(): void {
  fireAndForget(
    keepAwakeStore.setKeepAwakeManual(!keepAwakeStore.status.manual),
    'toggleKeepAwake',
  );
}

onMounted(() => {
  unsubscribe = [
    control.onCheckForUpdates(() => void appUpdateStore.checkForUpdates()),
    control.onOpenSettings(() => {
      settingsStore.settingsOpen = true;
    }),
    control.onNewConnection(() => connectionDialogStore.openNew()),
    // P28 D18: three menu-bar commands. Subscribed here rather than in the panels that used to
    // own the buttons, so they work with no panel mounted — which is the point of moving them.
    control.onNewRequest(() => {
      modeStore.setMode('api');
      openApiRequestTab();
    }),
    control.onImportPostman(() => {
      modeStore.setMode('api');
      void collectionsStore.importPostman();
    }),
    control.onImportDataGrip(() => {
      modeStore.setMode('studio');
      void datagripImportStore.pickAndScanDataGripProject();
    }),
    control.onToggleProjectPanel(layoutStore.toggleProjectPanel),
    control.onToggleOperationsPanel(layoutStore.toggleOperationsPanel),
    control.onCommandPalette(paletteStore.togglePalette),
    control.onTabNext(nextTab),
    control.onTabPrev(prevTab),
    control.onTabClose(closeTab),
    control.onViewFind(() => runCommand('view.find')),
    control.onViewRefresh(() => runCommand('view.refresh')),
    control.onViewRun(() => runCommand('view.run')),
    control.onViewRunAll(() => runCommand('view.run-all')),
    control.onViewFormat(() => runCommand('view.format')),
  ];
});

onUnmounted(() => {
  for (const off of unsubscribe) off();
});
</script>

<template>
  <!-- P104 §6.1: disable-hoverable-content matches the app's pointer-events: none tooltip. -->
  <TooltipProvider>
    <!-- P1 C8: the frame TitleBar + WorkbenchShell now share — WorkbenchShell.vue's own root swapped
         `height: 100%` for `flex: 1; min-height: 0` to match. -->
    <div class="h-full flex flex-col">
      <WorkbenchTitleBar
        :order="MODE_ORDER"
        :modes="MODES"
        :active="modeStore.active"
        :panel-label="t('common.module.connections.label')"
        :project-visible="layoutStore.panel.project.visible"
        :operations-visible="layoutStore.panel.operations.visible"
        :keep-awake="keepAwakeStore.status"
        @select="modeStore.setMode"
        @toggle-project="layoutStore.toggleProjectPanel"
        @toggle-operations="layoutStore.toggleOperationsPanel"
        @open-settings="settingsStore.settingsOpen = true"
        @toggle-keep-awake="onToggleKeepAwake"
        @new-window="onNewWindow"
      >
        <template #settings>
          <SettingsDialog
            v-if="settingsStore.settingsOpen"
            @close="settingsStore.settingsOpen = false"
          />
        </template>
      </WorkbenchTitleBar>
      <WorkbenchShell />
    </div>
    <NewConnectionDialog v-if="connectionDialogStore.newOpen" />
    <ConnectionFileDialog v-if="connectionDialogStore.file !== null" />
    <DataGripImportDialog v-if="datagripImportStore.open" />
    <ApiDialogs />
    <UploadObjectDialog v-if="objectStoreStore.open" />
    <GenerateDataDialog v-if="fakeDataStore.open" />
    <PromptHost :kinds="promptKinds" />
    <ApprovalsSurface />
    <ConfirmDialog />
    <AppLiveRegion />
    <UpdateDialog v-if="appUpdateStore.dialogOpen" :store="appUpdateStore" />
    <ContextMenu />
    <CommandPalette
      :placeholder="t('studio.shortcuts.palette.placeholder')"
      :empty-label="t('studio.shortcuts.palette.empty')"
    />
  </TooltipProvider>
</template>
