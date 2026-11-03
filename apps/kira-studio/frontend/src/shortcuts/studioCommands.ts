import { t } from '@theme/i18n';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { runCommand } from '@workbench/shortcuts/commands';
import type { PaletteCommand } from '@workbench/shortcuts/palette';
import { openApiRequestTab, openGrpcRequestTab } from '../api/tabs';
import { useKubeTabsStore } from '../kubernetes/tabs';
import { useAppUpdateStore } from '../state/appUpdate';
import { useConnectionDialogStore } from '../state/connectionDialogs';
import { useLayoutStore } from '../state/layout';
import { useModeStore } from '../state/mode';
import { useSettingsStore } from '../state/settings';
import { useTabsStore } from '../state/tabs';

// The shell tab store holds no tab in Kubernetes mode; the module keeps its own.
const kubernetesActive = (): boolean => useModeStore().active === 'kubernetes';

/** Tab stepping and closing for the palette and the Window menu; Kubernetes mode has its own store. */
export function nextTab(): void {
  if (kubernetesActive()) useKubeTabsStore().activateNext();
  else useTabsStore().activateNextTab();
}

export function prevTab(): void {
  if (kubernetesActive()) useKubeTabsStore().activatePrev();
  else useTabsStore().activatePrevTab();
}

export function closeTab(): void {
  if (kubernetesActive()) {
    useKubeTabsStore().closeActive();
    return;
  }
  const active = useModeStore().activeTab;
  if (active) fireAndForget(useTabsStore().closeTab(active.id), 'closeTab');
}

/** Studio's palette commands; call once from App setup (the stores resolve there). The getter re-reads labels when the locale changes. */
export function studioPaletteCommands(): () => PaletteCommand[] {
  const layoutStore = useLayoutStore();
  const settingsStore = useSettingsStore();
  const appUpdateStore = useAppUpdateStore();

  // Deliberately small and 1:1 with already-reachable actions — every global shortcut this
  // phase adds (the palette's own toggle excluded), plus the handful of other one-click actions
  // worth a name in the palette. No fuzzy scoring, no "go to anything" navigation.
  return () => [
    {
      id: 'new-connection',
      label: t('studio.project.connectionDialog.newConnection.title'),
      run: () => useConnectionDialogStore().openNew(),
    },
    // D13: a one-click action worth a name, the same bar this list's own comment states — no menu
    // or accelerator change (ApiStart.vue/CollectionsPanel.vue's own buttons are the other two).
    {
      id: 'api.newRequest',
      label: t('studio.api.common.newRequest.label'),
      run: () => void openApiRequestTab(),
    },
    // P13 D21: the same one-click bar, filling the gap P12's own F24 finding named — the action
    // already exists in three other places (ApiStart.vue, api/menus.ts's row and background menus).
    {
      id: 'api.newGrpcRequest',
      label: t('shared.grpc.newRequest.title'),
      run: () => void openGrpcRequestTab(),
    },
    // The discoverability answer for Save, at the same bar this list's own comment states.
    // No ⌘S and no accelerator — shared/domain/shortcuts.ts is a closed map and a menu accelerator
    // needs the seven-file path deferred as one deliberate pass. View-scoped like
    // view.run below, so it is a no-op when no request tab is mounted.
    { id: 'api.save', label: t('studio.api.saveRequest.title'), run: () => runCommand('api.save') },
    // P7 D10: view-scoped exactly like api.save above — a no-op with no request tab mounted.
    {
      id: 'api.copyAsCurl',
      label: t('studio.api.copyAsCurl.title'),
      run: () => runCommand('api.copyAsCurl'),
    },
    // P9 D8: same view-scoped shape — a no-op with no request tab mounted or a formdata/file body.
    {
      id: 'api.editRaw',
      label: t('studio.api.editRaw.title'),
      run: () => runCommand('api.editRaw'),
    },
    // Registered by CollectionsPanel.vue, which is mounted for the whole of Api mode — an import
    // is not tab-scoped the way Save is.
    {
      id: 'api.import',
      label: t('studio.api.common.importCollection.label'),
      run: () => runCommand('api.import'),
    },
    // P5 D3/D11: both registered by CollectionsPanel.vue, mounted for the whole of Api mode — no
    // tab to be scoped to.
    {
      id: 'api.variables',
      label: t('common.module.variables.label'),
      run: () => runCommand('api.variables'),
    },
    {
      id: 'api.environments',
      label: t('studio.api.common.environments.label'),
      run: () => runCommand('api.environments'),
    },
    // P6 D11: the reference dialog's palette entry, same "registered by CollectionsPanel.vue,
    // mounted for the whole of Api mode" shape as the two above.
    {
      id: 'api.dynamicValues',
      label: t('studio.api.dynamicValues.title'),
      run: () => runCommand('api.dynamicValues'),
    },
    // P7 D12: same "registered by CollectionsPanel.vue, mounted for the whole of Api mode" shape.
    {
      id: 'api.importCurl',
      label: t('studio.api.common.importCurl.label'),
      run: () => runCommand('api.importCurl'),
    },
    {
      id: 'check-for-updates',
      label: t('common.action.checkForUpdates.label'),
      run: () => void appUpdateStore.checkForUpdates(),
    },
    {
      id: 'open-settings',
      label: t('common.action.openSettings.label'),
      run: () => (settingsStore.settingsOpen = true),
    },
    {
      id: 'toggle-project-panel',
      label: t('studio.shortcuts.palette.toggleProjectPanel.label'),
      run: () => layoutStore.toggleProjectPanel(),
    },
    {
      id: 'toggle-operations-panel',
      label: t('studio.shortcuts.palette.toggleOperationsPanel.label'),
      run: () => layoutStore.toggleOperationsPanel(),
    },
    { id: 'view.find', label: t('common.action.find.label'), run: () => runCommand('view.find') },
    {
      id: 'view.refresh',
      label: t('common.action.refresh.label'),
      run: () => runCommand('view.refresh'),
    },
    {
      id: 'view.run',
      label: t('studio.shortcuts.palette.runStatement.label'),
      run: () => runCommand('view.run'),
    },
    {
      id: 'view.run-all',
      label: t('common.action.runAll.label'),
      run: () => runCommand('view.run-all'),
    },
    {
      id: 'view.format',
      label: t('studio.shortcuts.palette.formatQuery.label'),
      run: () => runCommand('view.format'),
    },
    // P18 (v1.1) C12/D12's own comment: chord-less by design — Format took the seven-file
    // accelerator path because that command has a chord users already have in their fingers;
    // Explain has no such convention, so the palette entry alone is where it is discoverable.
    {
      id: 'view.explain',
      label: t('studio.shortcuts.palette.explainQuery.label'),
      run: () => runCommand('view.explain'),
    },
    // P15 D11: DataView.vue registers this only while a data tab is active and its connection
    // allows it — a no-op elsewhere, same as every other view-scoped command above.
    {
      id: 'data.generate',
      label: t('studio.shortcuts.palette.generateData.label'),
      run: () => runCommand('data.generate'),
    },
    { id: 'tab-next', label: t('studio.shortcuts.palette.nextTab.label'), run: nextTab },
    { id: 'tab-prev', label: t('studio.shortcuts.palette.previousTab.label'), run: prevTab },
    { id: 'tab-close', label: t('studio.shortcuts.palette.closeTab.label'), run: closeTab },
  ];
}
