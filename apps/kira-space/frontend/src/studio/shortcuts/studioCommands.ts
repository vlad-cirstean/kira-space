import { t } from '@theme/i18n';
import { runCommand } from '@workbench/shortcuts/commands';
import type { PaletteCommand } from '@workbench/shortcuts/palette';

/** API-mode palette commands. A command belongs here only when its sole other path is a
 *  right-click menu or an empty-start button: anything with a visible control, menu-bar item or
 *  chord is not listed. CollectionsPanel.vue registers each, so both are no-ops outside API mode. */
export function studioPaletteCommands(): PaletteCommand[] {
  return [
    {
      id: 'api.dynamicValues',
      label: t('studio.api.dynamicValues.title'),
      run: () => runCommand('api.dynamicValues'),
    },
    {
      id: 'api.importCurl',
      label: t('studio.api.common.importCurl.label'),
      run: () => runCommand('api.importCurl'),
    },
  ];
}
