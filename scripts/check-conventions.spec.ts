import { describe, expect, test } from 'bun:test';
import {
  codeParityHits,
  componentsTsHit,
  eventHits,
  fileNameHit,
  folderHits,
  idRegistryHits,
  menuShortcutHits,
  migrationHits,
  oversizeVueHit,
  piniaHits,
  rawIdHits,
  specHelperHits,
  stateAndMenuNameHits,
  testSuffixHit,
} from './check-conventions';

describe('N1 file names', () => {
  test('modules are camelCase, components PascalCase', () => {
    expect(fileNameHit('p/shared/domain/error-codes.ts')?.rule).toBe('N1');
    expect(fileNameHit('p/shared/domain/errorCodes.ts')).toBeUndefined();
    expect(fileNameHit('p/shared/domain/errorCodes.spec.ts')).toBeUndefined();
    expect(fileNameHit('p/w/Foo-bar.vue')?.rule).toBe('N1');
    expect(fileNameHit('p/w/fooBar.vue')?.rule).toBe('N1');
    expect(fileNameHit('p/w/FooBar.vue')).toBeUndefined();
  });
  test('declaration and config files are exempt', () => {
    expect(fileNameHit('p/w/env.d.ts')).toBeUndefined();
    expect(fileNameHit('p/w/vite.config.ts')).toBeUndefined();
  });
});

describe('N2/N3', () => {
  test('flags .test.ts and helper folders', () => {
    expect(testSuffixHit('a/b.test.ts')?.rule).toBe('N2');
    expect(testSuffixHit('a/b.spec.ts')).toBeUndefined();
    expect(folderHits('p/workbench/src/util/x.ts').map((h) => h.path)).toEqual([
      'p/workbench/src/util',
    ]);
    expect(folderHits('p/workbench/src/lib/x.ts')).toEqual([]);
    expect(folderHits('p/util/lib/x.ts')).toEqual([]);
  });
});

describe('N6 error-code parity', () => {
  const go = (codes: string[], extra = '') =>
    `${codes.map((c) => `CodeX Code = "${c}"`).join('\n')}\n${extra}`;
  const ts = (codes: string[], extra = '') =>
    `export const ERROR_CODES = [${codes.map((c) => `'${c}'`).join(', ')}] as const;\n` +
    `export const CLIENT_ERROR_CODES = ['E_NETWORK'] as const;\n${extra}`;
  test('same sets pass, client-only codes are ignored', () => {
    expect(codeParityHits(go(['E_A', 'E_B']), ts(['E_B', 'E_A']))).toEqual([]);
  });
  test('a code on one side only fails and names the side', () => {
    const hits = codeParityHits(go(['E_A', 'E_B']), ts(['E_A', 'E_C']));
    expect(hits).toHaveLength(1);
    expect(hits[0]?.message).toContain('only in Go: E_B');
    expect(hits[0]?.message).toContain('only in TS: E_C');
  });
  test('a retired-code registry on either side fails', () => {
    const goLegacy = go(['E_A'], 'var Legacy = map[Code]Code{}');
    const tsLegacy = ts(['E_A'], 'export const LEGACY_ERROR_CODES = [] as const;');
    expect(codeParityHits(goLegacy, ts(['E_A']))).toHaveLength(1);
    expect(codeParityHits(go(['E_A']), tsLegacy)).toHaveLength(1);
    expect(codeParityHits(go(['E_A']), ts(['E_A']))).toEqual([]);
  });
});

describe('N4 Pinia ids', () => {
  const store = (exportName: string, id: string) =>
    `export const ${exportName} = defineStore('${id}', () => {});`;
  test('id is the export name without use/Store, camelCase', () => {
    expect(piniaHits('a.ts', store('useConfirmDialogStore', 'confirmDialog'))).toEqual([]);
    expect(piniaHits('a.ts', store('useDbMcp', 'dbmcp'))[0]?.rule).toBe('N4');
  });
  test('factory exports resolve to the nearest preceding export', () => {
    const src = `export function createModeStore() {\n  return defineStore('mode', () => {});\n}`;
    expect(piniaHits('a.ts', src)).toEqual([]);
    expect(piniaHits('a.ts', src.replace("'mode'", "'modes'"))).toHaveLength(1);
  });
});

