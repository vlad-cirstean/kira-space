import { createSettingsStore } from '@workbench/state/createSettingsStore';
import { control } from '../bridge/control';
import { defaultSettings, type Settings, type SettingsPatch } from './settingsDomain';

// Kira Studio's own state/settings.ts, ported. Settings itself stays this app's own
// three-section schema (./settingsDomain) — every section still round-trips
// through this store even though SettingsDialog.vue here (a small rewrite, not a port) surfaces
// only Appearance/Git/Advanced's own gitLogLevel leaf; the sections list below
// is this app's own dialog surface, not the full Settings shape.
export const sections = [
  'Appearance',
  'Git',
  'CLI tools',
  'File types',
  'Capture',
  'Updates',
  'Mobile access',
  'Agent',
  'Memory',
  'Prompts',
  'Advanced',
] as const;
export type Section = (typeof sections)[number];

// The shared skeleton now lives in
// packages/workbench/src/state/createSettingsStore.ts — this app has nothing to add, so this
// store's own public surface is unchanged.
// `<Section, Settings, SettingsPatch>` — this app's own settingsDomain.ts
// shape, not one shared @shared/domain/settings type.
export const useSettingsStore = createSettingsStore<Section, Settings, SettingsPatch>(
  defaultSettings,
)(control, ({ settingsState }) => ({
  extra: {},
  // P92 item 9: 0 = follow appearance.fontSize — git-ui's theme/git.css fallback
  // (var(--kira-graph-font-size, var(--kira-font-size))) is what "follow" actually means, so removing
  // the property (not writing 0px) is what lets that fallback apply.
  onApplyAppearance(): void {
    const root = document.documentElement.style;
    const graph = settingsState.git.graphFontSize;
    if (graph > 0) root.setProperty('--kira-graph-font-size', `${graph}px`);
    else root.removeProperty('--kira-graph-font-size');
  },
}));
