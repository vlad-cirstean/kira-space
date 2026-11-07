import type { SettingsPaneProps as GenericSettingsPaneProps } from '@workbench/lib/settingsPaneProps';
import type { Settings } from '../../state/settingsDomain';

// The one prop contract every settings pane takes: the shape SettingsShell.vue's `#pane` slot hands
// each pane, plus `active`.
//
// `ade` carries the one leaf the dialog edits (the other `ade` leaves are written live by the Plan
// and its panel), so it saves with the rest instead of writing on toggle.
type SettingsSections = Pick<
  Settings,
  | 'appearance'
  | 'advanced'
  | 'git'
  | 'agent'
  | 'memory'
  | 'experimental'
  | 'capture'
  | 'tracking'
  | 'data'
  | 'cache'
  | 'api'
> & {
  ade: Pick<Settings['ade'], 'headlessSettingSources'>;
};

export type SettingsPaneProps = GenericSettingsPaneProps<SettingsSections>;

/** A numeric field's input text as a draft value. A cleared or non-integer input is NaN, never 0
 *  or a fraction: `Number('')` is 0, a valid in-range value for some leaves, and Go decodes these
 *  leaves into `*int`. The pane's validator shows "Enter a whole number." for NaN. */
export function parseIntField(raw: string): number {
  const text = raw.trim();
  return text === '' ? Number.NaN : Number(text);
}