describe('N5 event names', () => {
  const literal = /'(kira:[^']*)'/g;
  test('three camelCase segments only', () => {
    expect(eventHits('e.ts', "a: 'kira:op:update', b: 'kira:app:metrics'", literal)).toEqual([]);
    const hits = eventHits('e.ts', "a: 'kira:open-settings', b: 'kira:menu:view-find'", literal);
    expect(hits).toHaveLength(1);
    expect(hits[0]?.message).toContain('kira:menu:view-find');
  });
});

describe('N7 migrations', () => {
  const files = new Map([
    ['0003_p150_pin.sql', 'ALTER TABLE x ADD COLUMN created_at TEXT;'],
    ['0004_p266_ok.sql', 'ALTER TABLE x ADD COLUMN created_at INTEGER;'],
    ['0005_p266_text.sql', 'CREATE TABLE y (id INTEGER, updated_at  TEXT NOT NULL);'],
    ['0006_badname.sql', 'SELECT 1;'],
  ]);
  test('only files above the baseline are checked', () => {
    const hits = migrationHits('d', 3, files);
    expect(hits.map((h) => h.path)).toEqual(['d/0005_p266_text.sql', 'd/0006_badname.sql']);
  });
});

describe('N7 strict tables', () => {
  const files = new Map([
    ['0082_p500_old.sql', 'CREATE TABLE a (id TEXT);'],
    [
      '0083_p552_ok.sql',
      'CREATE TABLE b (id TEXT PRIMARY KEY) STRICT;\nCREATE TABLE c (x INTEGER) STRICT, WITHOUT ROWID;',
    ],
    ['0084_p552_loose.sql', 'CREATE TABLE d (\n  id TEXT\n);\nCREATE TABLE e (id TEXT) STRICT;'],
  ]);
  test('tables from strictFrom must be STRICT', () => {
    const hits = migrationHits('d', 32, files, 83);
    expect(hits.map((h) => h.message)).toEqual(['table d must be STRICT']);
  });
});

describe('N8 spec helpers', () => {
  const body = (name: string, tail = 'a') =>
    `async function ${name}(page: Page): Promise<void> {\n  await page.click('${tail}');\n  await page.click('b');\n}\n`;
  test('a verbatim copy in two specs is flagged in both', () => {
    const specs = new Map([
      ['a.spec.ts', body('openIt')],
      ['b.spec.ts', body('openIt')],
    ]);
    expect(specHelperHits(specs, new Map()).map((h) => h.path)).toEqual(['a.spec.ts', 'b.spec.ts']);
  });
  test('a same-named helper with a different body is a specialisation', () => {
    const specs = new Map([
      ['a.spec.ts', body('openIt', 'x')],
      ['b.spec.ts', body('openIt', 'y')],
    ]);
    expect(specHelperHits(specs, new Map())).toEqual([]);
  });
  test('a copy of a support export is flagged, a different body is not', () => {
    const support = new Map([['s/tree.ts', `export ${body('openIt')}`]]);
    expect(specHelperHits(new Map([['a.spec.ts', body('openIt')]]), support)).toHaveLength(1);
    expect(specHelperHits(new Map([['a.spec.ts', body('openIt', 'z')]]), support)).toEqual([]);
  });
  test('a multi-line signature does not end the body early', () => {
    const fn = (extra: string) =>
      `function f(\n  a: string,\n  b: string,\n): string {\n  return a + b + '${extra}';\n}\n`;
    const specs = new Map([
      ['a.spec.ts', fn('1')],
      ['b.spec.ts', fn('2')],
    ]);
    expect(specHelperHits(specs, new Map())).toEqual([]);
  });
});

