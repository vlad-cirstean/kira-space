import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { type Binding, type Chord, SHORTCUTS } from '@shared/domain/shortcuts';

const chordId = (c: Chord): string =>
  [c.key, c.cmdOrCtrl && 'mod', c.ctrl && 'ctrl', c.shift && 'shift', c.alt && 'alt']
    .filter(Boolean)
    .join('+');

function conflicts(platform: 'mac' | 'other'): string[] {
  const seen = new Map<string, { id: string; scope: string }[]>();
  for (const [id, binding] of Object.entries(SHORTCUTS) as [string, Binding][]) {
    const chord = platform === 'mac' ? (binding.mac ?? binding.chord) : binding.chord;
    const key = chordId(chord);
    seen.set(key, [...(seen.get(key) ?? []), { id, scope: binding.scope }]);
  }
  const out: string[] = [];
  for (const [chord, users] of seen) {
    for (let i = 0; i < users.length; i++) {
      for (let j = i + 1; j < users.length; j++) {
        const a = users[i];
        const b = users[j];
        if (a.scope === b.scope || a.scope === 'global' || b.scope === 'global') {
          out.push(`${chord}: ${a.id} (${a.scope}) vs ${b.id} (${b.scope})`);
        }
      }
    }
  }
  return out;
}

function accelRows(): Map<string, string> {
  const source = readFileSync(
    resolve(import.meta.dir, '../../../../internal/shell/accel.go'),
    'utf8',
  );
  const rows = new Map<string, string>();
  for (const m of source.matchAll(/^\s*"([\w.]+)":\s*\{(.*)\},?$/gm)) rows.set(m[1], m[2]);
  return rows;
}

function goChord(fields: string): string {
  const flag = (name: string) => new RegExp(`(?<![A-Za-z])${name}:\\s*true`).test(fields);
  return chordId({
    key: /Key:\s*"([^"]+)"/.exec(fields)?.[1] ?? '',
    ...(flag('CmdOrCtrl') && { cmdOrCtrl: true as const }),
    ...(flag('Ctrl') && { ctrl: true as const }),
    ...(flag('Shift') && { shift: true as const }),
    ...(flag('Alt') && { alt: true as const }),
  });
}

describe('SHORTCUTS', () => {
  test('no duplicate chord inside a scope or against global, on either platform', () => {
    expect(conflicts('other')).toEqual([]);
    expect(conflicts('mac')).toEqual([]);
  });

  // The native menu accelerators are Go's own copy (internal/shell/accel.go); a drifted row would
  // leave the menu and the displayed chord disagreeing.
  test('every accel.go row matches its SHORTCUTS chord, and every global chord has a row', () => {
    const table = SHORTCUTS as Record<string, Binding>;
    const accel = accelRows();
    for (const [id, fields] of accel) {
      expect(table[id]).toBeDefined();
      expect(table[id].mac).toBeUndefined();
      expect(`${id} ${goChord(fields)}`).toBe(`${id} ${chordId(table[id].chord)}`);
    }
    const globals = Object.entries(table)
      .filter(([, b]) => b.scope === 'global')
      .map(([id]) => id);
    expect(globals.filter((id) => !accel.has(id))).toEqual([]);
  });

  // macOS and WKWebView claim these; a chord on one is swallowed or hijacks a system action.
  // Literal Control plus a letter is a shell control key in the terminal and an emacs key in
  // macOS text fields.
  test('no global or menu chord takes a reserved macOS chord, and none is Control plus a letter', () => {
    const mod = (key: string, extra: Partial<Chord> = {}): Chord => ({
      key,
      cmdOrCtrl: true,
      ...extra,
    });
    const reserved = [
      ...[
        'Q',
        'H',
        'M',
        '`',
        'Space',
        'Tab',
        'C',
        'V',
        'X',
        'Z',
        'A',
        '/',
        'D',
        'L',
        'G',
        'K',
        '[',
        ']',
      ].map((k) => mod(k)),
      mod('H', { alt: true }),
      mod('F', { ctrl: true }),
      ...['Z', 'G', 'K', 'L', 'O', '3', '4', '5'].map((k) => mod(k, { shift: true })),
    ].map(chordId);
    const entries = Object.entries(SHORTCUTS as Record<string, Binding>);
    const globalHits = entries
      .filter(
        ([id, b]) =>
          (b.scope === 'global' || accelRows().has(id)) && reserved.includes(chordId(b.chord)),
      )
      .map(([id]) => id);
    expect(globalHits).toEqual([]);
    const ctrlLetters = entries
      .filter(([, b]) => [b.chord, b.mac].some((c) => c?.ctrl && /^[A-Z]$/.test(c.key)))
      .map(([id]) => id);
    expect(ctrlLetters).toEqual([]);
  });
});
