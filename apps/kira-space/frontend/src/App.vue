<script setup lang="ts">
// P104 §6.1: every converted call site reaches timing (delayDuration/skipDelayDuration) through
// this one provider.
import { TooltipProvider } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { agentControlKey, useAgentMessageDefaults } from '@workbench/agent/agentInfo';
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
import { getCurrentInstance, markRaw, onMounted, onUnmounted, provide } from 'vue';
import AdeBranchRequestPrompt from './ade/v2/request/AdeBranchRequestPrompt.vue';
import AdeReviewWindow from './ade/v2/review/AdeReviewWindow.vue';
import AdeHandoverPrompt from './ade/v2/stage/AdeHandoverPrompt.vue';
import { useAdeContextUiStore } from './ade/v2/state/adeContextUi';
import { useAdeReviewWindowStore } from './ade/v2/state/adeReviewWindow';
import { useAdeSettingsUiStore } from './ade/v2/state/adeSettingsUi';
import { useAdeWorkflowsUiStore } from './ade/v2/state/adeWorkflowsUi';
import { control } from './bridge/control';
import CaptureWindow from './capture/CaptureWindow.vue';
import { useCaptureWindowStore } from './capture/state';
import HostViewWindow from './hostview/HostViewWindow.vue';
import { useHostViewWindowStore } from './hostview/state';
import { spacePaletteCommands } from './shortcuts/spaceCommands';
import { useAppUpdateStore } from './state/appUpdate';
import { useKeepAwakeStore } from './state/keepAwake';
import { useLayoutStore } from './state/layout';
import { type SpaceMode, useModeStore } from './state/mode';
import { useSettingsStore } from './state/settings';
import { createAutomationsModule } from './workbench/automationsModule';
import GitCredentialDialog from './workbench/GitCredentialDialog.vue';
import { createWorkbenchHost, useTabsStore } from './workbench/host';
import MobilePairingDialog from './workbench/MobilePairingDialog.vue';
import { createMemoryModule } from './workbench/memoryModule';
import { MODE_ORDER, MODES } from './workbench/modes';
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
provide(llmPromptsControlKey, control);
// App level, not component level: Pinia stores and composables outside the tree inject it too.
getCurrentInstance()?.appContext.app.provide(agentControlKey, control);

// Always-mounted root dialogs (ConfirmDialog, GitCredentialDialog: a credential prompt must be able
// to appear with nothing else open) and the Go menu's channel subscriptions.
const layoutStore = useLayoutStore();
const settingsStore = useSettingsStore();
useAgentMessageDefaults(() => settingsStore.agent.provider, control);
const tabsStore = useTabsStore();
const modeStore = useModeStore();
const keepAwakeStore = useKeepAwakeStore();
const paletteStore = usePaletteStore();
providePaletteCommands(spacePaletteCommands());
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
  'branch-request': markRaw(AdeBranchRequestPrompt),
};
const reviewWindow = useAdeReviewWindowStore();
const hostView = useHostViewWindowStore();
const capture = useCaptureWindowStore();

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
      if (!reviewWindow.target && !hostView.target && !capture.item)
        layoutStore.toggleProjectPanel();
    }),
    control.onToggleOperationsPanel(() => {
      if (reviewWindow.target) reviewWindow.toggleAgentPanel();
      else if (!hostView.target && !capture.item) layoutStore.toggleOperationsPanel();
    }),
    control.onTabNext(tabsStore.activateNextTab),
    control.onTabPrev(tabsStore.activatePrevTab),
    control.onTabClose(tabsStore.closeActiveTab),
    control.onCommandPalette(paletteStore.togglePalette),
    control.onViewFind(() => runCommand('view.find')),
  ];
});

onUnmounted(() => {
  for (const off of unsubscribe) off();
});

// Leaving Agents unmounts the workflow, context and settings editors: confirm before dropping unsaved edits.
async function beforeModeChange(mode: SpaceMode): Promise<boolean> {
  if (modeStore.active === 'ade' && mode !== 'ade') {
    return (
      (await useAdeWorkflowsUiStore().leave()) &&
      (await useAdeContextUiStore().leave()) &&
      (await useAdeSettingsUiStore().leave())
    );
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
    <div v-else class="h-full flex flex-col">
      <WorkbenchTitleBar
        :order="MODE_ORDER"
        :modes="MODES"
        :active="modeStore.active"
        :panel-label="t('common.module.repositories.label')"
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
    <PromptHost :kinds="promptKinds" />
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
