import type { SettingsPaneProps as GenericSettingsPaneProps } from '@workbench/lib/settingsPaneProps';
import type { Settings } from '../../state/settingsDomain';

// The one prop contract every one of this app's four settings panes takes,
// factored out once rather than repeated four times (CLAUDE.md's "code never duplicates") — the
// exact shape SettingsShell.vue's own `#pane` scoped slot hands each pane, plus `active` (this
// app's own `workbench/SettingsDialog.vue` wrapper binds it per pane,
// `activeSection === '<Section>'`).
//
// `ade` carries the one leaf the dialog edits (the other `ade` leaves are written live by the Plan
// and its panel), so it saves with the rest instead of writing on toggle.
type SettingsSections = Pick<
  Settings,
  'appearance' | 'advanced' | 'git' | 'agent' | 'memory' | 'capture' | 'tracking'
> & {
  ade: Pick<Settings['ade'], 'headlessSettingSources'>;
};

// P115 H9: this app and Kira Studio each declared the identical SettingsPaneProps interface,
// differing only in SettingsSections' own Pick — both now instantiate the shared generic instead.
export type SettingsPaneProps = GenericSettingsPaneProps<SettingsSections>;
