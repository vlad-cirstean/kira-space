import { createSettingsStore } from '@workbench/state/createSettingsStore';
import { reactive } from 'vue';
import { control } from '../bridge/control';
import { defaultSettings, type Settings, type SettingsPatch } from './settingsDomain';

// 'Database MCP' bypasses draft/Save for its own stated reason — an instant-effect
// CRUD/toggle section, not a staged leaf.
// 'Scripts' is gone — custom scripts are configured only from the Automations module's own
// ScriptDialog.vue now, `openSettingsAt` stays for the Api panes' own deep link
// (CookiesPane.vue/RequestSettingsPane.vue's "Edit global defaults…").
export const sections = [
  'Appearance',
  'Data',
  'Cache',
  'Api',
  'Database MCP',
  'Prompts',
  'Advanced',
] as const;
export type Section = (typeof sections)[number];

// The shared skeleton now lives in
// packages/workbench/src/state/createSettingsStore.ts. `appearanceVersion` — the measured-text
// re-measure signal views/shared/page/columns.ts takes as an explicit reactive dependency, so a
// font change re-measures instead of reusing widths sized for whatever font was active when the
// module first measured — is this app's own extra; Kira Space has no data grid to re-measure.
// `<Section, Settings, SettingsPatch>` — this app's own settingsDomain.ts
// shape, not one shared @shared/domain/settings type.
export const useSettingsStore = createSettingsStore<Section, Settings, SettingsPatch>(
  defaultSettings,
)(control, () => {
  const appearanceVersion = reactive({ n: 0 });
  return {
    extra: { appearanceVersion },
    onApplyAppearance: () => {
      appearanceVersion.n++;
    },
  };
});
