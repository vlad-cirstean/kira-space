import type { ShortcutId } from '@shared/domain/shortcuts';
import { IS_MAC as isMac, resolveChord } from '@theme/shortcutText';

/** 'Cmd' on mac, 'Ctrl' elsewhere: copy like "Cmd-click". */
export function primaryModifierName(): 'Cmd' | 'Ctrl' {
  return isMac ? 'Cmd' : 'Ctrl';
}

/** The platform's primary modifier: Cmd on mac, Ctrl elsewhere. */
export function isPrimaryModifier(e: Pick<KeyboardEvent, 'metaKey' | 'ctrlKey'>): boolean {
  return isMac ? e.metaKey : e.ctrlKey;
}

const DOM_KEY: Record<string, string> = { Return: 'Enter', Space: ' ' };

// Layout-correct match: `e.key` first, so Dvorak/Colemak follow the printed label. Falls back to
// the physical `e.code` only when `e.key` is not ASCII — macOS composes Option+letter into a symbol
// (⌥⌘C reports `"ç"`), and a non-Latin layout reports its own script.
function domCodeForKey(key: string): string | null {
  if (/^[A-Za-z]$/.test(key)) return `Key${key.toUpperCase()}`;
  if (/^[0-9]$/.test(key)) return `Digit${key}`;
  return null;
}

// `chord.ctrl` is a literal Control on every platform, distinct from cmdOrCtrl.
function matchesShortcut(id: ShortcutId, e: KeyboardEvent): boolean {
  if (id === 'menu.open' && e.key === 'ContextMenu') return true;
  const chord = resolveChord(id);
  const wantsCtrl = Boolean(chord.ctrl) || (!isMac && Boolean(chord.cmdOrCtrl));
  const wantsMeta = isMac && Boolean(chord.cmdOrCtrl);
  if (wantsCtrl !== e.ctrlKey || wantsMeta !== e.metaKey) return false;
  if (Boolean(chord.shift) !== e.shiftKey) return false;
  if (Boolean(chord.alt) !== e.altKey) return false;
  const domCode = domCodeForKey(chord.key);
  if (domCode) {
    if (/^[A-Za-z0-9]$/.test(e.key)) return e.key.toUpperCase() === chord.key.toUpperCase();
    return e.code === domCode;
  }
  const domKey = DOM_KEY[chord.key] ?? chord.key;
  return e.key.toUpperCase() === domKey.toUpperCase();
}

/** The first id in `ids` whose binding matches `e`, else null. */
export function shortcutFor(e: KeyboardEvent, ids: readonly ShortcutId[]): ShortcutId | null {
  return ids.find((id) => matchesShortcut(id, e)) ?? null;
}
