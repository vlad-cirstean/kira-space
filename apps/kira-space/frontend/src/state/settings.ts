import { createSettingsStore } from '@workbench/state/createSettingsStore';
import { reactive } from 'vue';
import { control } from '../bridge/control';
import { defaultSettings, type Settings, type SettingsPatch } from './settingsDomain';

// `sections` is the dialog's nav: Space's panes, then the Studio modules'.
export const sections = [
  'Appearance',
  'Git',
  'Shared config',
  'CLI tools',
  'File types',
  'Capture',
  'Updates',
  'Mobile access',
  'Agent',
  'Memory',
  'Data',
  'Cache',
  'Api',
  'Database MCP',
  'Prompts',
  'Advanced',
  'Experimental',
] as const;
export type Section = (typeof sections)[number];

// `appearanceVersion` is the measured-text re-measure signal the data grids take as a reactive
// dependency, so a font change re-measures instead of reusing stale column widths.
export const useSettingsStore = createSettingsStore<Section, Settings, SettingsPatch>(
  defaultSettings,
)(control, ({ settingsState }) => {
  const appearanceVersion = reactive({ n: 0 });
  return {
    extra: { appearanceVersion },
    // P92 item 9: 0 = follow appearance.fontSize — git-ui's theme/git.css fallback
    // (var(--kira-graph-font-size, var(--kira-font-size))) is what "follow" actually means, so removing
    // the property (not writing 0px) is what lets that fallback apply.
    onApplyAppearance(): void {
      const root = document.documentElement.style;
      const graph = settingsState.git.graphFontSize;
      if (graph > 0) root.setProperty('--kira-graph-font-size', `${graph}px`);
      else root.removeProperty('--kira-graph-font-size');
      appearanceVersion.n++;
    },
  };
});