describe('N9 components dir', () => {
  test('.ts under components/ is flagged except ui/ and specs', () => {
    expect(componentsTsHit('packages/theme/src/components/frames.ts')?.rule).toBe('N9');
    expect(componentsTsHit('packages/theme/src/components/ui/button/index.ts')).toBeUndefined();
    expect(componentsTsHit('packages/theme/src/components/Foo.spec.ts')).toBeUndefined();
    expect(componentsTsHit('packages/theme/src/lib/frames.ts')).toBeUndefined();
    expect(componentsTsHit('apps/x/frontend/src/components/a.ts')).toBeUndefined();
  });
});

describe('N10 oversize .vue', () => {
  const big = `${'x\n'.repeat(700)}`;
  test('over 600 lines fails unless a named host', () => {
    expect(oversizeVueHit('p/Foo.vue', big)?.rule).toBe('N10');
    expect(oversizeVueHit('p/Foo.vue', 'x\n'.repeat(100))).toBeUndefined();
    expect(oversizeVueHit('p/SlickGridHost.vue', big)).toBeUndefined();
    expect(oversizeVueHit('p/foo.ts', big)).toBeUndefined();
  });
});

describe('N11 menu shortcuts', () => {
  const menu = "{ shortcut: 'a.run' }, { shortcut: 'b.go' }, { shortcut: 'tree.open' }";
  const scopes = new Map([['b.go', 'global']]);
  test('unbound non-global shortcuts fail', () => {
    const hits = menuShortcutHits('d/menu.ts', menu, ["useShortcut('a.run', f)"], scopes);
    expect(hits.map((h) => h.message)).toEqual([expect.stringContaining("'tree.open'")]);
  });
  test('useTreeShortcuts binds tree.* ids', () => {
    const hits = menuShortcutHits(
      'd/menu.ts',
      menu,
      ["shortcutFor(e, ['a.run'])", 'useTreeShortcuts(x)'],
      scopes,
    );
    expect(hits).toEqual([]);
  });
});

describe('N13 id registry parity', () => {
  const go = `Worktree Prefix = "wt"\n\tShot Prefix = "shot"\nvar sizes = map[Prefix]int{\n\tWorktree: 10, Shot: 12,\n}`;
  test('equal registries pass', () => {
    expect(idRegistryHits(go, 'export const PREFIXES = { wt: 10, shot: 12 } as const;')).toEqual(
      [],
    );
  });
  test('a differing length or prefix fails', () => {
    const hits = idRegistryHits(go, 'export const PREFIXES = { wt: 12, abc: 12 } as const;');
    expect(hits[0]?.message).toContain('wt: Go 10, TS 12');
    expect(hits[0]?.message).toContain('shot: Go 12, TS missing');
    expect(hits[0]?.message).toContain('abc: only in TS');
  });
});

describe('N14 raw id generators', () => {
  test('randomUUID and direct nanoid or uuid imports fail', () => {
    expect(rawIdHits('a.ts', 'const id = crypto.randomUUID();')[0]?.rule).toBe('N14');
    expect(rawIdHits('a.ts', "import { v7 } from 'uuid';")).toHaveLength(1);
    expect(rawIdHits('a.ts', "import { nanoid } from 'nanoid';")).toHaveLength(1);
    expect(rawIdHits('a.ts', 'const id = Math.random().toString(36).slice(2);')[0]?.rule).toBe(
      'N14',
    );
  });
  test('the helper and clean code pass', () => {
    expect(rawIdHits('packages/shared/ids.ts', "import { v7 } from 'uuid';")).toEqual([]);
    expect(rawIdHits('a.ts', "import { newUuid } from '@shared/ids';")).toEqual([]);
  });
});

describe('N12 state and menu names', () => {
  test('one-file state dir and misnamed menu builders fail', () => {
    const hits = stateAndMenuNameHits([
      'a/state/only.ts',
      'b/state/one.ts',
      'b/state/two.ts',
      'c/menus.ts',
      'c/rowMenu.ts',
      'c/useRowMenu.ts',
      'c/menu.ts',
    ]);
    expect(hits.map((h) => h.path)).toEqual(['c/menus.ts', 'c/rowMenu.ts', 'a/state']);
  });
});
