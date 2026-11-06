import { createSettingsStore } from '@workbench/state/createSettingsStore';
import { control } from '../bridge/control';
import { defaultSettings, type Settings, type SettingsPatch } from './settingsDomain';

// The Studio modules' settings (data, cache, advanced, api, dbMcp). The dialog is Space's
// workbench/SettingsDialog.vue; deep links open it through Space's settings store.
export const useStudioSettingsStore = createSettingsStore<string, Settings, SettingsPatch>(
  defaultSettings,
  'studioSettings',
)(control, () => ({ extra: {} }));
