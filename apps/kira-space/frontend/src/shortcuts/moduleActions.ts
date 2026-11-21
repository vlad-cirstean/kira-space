import type { AppMode } from '@shared/domain/mode';
import type { MappedModule, MODULE_SHORTCUTS, ModuleAction } from '@shared/domain/moduleShortcuts';
import { useNewScriptRequestStore } from '@workbench/automations/newScriptRequest';
import { useAdeAddUiStore } from '../ade/v2/state/adeAddUi';
import { useLayoutStore } from '../state/layout';
import { useModeStore } from '../state/mode';
import { openActiveRepoTerminal } from '../state/repoTabs';
import { useCollectionsStore } from '../studio/api/state/collections';
import { useVariablesStore } from '../studio/api/state/variables';
import { openApiRequestTab, openGrpcRequestTab } from '../studio/api/tabs';
import { useConnectionDialogStore } from '../studio/state/connectionDialogs';
import { useDatagripImportStore } from '../studio/state/datagripImport';

function newScript(kind: 'script' | 'smart'): void {
  const layout = useLayoutStore();
  if (!layout.panel.project.visible) layout.toggleProjectPanel();
  useNewScriptRequestStore().request(kind);
}

/** Per module, the visible "+" or import control each chord stands for. A module or action absent
 *  here does nothing. Handlers resolve their stores at call time, never at import. */
const MODULE_ACTIONS: {
  [M in MappedModule]: Record<(typeof MODULE_SHORTCUTS)[M][number], () => void>;
} = {
  studio: {
    'app.new': () => useConnectionDialogStore().openNew(),
    'app.import': () => void useDatagripImportStore().pickAndScanDataGripProject(),
  },
  api: {
    'app.new': () => void openApiRequestTab(),
    'app.newAlternate': () => void openGrpcRequestTab(),
    'app.import': () => void useCollectionsStore().importPostman(),
    'api.environments': () => useVariablesStore().openEnvironments(),
  },
  automations: {
    'app.new': () => newScript('script'),
    'app.newAlternate': () => newScript('smart'),
  },
  ade: {
    'app.new': () => {
      useAdeAddUiStore().addOpen = true;
    },
  },
  git: {
    'app.new': openActiveRepoTerminal,
  },
};

export function runModuleAction(action: ModuleAction): void {
  const handlers: Partial<Record<AppMode, Partial<Record<ModuleAction, () => void>>>> =
    MODULE_ACTIONS;
  handlers[useModeStore().active]?.[action]?.();
}
