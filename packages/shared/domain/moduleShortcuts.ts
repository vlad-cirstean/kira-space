import type { AppMode } from './mode';
import type { ShortcutId } from './shortcuts';

/** The global chords whose effect follows the active module. */
export type ModuleAction = Extract<
  ShortcutId,
  'app.new' | 'app.newAlternate' | 'app.import' | 'api.environments'
>;

/** Per module, the chords its dispatcher (`MODULE_ACTIONS`) handles. Data only so a package can
 *  name a hint without importing the app. */
export const MODULE_SHORTCUTS = {
  studio: ['app.new', 'app.import'],
  api: ['app.new', 'app.newAlternate', 'app.import', 'api.environments'],
  automations: ['app.new', 'app.newAlternate'],
  ade: ['app.new'],
  git: ['app.new'],
} as const satisfies Partial<Record<AppMode, readonly ModuleAction[]>>;

export type MappedModule = keyof typeof MODULE_SHORTCUTS;

/** A control's shortcut hint for a module's chord; an unmapped pair is a type error, so a hint can
 *  never claim a chord its module does not dispatch. */
export function moduleShortcut<
  M extends MappedModule,
  A extends (typeof MODULE_SHORTCUTS)[M][number],
>(_mode: M, action: A): A {
  return action;
}
