import type { SettingsPaneProps as GenericSettingsPaneProps } from '@workbench/lib/settingsPaneProps';
import type { Settings } from '../../state/settingsDomain';

// The one prop contract every one of this app's seven settings panes takes,
// factored out once rather than repeated seven times (CLAUDE.md's "code never duplicates") — the
// exact shape SettingsShell.vue's own `#pane` scoped slot hands each pane, plus `active` (this
// app's own `workbench/SettingsDialog.vue` wrapper binds it per pane, `activeSection ===
// '<Section>'`).
//
// `SettingsSections` — this dialog's own five sections of the full shared `Settings` (everything
// except `dbMcp`, which this dialog edits through its own instant-action controls,
// never through the draft) — is what SettingsShell.vue's own `T` generic is instantiated
// with at this app's `<SettingsShell>` call site (`workbench/SettingsDialog.vue`'s own `:current`/
// `:defaults`), so every pane's prop types must match that same narrowed shape, not the full
// `Settings` type, or Vue's generic prop inference rejects the binding.
type SettingsSections = Pick<Settings, 'appearance' | 'data' | 'cache' | 'advanced' | 'api'>;

// P115 H9: this app and Kira Space each declared the identical SettingsPaneProps interface,
// differing only in SettingsSections' own Pick — both now instantiate the shared generic instead.
export type SettingsPaneProps = GenericSettingsPaneProps<SettingsSections>;

/** A numeric field's input text as a draft value. A cleared or non-integer input is NaN, never 0
 *  or a fraction: `Number('')` is 0, a valid in-range value for some leaves, and Go decodes these
 *  leaves into `*int`. The pane's validator shows "Enter a whole number." for NaN. */
export function parseIntField(raw: string): number {
  const text = raw.trim();
  return text === '' ? Number.NaN : Number(text);
}
