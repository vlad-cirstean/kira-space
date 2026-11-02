import type { AutomationsModuleContext } from '@workbench/automations/module';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { control } from '../bridge/control';
import { useCustomScriptsStore } from '../state/customScripts';
import { useLayoutStore } from '../state/layout';
import { useModeStore } from '../state/mode';
import { useSettingsStore } from '../state/settings';
import { useTabsStore } from '../state/tabs';
import { useTerminalsStore } from '../state/terminals';
import { openTerminalTab } from '../state/terminalTabs';

// P128 §2.4: this app's own Automations-module context, provided once in App.vue (alongside
// workbenchHostKey) for the shared AutomationsPanel.vue/AutomationsStart.vue/AutomationsNewTab.vue/
// TerminalTabView.vue to inject. `scripts` wires the shared script store's own CRUD.
export function createAutomationsModule(): AutomationsModuleContext {
  const customScriptsStore = useCustomScriptsStore();
  const settingsStore = useSettingsStore();
  const terminalsStore = useTerminalsStore();
  const modeStore = useModeStore();
  const tabsStore = useTabsStore();
  const layoutStore = useLayoutStore();

  return {
    defaultCwd: () => terminalsStore.terminalDefaults.cwd,
    openTerminalTab: (opts) => {
      openTerminalTab({
        workspaceId: 'automations',
        cwd: opts.cwd,
        launch: opts.launch,
        replace: opts.replace,
      });
    },
    openRunTab: (state, replace) => {
      tabsStore.openTab('script-run', null, state.runId, () => state, {
        reuse: true,
        workspaceId: 'automations',
        replace,
      });
    },
    tabs: {
      list: () => tabsStore.tabsForWorkspace('automations'),
      activate: (id) => tabsStore.activateTab(id),
    },
    host: {
      rendererDeps: {
        appearance: () => settingsStore.appearance,
        onTerminalOutput: terminalsStore.onTerminalOutput,
        writeTerminal: terminalsStore.writeTerminal,
        openLink: (url) => fireAndForget(control.openExternal(url), 'terminalLink'),
        linkActivation: 'modifier',
      },
      terminalSession: terminalsStore.terminalSession,
      openTerminalSession: terminalsStore.openTerminalSession,
      resizeTerminal: terminalsStore.resizeTerminal,
    },
    runs: {
      list: (limit, taskId) => control.scriptRunsList(limit, taskId),
      stop: (id) => control.scriptRunsStop(id),
      resolveDir: (scriptId) => control.scriptRunsResolveDir(scriptId),
      onChanged: (cb) => control.onScriptRunsChanged(cb),
      preview: (args) => control.scriptRunsPreview(args),
      start: (args, hash) => control.scriptRunsStart(args, hash),
      resume: (id) => control.scriptRunsResume(id),
      resumeCommand: (runId) => control.scriptRunsResumeCommand(runId),
      restoreState: () => control.scriptRunsRestoreState(),
      restore: (items) => control.scriptRunsRestore(items),
      restoreDismiss: () => control.scriptRunsRestoreDismiss(),
      readLog: (id, afterSeq) => control.scriptRunsReadLog(id, afterSeq),
      onLog: (cb) => control.onScriptRunLog(cb),
      nextFires: (cron, timezone, count) => control.scriptRunsNextFires(cron, timezone, count),
      schedulePreview: (scriptId, secrets) => control.scriptRunsSchedulePreview(scriptId, secrets),
      runScheduleNow: (scriptId, hash, secrets) =>
        control.scriptRunsRunScheduleNow(scriptId, hash, secrets),
      confirmAccept: (runId, hash, secrets) =>
        control.scriptRunsConfirmAccept(runId, hash, secrets),
      confirmDecline: (runId) => control.scriptRunsConfirmDecline(runId),
      mcpServers: () => control.scriptRunsMcpServers(),
    },
    ade: false,
    showAutomations: () => modeStore.setMode('automations'),
    showOperations: () => layoutStore.showOperationsPanel(),
    chooseFolder: async (title) => {
      const chosen = await control.filesChooseFolder(title);
      return chosen.canceled ? null : chosen.path;
    },
    scripts: {
      records: () => customScriptsStore.records,
      create: (fields, secretValues) =>
        customScriptsStore.createCustomScript(fields, secretValues ?? null),
      update: (id, fields, secretValues) =>
        customScriptsStore.updateCustomScript(id, fields, secretValues ?? null),
      secretStorage: () => customScriptsStore.secretStorage,
      remove: (id) => customScriptsStore.removeCustomScript(id),
      collections: () => customScriptsStore.collections,
      createCollection: (name) => customScriptsStore.createCollection(name),
      renameCollection: (id, name) => customScriptsStore.renameCollection(id, name),
      removeCollection: (id) => customScriptsStore.removeCollection(id),
      move: (id, collectionId, beforeId) =>
        customScriptsStore.moveScript(id, collectionId, beforeId),
      reorder: (collectionId, ids) => customScriptsStore.reorderScripts(collectionId, ids),
      reorderCollections: (ids) => customScriptsStore.reorderCollections(ids),
    },
  };
}
