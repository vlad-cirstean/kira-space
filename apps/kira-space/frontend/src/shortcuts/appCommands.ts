import { t } from '@theme/i18n';
import { runCommand } from '@workbench/shortcuts/commands';
import type { PaletteCommand } from '@workbench/shortcuts/palette';
import { useAppUpdateStore } from '../state/appUpdate';
import { useLayoutStore } from '../state/layout';
import { useModeStore } from '../state/mode';
import { useOpenedFilesWindowStore } from '../state/openedFilesWindow';
import { useSettingsStore } from '../state/settings';
import { useTabsStore } from '../state/tabs';
import { useKubeTabsStore } from '../studio/kubernetes/tabs';
import { studioPaletteCommands } from '../studio/shortcuts/studioCommands';

// Kubernetes mode keeps its own tab store; the shell store holds no tab there.
const kubernetesActive = (): boolean => useModeStore().active === 'kubernetes';

/** Tab stepping and closing for the palette and the Window menu. */
export function nextTab(): void {
  if (kubernetesActive()) useKubeTabsStore().activateNext();
  else useTabsStore().activateNextTab();
}

export function prevTab(): void {
  if (kubernetesActive()) useKubeTabsStore().activatePrev();
  else useTabsStore().activatePrevTab();
}

export function closeTab(): void {
  if (kubernetesActive()) useKubeTabsStore().closeActive();
  else useTabsStore().closeActiveTab();
}

/** The app's palette commands; call once from App setup (the stores resolve there). The getter re-reads labels when the locale changes. */
export function appPaletteCommands(): () => PaletteCommand[] {
  const layoutStore = useLayoutStore();
  const settingsStore = useSettingsStore();
  const appUpdateStore = useAppUpdateStore();
  const filesWindow = useOpenedFilesWindowStore();

  // The files window has no settings mount or panels; the toggles would move the shared layout.
  const workbenchOnly = (): PaletteCommand[] =>
    filesWindow.active
      ? []
      : [
          {
            id: 'open-settings',
            label: t('common.action.openSettings.label'),
            run: () => {
              settingsStore.settingsOpen = true;
            },
          },
          {
            id: 'toggle-project-panel',
            label: t('space.shortcuts.palette.toggleProjectPanel.label'),
            run: () => layoutStore.toggleProjectPanel(),
          },
          {
            id: 'toggle-operations-panel',
            label: t('space.shortcuts.palette.toggleOperationsPanel.label'),
            run: () => layoutStore.toggleOperationsPanel(),
          },
        ];

  return () => [
    {
      id: 'check-for-updates',
      label: t('common.action.checkForUpdates.label'),
      run: () => void appUpdateStore.checkForUpdates(),
    },
    ...workbenchOnly(),
    { id: 'view.find', label: t('common.action.find.label'), run: () => runCommand('view.find') },
    {
      id: 'tab-next',
      label: t('space.shortcuts.palette.nextTab.label'),
      run: nextTab,
    },
    {
      id: 'tab-prev',
      label: t('space.shortcuts.palette.previousTab.label'),
      run: prevTab,
    },
    {
      id: 'tab-close',
      label: t('space.shortcuts.palette.closeTab.label'),
      run: closeTab,
    },
    ...studioPaletteCommands(),
  ];
}
