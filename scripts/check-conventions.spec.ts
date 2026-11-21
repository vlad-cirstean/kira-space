import { describe, expect, test } from 'bun:test';
import {
  buttonVariantHits,
  codeParityHits,
  componentsTsHit,
  cssSelectionHits,
  engineThemeHits,
  eventHits,
  fileNameHit,
  folderHits,
  idRegistryHits,
  menuShortcutHits,
  migrationHits,
  neutralTokenHits,
  oversizeVueHit,
  piniaHits,
  rawIdHits,
  rawSqlHits,
  selectionClassHits,
  shortcutDisplayHits,
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

describe('N16 raw SQL', () => {
  const repo = 'apps/kira-space/internal/storage/repos/x.go';
  test('a string-literal statement fails, in every call form', () => {
    expect(rawSqlHits(repo, 'db.Exec(`DELETE FROM t`)')[0]?.rule).toBe('N16');
    expect(rawSqlHits(repo, 'tx.QueryRow("SELECT 1 FROM "+table)')).toHaveLength(1);
    expect(rawSqlHits(repo, 'db.QueryContext(ctx, "SELECT 1")')).toHaveLength(1);
    expect(rawSqlHits(repo, 'db.Prepare(`SELECT ?`)')).toHaveLength(1);
    expect(rawSqlHits(repo, 'db.Query(fmt.Sprintf("SELECT %s", c), a)')).toHaveLength(1);
    expect(rawSqlHits(repo, 'q, _ := sqlbuilder.SQLite.Interpolate(s, a)')).toHaveLength(1);
  });
  test('builder text, comments and other literals pass', () => {
    expect(rawSqlHits(repo, 'q, args := sb.Build()\nrows, err := db.Query(q, args...)')).toEqual(
      [],
    );
    expect(rawSqlHits(repo, 'rows, err := db.QueryContext(ctx, q, "x")')).toEqual([]);
    expect(rawSqlHits(repo, '// db.Exec(`DELETE FROM t`)\n/* tx.Query("x") */')).toEqual([]);
  });
  test('the two allowlisted raw spots pass; other SQL beside them fails', () => {
    const maint = 'apps/kira-space/internal/storage/repos/maintenance.go';
    const oplog = 'apps/kira-space/internal/storage/repos/oplogforeign.go';
    expect(rawSqlHits(maint, 'db.Exec(`PRAGMA incremental_vacuum`)')).toEqual([]);
    expect(rawSqlHits(oplog, 'db.QueryRow(`PRAGMA data_version`)')).toEqual([]);
    expect(rawSqlHits(oplog, 'db.Exec(`DELETE FROM op_log`)')).toHaveLength(1);
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

describe('N17 semantic button variant', () => {
  const btn = (variant: string, label: string, extra = '') =>
    `<Button variant="${variant}" ${extra}@click="go(a => b)">{{ t('${label}') }}</Button>`;
  const hits = (src: string) => buttonVariantHits('p/w/X.vue', src);

  test('approve labels need dialog-approve', () => {
    expect(hits(btn('dialog-approve', 'common.action.approve.label'))).toEqual([]);
    expect(hits(btn('dialog-primary', 'common.action.approve.label'))[0]?.rule).toBe('N17');
    expect(hits(btn('dialog', 'studio.x.allow.label'))).toHaveLength(1);
    expect(
      hits(btn('dialog-primary', 'studio.dbMcp.approval.exemption.approve.label')),
    ).toHaveLength(1);
    expect(hits(btn('dialog-primary', 'common.action.send.label'))).toHaveLength(1);
  });
  test('deny labels need dialog-deny', () => {
    expect(hits(btn('dialog-deny', 'common.action.deny.label'))).toEqual([]);
    expect(hits(btn('dialog-danger', 'common.action.deny.label'))).toHaveLength(1);
    expect(hits(btn('dialog', 'x.proof.reject.label'))).toHaveLength(1);
    expect(hits(btn('dialog-danger', 'workbench.approvals.denyAll.label'))).toHaveLength(1);
  });
  test('destructive labels need dialog-danger', () => {
    expect(hits(btn('dialog-danger', 'common.action.delete.label'))).toEqual([]);
    expect(hits(btn('dialog-primary', 'common.action.remove.label'))).toHaveLength(1);
    expect(hits(btn('dialog', 'common.action.discard.label'))).toHaveLength(1);
    expect(hits(btn('dialog', 'git.common.forceDelete.confirm'))).toHaveLength(1);
  });
  test('other labels and non-dialog variants pass', () => {
    expect(hits(btn('dialog', 'common.action.cancel.label'))).toEqual([]);
    expect(hits(btn('dialog-primary', 'common.action.save.label'))).toEqual([]);
    expect(hits(btn('toolbar', 'common.action.delete.label'))).toEqual([]);
    expect(hits(btn('dialog', 'x.reviewAllowAll.label'))).toEqual([]);
  });
  test('labels resolve from def.labels too', () => {
    const src = `<Button variant="dialog-danger">{{ labels?.deny ?? t('common.action.deny.label') }}</Button>`;
    expect(hits(src)).toHaveLength(1);
    expect(hits(`<Button variant="dialog-primary">{{ labels?.approve }}</Button>`)).toHaveLength(1);
  });
  test('tone classes on a dialog variant are banned', () => {
    expect(hits(btn('dialog', 'common.action.edit.label', 'class="text-error" '))).toHaveLength(1);
    expect(
      hits(btn('dialog', 'common.action.edit.label', 'class="w-full border-ok/50" ')),
    ).toHaveLength(1);
    expect(hits(btn('dialog', 'common.action.edit.label', 'class="w-full text-errors" '))).toEqual(
      [],
    );
    expect(hits(btn('toolbar', 'common.action.edit.label', 'class="text-error" '))).toEqual([]);
  });
  test('an allow comment before the element exempts it', () => {
    const src = `<!-- conventions-allow N17: sends a test mail, not an approval -->\n${btn('dialog', 'common.sendTest.label')}`;
    expect(hits(src)).toEqual([]);
    expect(
      hits(`<!-- conventions-allow N17: x -->\n<p/>\n${btn('dialog', 'a.send.label')}`),
    ).toHaveLength(1);
  });
  test('self-closing and multiline tags parse', () => {
    expect(hits('<Button variant="dialog-primary" :x="a > b" />')).toEqual([]);
    expect(
      hits(
        `<Button\n  variant="dialog-primary"\n  :pending="a > b"\n>\n  {{ t('x.approve.label') }}\n</Button>`,
      ),
    ).toHaveLength(1);
  });
});

describe('N18 selection stays neutral', () => {
  const tokens = (select: string, on = 'var(--kira-fg)') =>
    [
      `--kira-select: ${select};`,
      '--kira-select-edge: #71717a;',
      '--kira-select-text: #45454d;',
      '--kira-focus: #a1a1aa;',
      '--kira-bg-input: #27272c;',
      `--kira-on: ${on};`,
      '--kira-on-fg: var(--kira-bg-chrome);',
    ].join('\n');

  test('a blue selection token fails, the grey passes', () => {
    expect(neutralTokenHits(tokens('#36363c'))).toEqual([]);
    expect(neutralTokenHits(tokens('#1f3354'))[0]?.rule).toBe('N18');
    expect(neutralTokenHits(tokens('#36363c', '#3b6ed1'))[0]?.message).toContain('--kira-on');
    expect(neutralTokenHits('--kira-select: #36363c;')).toHaveLength(6);
  });

  test('Monaco keeps every required grey key and ::selection stays themed', () => {
    const keys = [
      'editor.selectionBackground',
      'editor.inactiveSelectionBackground',
      'editor.selectionHighlightBackground',
      'editor.wordHighlightStrongBackground',
      'editor.hoverHighlightBackground',
      'editorSuggestWidget.selectedBackground',
      'list.activeSelectionBackground',
      'list.focusBackground',
      'list.inactiveSelectionBackground',
      'list.inactiveFocusBackground',
      'list.dropBackground',
      'peekViewResult.selectionBackground',
      'inputOption.activeBackground',
      'list.activeSelectionForeground',
      'inputOption.activeForeground',
      'inputOption.activeBorder',
      'peekView.border',
      'minimap.selectionHighlight',
    ];
    const monaco = (skip = '', literal = '') =>
      keys
        .filter((k) => k !== skip)
        .map(
          (k) =>
            `      '${k}': ${k === literal ? "'#264f78'" : "cssVar('--kira-select', '#36363c')"},`,
        )
        .join('\n');
    const term = "selectionBackground: cssVar('--kira-select-text'),";
    const base = '::selection { background-color: var(--kira-select-text); }';
    expect(engineThemeHits(monaco(), term, base)).toEqual([]);
    expect(engineThemeHits(monaco('list.dropBackground'), term, base)[0]?.message).toContain(
      'list.dropBackground',
    );
    expect(engineThemeHits(monaco('', 'peekView.border'), term, base)).toHaveLength(1);
    expect(engineThemeHits(monaco(), "selectionBackground: '#264f78',", base)).toHaveLength(1);
    expect(engineThemeHits(monaco(), term, '')).toHaveLength(1);
  });

  test('blue utilities marking selection or on-state fail, grey on-state passes', () => {
    const hits = (path: string, src: string) => selectionClassHits(path, src);
    expect(hits('p/X.vue', "selected ? 'border-focus' : 'border-border'")[0]?.rule).toBe('N18');
    expect(hits('p/X.vue', "x && 'ring-1'; isSelected && 'bg-info/10'")).toHaveLength(1);
    expect(hits('p/X.vue', "'data-[state=on]:decoration-primary underline'")).toHaveLength(1);
    expect(hits('p/X.vue', "'aria-selected:bg-accent'")).toHaveLength(1);
    expect(hits('p/X.vue', "selected ? 'border-select-edge' : 'border-border'")).toEqual([]);
    expect(hits('p/X.vue', "'text-info hover:underline'")).toEqual([]);
    const checkbox = 'packages/theme/src/components/ui/checkbox/Checkbox.vue';
    expect(hits(checkbox, "'data-[state=checked]:bg-primary'")[0]?.rule).toBe('N18');
    expect(hits(checkbox, "'data-[state=checked]:bg-on text-on-fg'")).toEqual([]);
    expect(
      hits(
        'packages/theme/src/components/ui/progress/Progress.vue',
        'class="bg-primary size-full"',
      ),
    ).toHaveLength(1);
    expect(hits('p/X.spec.ts', "selected ? 'border-focus' : ''")).toEqual([]);
  });

  test('CSS selection rules and blue literals fail outside tokens.css', () => {
    expect(
      cssSelectionHits('p/a.css', '.row.selected { outline: 1px solid var(--kira-accent); }'),
    ).toHaveLength(1);
    expect(
      cssSelectionHits('p/a.css', '.row.inserted { background: var(--kira-accent); }'),
    ).toEqual([]);
    expect(cssSelectionHits('p/a.css', '.x { color: #3b6ed1; }')).toHaveLength(1);
    expect(cssSelectionHits('p/tokens.css', ':root { --a: #3b6ed1; }')).toEqual([]);
    expect(cssSelectionHits('p/a.css', '.x { color: #36363c; } /* #3b6ed1 */')).toEqual([]);
  });
});

describe('N19 every shortcut shows', () => {
  const scopes = new Map([
    ['x.y', 'view'],
    ['x.z', 'view'],
    ['g.go', 'global'],
    ['n.one', 'view'],
  ]);
  const noControl = { 'n.one': 'no control' };
  const shows = (...ids: string[]): string => ids.map((id) => `<Btn shortcut="${id}" />`).join('');
  // Coverage filler: every id shown far from the binding files under test.
  const far: [string, string] = ['p/far/F.vue', shows('x.y', 'x.z', 'g.go')];
  const bind = "useShortcut(root, ['x.y'], run)";

  test('an id with no display site fails; a SHORTCUT_NO_CONTROL id passes', () => {
    const hits = shortcutDisplayHits(scopes, new Map([['p/a/A.vue', shows('x.y')]]), noControl, {});
    expect(hits.map((h) => h.message)).toEqual([
      expect.stringContaining("'x.z'"),
      expect.stringContaining("'g.go'"),
    ]);
  });
  test('prop, ternary, menu field, moduleShortcut, ShortcutKbd and Go accelerators count', () => {
    const sources = new Map([
      ['p/a/A.vue', ':shortcut="on ? \'x.y\' : undefined" <ShortcutKbd id="x.z" />'],
      ['p/a/menu.ts', "{ shortcut: moduleShortcut('api', 'g.go') }"],
      ['p/a/m.go', 'Accelerator: Shortcuts["n.one"].Accelerator()'],
    ]);
    expect(shortcutDisplayHits(scopes, sources, {}, {})).toEqual([]);
  });
  test('a bound id with no display in its directory fails; a sibling display passes', () => {
    const gap = shortcutDisplayHits(scopes, new Map([far, ['p/a/B.vue', bind]]), noControl, {});
    expect(gap.map((h) => h.path)).toEqual(['p/a/B.vue']);
    const sibling = new Map([far, ['p/a/B.vue', bind], ['p/a/C.vue', shows('x.y')]]);
    expect(shortcutDisplayHits(scopes, sibling, noControl, {})).toEqual([]);
  });
  test('a mapping passes only when the target shows the id', () => {
    const mapped = { 'p/a/B.vue': 'p/z/Y.vue' };
    const good = new Map([far, ['p/a/B.vue', bind], ['p/z/Y.vue', shows('x.y')]]);
    expect(shortcutDisplayHits(scopes, good, noControl, mapped)).toEqual([]);
    const bad = new Map([far, ['p/a/B.vue', bind], ['p/z/Y.vue', shows('x.z')]]);
    expect(shortcutDisplayHits(scopes, bad, noControl, mapped)[0]?.message).toContain(
      'does not show',
    );
  });
  test('registerCommand ids count as bindings; a null mapping exempts the file', () => {
    const sources = new Map([far, ['p/a/B.vue', "registerCommand('x.y', run)"]]);
    expect(shortcutDisplayHits(scopes, sources, noControl, {}).map((h) => h.path)).toEqual([
      'p/a/B.vue',
    ]);
    expect(shortcutDisplayHits(scopes, sources, noControl, { 'p/a/B.vue': null })).toEqual([]);
  });
});
