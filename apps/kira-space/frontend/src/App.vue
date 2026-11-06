<script setup lang="ts">
import type { AppMode } from '@shared/domain/mode';
// P104 §6.1: every converted call site reaches timing (delayDuration/skipDelayDuration) through
// this one provider.
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
import { memoryModuleKey } from '@workbench/memory/module';
import PromptHost from '@workbench/prompts/PromptHost.vue';
import { promptsControlKey } from '@workbench/prompts/promptsQueries';
import QuitPrompt from '@workbench/prompts/QuitPrompt.vue';
import UpdatePrompt from '@workbench/prompts/UpdatePrompt.vue';
import CommandPalette from '@workbench/shortcuts/CommandPalette.vue';
import { runCommand } from '@workbench/shortcuts/commands';
import { providePaletteCommands, usePaletteStore } from '@workbench/shortcuts/palette';
import { appUpdateStoreKey } from '@workbench/state/createAppUpdateStore';
import { computed, getCurrentInstance, markRaw, onMounted, onUnmounted, provide } from 'vue';
import { spaceApprovalKinds } from './ade/v2/request/kinds';
import AdeReviewWindow from './ade/v2/review/AdeReviewWindow.vue';
import AdeHandoverPrompt from './ade/v2/stage/AdeHandoverPrompt.vue';
import { useAdeAgentsUiStore } from './ade/v2/state/adeAgentsUi';
import { useAdeReviewWindowStore } from './ade/v2/state/adeReviewWindow';
import { useAdeWorkflowsUiStore } from './ade/v2/state/adeWorkflowsUi';
import { control } from './bridge/control';
import CaptureWindow from './capture/CaptureWindow.vue';
import { useCaptureWindowStore } from './capture/state';
import HostViewWindow from './hostview/HostViewWindow.vue';
import { useHostViewWindowStore } from './hostview/state';
import OpenedFilesWindow from './openedfiles/OpenedFilesWindow.vue';
import { appPaletteCommands, closeTab, nextTab, prevTab } from './shortcuts/appCommands';
import { useAppUpdateStore } from './state/appUpdate';
import { useKeepAwakeStore } from './state/keepAwake';
import { useLayoutStore } from './state/layout';
import { useModeStore } from './state/mode';
import { useOpenedFilesWindowStore } from './state/openedFilesWindow';
import { useSettingsStore } from './state/settings';
import ApiDialogs from './studio/api/ApiDialogs.vue';
import { useCollectionsStore } from './studio/api/state/collections';
import { openApiRequestTab } from './studio/api/tabs';
import { studioApprovalKinds } from './studio/approvals/kinds';
import { control as studioControl } from './studio/bridge/control';
import ConnectionFileDialog from './studio/project/ConnectionFileDialog.vue';
import DataGripImportDialog from './studio/project/DataGripImportDialog.vue';
import MaskingReviewPrompt from './studio/project/MaskingReviewPrompt.vue';
import NewConnectionDialog from './studio/project/NewConnectionDialog.vue';
import ProofReadyPrompt from './studio/proof/ProofReadyPrompt.vue';
import ProofWindow from './studio/proof/ProofWindow.vue';
import { useProofWindowStore } from './studio/proof/state';
import { useConnectionDialogStore } from './studio/state/connectionDialogs';
import { useDatagripImportStore } from './studio/state/datagripImport';
import { useFakeDataStore } from './studio/state/fakeData';
import { useObjectStoreStore } from './studio/state/objectStore';
import GenerateDataDialog from './studio/workbench/GenerateDataDialog.vue';
import UploadObjectDialog from './studio/workbench/UploadObjectDialog.vue';
import { createAutomationsModule } from './workbench/automationsModule';
import GitCredentialDialog from './workbench/GitCredentialDialog.vue';
import { createWorkbenchHost } from './workbench/host';
import MobilePairingDialog from './workbench/MobilePairingDialog.vue';
import { createMemoryModule } from './workbench/memoryModule';
import { MODE_ORDER, MODES, STUDIO_MODES } from './workbench/modes';
import SettingsDialog from './workbench/SettingsDialog.vue';
import WorkbenchShell from './workbench/WorkbenchShell.vue';

// P103 Part 2 (§5.4): provided once, here, for MainView/TabStrip/WorkbenchShell (via their own
// per-app workbench/*.vue wrappers) to inject through packages/workbench/src/host.ts.
provide(workbenchHostKey, createWorkbenchHost());
// P128 §2.4/§2.7: the automations module's own context, for AutomationsPanel.vue/AutomationsStart.vue/
// AutomationsNewTab.vue/TerminalTabView.vue (all shared with Kira Studio) to inject.
provide(automationsModuleKey, createAutomationsModule());
provide(memoryModuleKey, createMemoryModule());
provide(promptsControlKey, control);
provide(approvalsControlKey, control);
provideApprovalKinds({ ...spaceApprovalKinds(), ...studioApprovalKinds() });
provide(llmPromptsControlKey, control);
// App level, not component level: Pinia stores and composables outside the tree inject it too.
getCurrentInstance()?.appContext.app.provide(agentControlKey, control);

