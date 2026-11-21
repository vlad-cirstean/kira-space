import { type Binding, type Chord, SHORTCUTS, type ShortcutId } from '@shared/domain/shortcuts';

// Menus build synchronously and AppInfo (bridge/control.ts's appInfo()) is fetched async, so a UA
// sniff beats plumbing a platform bridge through for one boolean that never changes for the
// process's lifetime.
export const IS_MAC = navigator.userAgent.includes('Mac');

export function resolveChord(id: ShortcutId): Chord {
  const binding: Binding = SHORTCUTS[id];
  return IS_MAC ? (binding.mac ?? binding.chord) : binding.chord;
}

const MAC_KEY_GLYPH: Record<string, string> = {
  Return: '⏎',
  Backspace: '⌫',
  Delete: '⌦',
  Tab: '⇥',
  Escape: '⎋',
};

const KEY_DISPLAY: Record<string, string> = {
  Return: 'Enter',
  Escape: 'Esc',
  ArrowUp: 'Up',
  ArrowDown: 'Down',
  ArrowLeft: 'Left',
  ArrowRight: 'Right',
};

/** 'Ctrl+C' / '⌘C', matching VS Code's own display order and glyphs. */
export function formatShortcut(id: ShortcutId): string {
  const chord = resolveChord(id);
  if (IS_MAC) {
    const mods = [
      chord.ctrl ? '⌃' : '',
      chord.alt ? '⌥' : '',
      chord.shift ? '⇧' : '',
      chord.cmdOrCtrl ? '⌘' : '',
    ].join('');
    return `${mods}${MAC_KEY_GLYPH[chord.key] ?? chord.key}`;
  }
  const mods = [
    chord.cmdOrCtrl || chord.ctrl ? 'Ctrl' : '',
    chord.shift ? 'Shift' : '',
    chord.alt ? 'Alt' : '',
  ].filter(Boolean);
  return [...mods, KEY_DISPLAY[chord.key] ?? chord.key].join('+');
}

const ARIA_KEY: Record<string, string> = {
  Return: 'Enter',
  Space: 'Space',
};

/** WAI-ARIA `aria-keyshortcuts` form: 'Meta+N', 'Control+Shift+P', 'Alt+F8'. */
export function ariaKeyShortcuts(id: ShortcutId): string {
  const chord = resolveChord(id);
  const mods = [
    chord.ctrl || (!IS_MAC && chord.cmdOrCtrl) ? 'Control' : '',
    IS_MAC && chord.cmdOrCtrl ? 'Meta' : '',
    chord.alt ? 'Alt' : '',
    chord.shift ? 'Shift' : '',
  ].filter(Boolean);
  const key = ARIA_KEY[chord.key] ?? (chord.key.length === 1 ? chord.key.toUpperCase() : chord.key);
  return [...mods, key].join('+');
}
