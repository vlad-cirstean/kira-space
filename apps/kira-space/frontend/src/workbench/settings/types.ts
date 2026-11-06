import type { SettingsPaneProps as GenericSettingsPaneProps } from '@workbench/lib/settingsPaneProps';
import type { Settings } from '../../state/settingsDomain';
import type { Settings as StudioSettings } from '../../studio/state/settingsDomain';

// The one prop contract every settings pane takes: the shape SettingsShell.vue's `#pane` slot hands
// each pane, plus `active`. The draft holds Space's sections and the Studio modules' (`data`,
// `cache`, `api`; `studioAdvanced` is Studio's `advanced`, named apart from Space's).
//
// `ade` carries the one leaf the dialog edits (the other `ade` leaves are written live by the Plan
// and its panel), so it saves with the rest instead of writing on toggle.
type SettingsSections = Pick<
  Settings,
  'appearance' | 'advanced' | 'git' | 'agent' | 'memory' | 'capture' | 'tracking'
> & {
  ade: Pick<Settings['ade'], 'headlessSettingSources'>;
  data: StudioSettings['data'];
  cache: StudioSettings['cache'];
  api: StudioSettings['api'];
  studioAdvanced: StudioSettings['advanced'];
};

export type SettingsPaneProps = GenericSettingsPaneProps<SettingsSections>;

/** A numeric field's input text as a draft value. A cleared or non-integer input is NaN, never 0
 *  or a fraction: `Number('')` is 0, a valid in-range value for some leaves, and Go decodes these
 *  leaves into `*int`. The pane's validator shows "Enter a whole number." for NaN. */
export function parseIntField(raw: string): number {
  const text = raw.trim();
  return text === '' ? Number.NaN : Number(text);
}