// Always-mounted root dialogs (ConfirmDialog, GitCredentialDialog: a credential prompt must be able
// to appear with nothing else open) and the Go menu's channel subscriptions.
const layoutStore = useLayoutStore();
const settingsStore = useSettingsStore();
useAgentMessageDefaults(() => settingsStore.agent.provider, control);
const modeStore = useModeStore();
const keepAwakeStore = useKeepAwakeStore();
const paletteStore = usePaletteStore();
providePaletteCommands(appPaletteCommands());
const appUpdateStore = useAppUpdateStore();
provide(appUpdateStoreKey, appUpdateStore);
// P246: one dialog per routed popup kind; PromptHost shows the one the router targeted here.
const promptKinds = {
  schedule: markRaw(ScheduleConfirmPrompt),
  'git-credential': markRaw(GitCredentialDialog),
  'mobile-pairing': markRaw(MobilePairingDialog),
  update: markRaw(UpdatePrompt),
  quit: markRaw(QuitPrompt),
  handover: markRaw(AdeHandoverPrompt),
  proof: markRaw(ProofReadyPrompt),
  masking: markRaw(MaskingReviewPrompt),
};
const reviewWindow = useAdeReviewWindowStore();
const hostView = useHostViewWindowStore();
const capture = useCaptureWindowStore();
const openedFilesWindow = useOpenedFilesWindowStore();
const proofWindow = useProofWindowStore();
const connectionDialogStore = useConnectionDialogStore();
const datagripImportStore = useDatagripImportStore();
const fakeDataStore = useFakeDataStore();
const objectStoreStore = useObjectStoreStore();
const collectionsStore = useCollectionsStore();

// Review, host-view, capture, opened-files and proof windows own no project or operations panel.
const mainWindow = (): boolean =>
  !reviewWindow.target &&
  !hostView.target &&
  !capture.item &&
  !openedFilesWindow.active &&
  !proofWindow.active;
const panelLabel = computed(() =>
  STUDIO_MODES.has(modeStore.active)
    ? t('common.module.connections.label')
    : t('common.module.repositories.label'),
);

let unsubscribe: Array<() => void> = [];

onMounted(() => {
  unsubscribe = [
    control.onCheckForUpdates(() => void appUpdateStore.checkForUpdates()),
    control.onOpenSettings(() => {
      settingsStore.settingsOpen = true;
    }),
    // A review window has no project panel, and its secondary panel is the agent panel; the layout
    // is shared across windows, so toggling it from here would move the main window's panels.
    control.onToggleProjectPanel(() => {
      if (mainWindow()) layoutStore.toggleProjectPanel();
    }),
    control.onToggleOperationsPanel(() => {
      if (reviewWindow.target) reviewWindow.toggleAgentPanel();
      else if (mainWindow()) layoutStore.toggleOperationsPanel();
    }),
    control.onTabNext(nextTab),
    control.onTabPrev(prevTab),
    control.onTabClose(closeTab),
    control.onCommandPalette(paletteStore.togglePalette),
    control.onViewFind(() => runCommand('view.find')),
    studioControl.onNewConnection(() => connectionDialogStore.openNew()),
    studioControl.onNewRequest(() => {
      modeStore.setMode('api');
      openApiRequestTab();
    }),
    studioControl.onImportPostman(() => {
      modeStore.setMode('api');
      void collectionsStore.importPostman();
    }),
    studioControl.onImportDataGrip(() => {
      modeStore.setMode('studio');
      void datagripImportStore.pickAndScanDataGripProject();
    }),
    studioControl.onViewRefresh(() => runCommand('view.refresh')),
    studioControl.onViewRun(() => runCommand('view.run')),
    studioControl.onViewRunAll(() => runCommand('view.run-all')),
    studioControl.onViewFormat(() => runCommand('view.format')),
  ];
});

onUnmounted(() => {
  for (const off of unsubscribe) off();
});

// Leaving Agents unmounts the workflow and agent config editors: confirm before dropping unsaved edits.
async function beforeModeChange(mode: AppMode): Promise<boolean> {
  if (modeStore.active === 'ade' && mode !== 'ade') {
    return (await useAdeWorkflowsUiStore().leave()) && (await useAdeAgentsUiStore().leave());
  }
  return true;
}

// The title bar has no toast channel: a rejection is logged, not surfaced.
function onNewWindow(): void {
  fireAndForget(control.windowsOpenNew(), 'window');
}

// The button shows only the manual source.
function onToggleKeepAwake(): void {
  fireAndForget(keepAwakeStore.setKeepAwakeManual(!keepAwakeStore.status.manual), 'keepAwake');
}
</script>

<template>
  <!-- P104 §6.1: disable-hoverable-content matches the app's pointer-events: none tooltip. -->
  <TooltipProvider>
    <CaptureWindow v-if="capture.item" />
    <HostViewWindow v-else-if="hostView.target" />
    <AdeReviewWindow v-else-if="reviewWindow.target" />
    <OpenedFilesWindow v-else-if="openedFilesWindow.active" />
    <ProofWindow v-else-if="proofWindow.active" />
    <div v-else class="h-full flex flex-col">
      <WorkbenchTitleBar
        :order="MODE_ORDER"
        :modes="MODES"
        :active="modeStore.active"
        :panel-label="panelLabel"
        :project-visible="layoutStore.panel.project.visible"
        :operations-visible="layoutStore.panel.operations.visible"
        :keep-awake="keepAwakeStore.status"
        :before-mode-change="beforeModeChange"
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
      :placeholder="t('space.shortcuts.palette.placeholder')"
      :empty-label="t('space.shortcuts.palette.empty')"
    />
  </TooltipProvider>
</template>
