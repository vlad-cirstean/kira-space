import { t } from '@theme/i18n';
import { runCommand } from '@workbench/shortcuts/commands';
import type { PaletteCommand } from '@workbench/shortcuts/palette';
import { useAppUpdateStore } from '../state/appUpdate';
import { useLayoutStore } from '../state/layout';
import { useOpenedFilesWindowStore } from '../state/openedFilesWindow';
import { useSettingsStore } from '../state/settings';
import { useTabsStore } from '../state/tabs';

/** Space's palette commands; call once from App setup (the stores resolve there). The getter re-reads labels when the locale changes. */
export function spacePaletteCommands(): () => PaletteCommand[] {
  const layoutStore = useLayoutStore();
  const tabsStore = useTabsStore();
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
      run: tabsStore.activateNextTab,
    },
    {
      id: 'tab-prev',
      label: t('space.shortcuts.palette.previousTab.label'),
      run: tabsStore.activatePrevTab,
    },
    {
      id: 'tab-close',
      label: t('space.shortcuts.palette.closeTab.label'),
      run: tabsStore.closeActiveTab,
    },
  ];
}
