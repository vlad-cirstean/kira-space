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
    chooseFolder: async (title) => (await control.filesChooseFolder(title)).path,
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
      mcpServers: () => control.scriptRunsMcpServers(),
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
    },
    ade: false,
    showAutomations: () => modeStore.setMode('automations'),
    showOperations: () => layoutStore.showOperationsPanel(),
    files: {
      open: (file) => control.configFilesOpen({ folder: 'runs', file }),
      reveal: (file) => control.configFilesReveal({ folder: 'runs', file }),
    },
    scripts: {
      records: () => customScriptsStore.records,
      collections: () => customScriptsStore.collections,
      problems: () => customScriptsStore.problems,
      dir: () => customScriptsStore.dir,
      secretStorage: () => customScriptsStore.secretStorage,
      create: (fields) => customScriptsStore.createCustomScript(fields),
      remove: (id) => customScriptsStore.removeCustomScript(id),
      readFile: (file) => customScriptsStore.readFile(file),
      saveFile: (file, text, baseHash) => customScriptsStore.saveFile(file, text, baseHash),
      form: (file, text, edits) => customScriptsStore.form(file, text, edits),
      setSecrets: (id, values) => customScriptsStore.setSecrets(id, values),
      reorder: (collection, ids) => customScriptsStore.reorderScripts(collection, ids),
      reorderCollections: (names) => customScriptsStore.reorderCollections(names),
      move: (id, collection, beforeId) => customScriptsStore.move(id, collection, beforeId),
      renameCollection: (name, newName) => customScriptsStore.renameCollection(name, newName),
      removeCollection: (name) => customScriptsStore.removeCollection(name),
    },
  };
}
