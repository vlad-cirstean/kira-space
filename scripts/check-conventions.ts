#!/usr/bin/env bun
/**
 * Code convention guard N1-N7 and N8-N15.
 *
 * N1 file names (.ts camelCase, .vue PascalCase), N2 no `.test.ts`, N3 no util/utils/helpers
 * folder, N4 Pinia id = export name without `use`/`create` and `Store`, N5 event names
 * `kira:<scope>:<name>` in camelCase, N7 new migrations follow `NNNN_<phase>_<slug>.sql` and add no
 * TEXT `*_at` column, N6 the Go (`internal/ipcerr/codes.go`) and TS
 * (`packages/shared/domain/errorCodes.ts`) error-code registries list the same codes.
 *
 * N8 a spec helper (3+ lines) copied verbatim into 2+ spec files, or copied from a
 * `tests/<suite>/support` or `workbench/testing/ui` export, N9 no non-spec `.ts` under a package
 * `components/` dir (outside `components/ui`), N10 a `.vue` over 600 lines needs a named
 * engine-host exemption, N11 a non-global `shortcut: '<id>'` in a `menu.ts` is bound in its feature
 * dir, N12 a `state/` dir holds 2+ files and menu builders live in `menu.ts`.
 *
 * N15 no tracked PNG under `screenshots/` or `site/` (product screenshots are WebP); Playwright
 * `toHaveScreenshot` baselines in `-snapshots` dirs under `site/tests` are PNG only and stay.
 *
 * No pending list: every hit fails. N6 also fails on any retired-code registry (`Legacy` in Go,
 * `LEGACY_ERROR_CODES` in TS).
 */
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

export type Rule =
  | 'N1'
  | 'N2'
  | 'N3'
  | 'N4'
  | 'N5'
  | 'N6'
  | 'N7'
  | 'N8'
  | 'N9'
  | 'N10'
  | 'N11'
  | 'N12'
  | 'N13'
  | 'N14'
  | 'N15';
export interface Hit {
  rule: Rule;
  path: string;
  message: string;
}

const REPO_ROOT = join(import.meta.dir, '..');
const SOURCE_ROOTS = ['apps/kira-space/frontend', 'packages'];
const SKIP_DIR = new Set([
  'node_modules',
  'dist',
  'bindings',
  'wire',
  'generated',
  'tests',
  'test',
  '__tests__',
  'db-fixtures',
  '.cache',
]);
const EVENT_FILES = [
  { path: 'packages/shared/protocol/events.ts', literal: /'(kira:[^']*)'/g },
  { path: 'internal/appevent/appevent.go', literal: /"(kira:[^"]*)"/g },
];
const GO_CODES_PATH = 'internal/ipcerr/codes.go';
const TS_CODES_PATH = 'packages/shared/domain/errorCodes.ts';
const GO_IDS_PATH = 'internal/ids/prefixes.go';
const TS_IDS_PATH = 'packages/shared/ids.ts';
const EVENT_NAME = /^kira:[a-z][a-zA-Z]*:[a-z][a-zA-Z]*$/;
const MIGRATION_SETS: ReadonlyArray<{ dir: string; baseline: number }> = [
  { dir: 'apps/kira-space/internal/gitreview/migrations', baseline: 3 },
  { dir: 'apps/kira-space/internal/storage/migrations', baseline: 32 },
  { dir: 'apps/kira-space/internal/studiostorage/migrations', baseline: 37 },
  // 0004 rebuilds memories and memory_events with their legacy TEXT ISO created_at/superseded_at unchanged.
  { dir: 'internal/memory/migrations', baseline: 4 },
];
const MIGRATION_NAME = /^(\d{4})_[a-z]\d+[a-z]?_[a-z0-9]+(?:_[a-z0-9]+)*\.sql$/;
const BAD_FOLDERS = new Set(['util', 'utils', 'helpers']);
const SPEC_ROOTS = [
  'apps/kira-space/tests/ui',
  'apps/kira-space/tests/visual',
  'apps/kira-space/tests/mobile',
];
const SUPPORT_ROOTS = ['apps/kira-space/tests/ui/support', 'packages/workbench/src/testing/ui'];
const MAX_VUE_LINES = 600;
// Engine hosts and shells: one file owns a whole third-party engine or app frame.
const OVERSIZE_VUE_OK = new Set([
  'SlickGridHost.vue',
  'ConsoleSlickGrid.vue',
  'CommitGrid.vue',
  'MonacoHost.vue',
  'CheetahProto.vue', // P165 prototype
  'packages/git-ui/src/App.vue',
]);
// Printed in a menu, bound by the grid engine itself rather than a shortcut call.
const ENGINE_BOUND_SHORTCUTS = new Set(['grid.edit']);
const SHORTCUTS_PATH = 'packages/shared/domain/shortcuts.ts';
const MENU_ROOTS = ['apps/kira-space/frontend/src'];

export function fileNameHit(path: string): Hit | undefined {
  const name = path.slice(path.lastIndexOf('/') + 1);
  if (name.endsWith('.d.ts') || name.endsWith('.config.ts')) return undefined;
  if (name.endsWith('.vue')) {
    return /^[A-Z][a-zA-Z0-9]*\.vue$/.test(name)
      ? undefined
      : { rule: 'N1', path, message: 'component file name is not PascalCase' };
  }
  if (!name.endsWith('.ts')) return undefined;
  // `.worker` (Vite worker entry) and `.check` (type-level check) are role suffixes, not word breaks.
  const base = name.replace(/(?:\.(?:worker|check|spec|test))?\.ts$/, '');
  return /^[a-z][a-zA-Z0-9]*$/.test(base)
    ? undefined
    : { rule: 'N1', path, message: 'module file name is not camelCase' };
}

export function testSuffixHit(path: string): Hit | undefined {
  return path.endsWith('.test.ts')
    ? { rule: 'N2', path, message: 'use the .spec.ts suffix' }
    : undefined;
}

export function folderHits(path: string): Hit[] {
  const dirs = path.split('/').slice(0, -1);
  return BAD_FOLDERS.has(dirs.at(-1) ?? '')
    ? [{ rule: 'N3', path: dirs.join('/'), message: 'use lib/ for helpers' }]
    : [];
}

/** `export const useFooStore = defineStore('foo'` / `createFooStore` factories. */
export function piniaHits(path: string, source: string): Hit[] {
  const hits: Hit[] = [];
  for (const m of source.matchAll(/defineStore\(\s*'([^']*)'/g)) {
    const before = source.slice(0, m.index);
    const exp = [...before.matchAll(/export (?:const|function) (?:use|create)(\w+)/g)].at(-1);
    if (!exp) continue;
    const stem = (exp[1] ?? '').replace(/Store$/, '');
    const want = stem.charAt(0).toLowerCase() + stem.slice(1);
    if (m[1] !== want) {
      hits.push({
        rule: 'N4',
        path,
        message: `Pinia id '${m[1]}' should be '${want}'`,
      });
    }
  }
  return hits;
}

export function eventHits(path: string, source: string, literal: RegExp): Hit[] {
  const names = new Set([...source.matchAll(literal)].map((m) => m[1] ?? ''));
  const bad = [...names].filter((n) => !EVENT_NAME.test(n));
  return bad.length === 0
    ? []
    : [
        {
          rule: 'N5',
          path,
          message: `event names not kira:<scope>:<name> camelCase: ${bad.join(', ')}`,
        },
      ];
}

function codesOf(text: string): Set<string> {
  return new Set([...text.matchAll(/"?'?(E_[A-Z_]+)"?'?/g)].map((m) => m[1] ?? ''));
}

/** Go constants == TS `ERROR_CODES`; no retired-code registry on either side. */
export function codeParityHits(goSource: string, tsSource: string): Hit[] {
  const goActive = new Set(
    [...goSource.matchAll(/\bCode\s*=\s*"(E_[A-Z_]+)"/g)].map((m) => m[1] ?? ''),
  );
  const tsActive = codesOf(
    tsSource.match(/export const ERROR_CODES = \[([\s\S]*?)\] as const/)?.[1] ?? '',
  );
  const onlyGo = [...goActive].filter((c) => !tsActive.has(c)).sort();
  const onlyTs = [...tsActive].filter((c) => !goActive.has(c)).sort();
  const parts = [
    onlyGo.length > 0 ? `only in Go: ${onlyGo.join(', ')}` : '',
    onlyTs.length > 0 ? `only in TS: ${onlyTs.join(', ')}` : '',
  ].filter(Boolean);
  const hits: Hit[] = [];
  if (parts.length > 0) {
    hits.push({
      rule: 'N6',
      path: TS_CODES_PATH,
      message: `error codes differ (${parts.join('; ')})`,
    });
  }
  if (/\bvar Legacy\b/.test(goSource) || /\bLEGACY_ERROR_CODES\b/.test(tsSource)) {
    hits.push({
      rule: 'N6',
      path: TS_CODES_PATH,
      message: 'retired-code registry (Go `Legacy` or TS `LEGACY_ERROR_CODES`); delete it',
    });
  }
  return hits;
}

/** N13: the Go nanoid prefix registry equals the TS `PREFIXES` (same prefixes, same body lengths). */
export function idRegistryHits(goSource: string, tsSource: string): Hit[] {
  const names = new Map(
    [...goSource.matchAll(/^\s*(\w+)\s+Prefix\s*=\s*"([a-z]+)"/gm)].map(
      (m) => [m[1] ?? '', m[2] ?? ''] as const,
    ),
  );
  const go = new Map<string, number>();
  const sizes = goSource.match(/var sizes = map\[Prefix\]int\{([\s\S]*?)\n\}/)?.[1] ?? '';
  for (const m of sizes.matchAll(/(\w+):\s*(\d+)/g)) {
    go.set(names.get(m[1] ?? '') ?? `?${m[1]}`, Number(m[2]));
  }
  const ts = new Map<string, number>();
  const body = tsSource.match(/export const PREFIXES = \{([\s\S]*?)\} as const/)?.[1] ?? '';
  for (const m of body.matchAll(/(\w+):\s*(\d+)/g)) ts.set(m[1] ?? '', Number(m[2]));
  const diff: string[] = [];
  for (const [p, n] of go)
    if (ts.get(p) !== n) diff.push(`${p}: Go ${n}, TS ${ts.get(p) ?? 'missing'}`);
  for (const p of ts.keys()) if (!go.has(p)) diff.push(`${p}: only in TS`);
  if (diff.length === 0) return [];
  return [{ rule: 'N13', path: TS_IDS_PATH, message: `id registry differs (${diff.join('; ')})` }];
}

/** N14: ids come from `@shared/ids`; no raw `randomUUID`, `Math.random().toString(36)`, `nanoid` or `uuid` import elsewhere. */
export function rawIdHits(path: string, source: string): Hit[] {
  if (path === TS_IDS_PATH) return [];
  const hits: Hit[] = [];
  if (/\bcrypto\.randomUUID\s*\(|\brandomUUID\s*\(/.test(source)) {
    hits.push({ rule: 'N14', path, message: 'randomUUID; use newUuid from @shared/ids' });
  }
  if (/from\s+['"](?:nanoid|uuid)(?:\/[^'"]*)?['"]/.test(source)) {
    hits.push({ rule: 'N14', path, message: 'imports nanoid or uuid; use @shared/ids' });
  }
  if (/Math\.random\(\)\.toString\(36\)/.test(source)) {
    hits.push({ rule: 'N14', path, message: 'Math.random().toString(36); use @shared/ids' });
  }
  return hits;
}

export function migrationHits(
  dir: string,
  baseline: number,
  files: ReadonlyMap<string, string>,
): Hit[] {
  const hits: Hit[] = [];
  for (const [name, sql] of files) {
    const num = Number.parseInt(name.slice(0, 4), 10);
    if (!(num > baseline)) continue;
    const path = `${dir}/${name}`;
    if (!MIGRATION_NAME.test(name)) {
      hits.push({
        rule: 'N7',
        path,
        message: 'name must be NNNN_<phase>_<slug>.sql',
      });
    }
    if (/\b\w+_at\s+TEXT\b/i.test(sql)) {
      hits.push({
        rule: 'N7',
        path,
        message: 'TEXT *_at column (store an integer instant)',
      });
    }
  }
  return hits;
}

// Top-level function declarations and arrow-function consts; data constants are not helpers.
const TOP_LEVEL_DECL =
  /^(?:export\s+)?(?:async\s+)?(?:function\s+(\w+)|const\s+(\w+)\s*(?::[^=\n]+)?=\s*(?:async\s*)?(?:\([^)\n]*\)|\w+)\s*(?::[^=\n]+)?=>)/gm;

// One- and two-line locator/alias helpers are cheaper to keep local than to import.
const MIN_HELPER_LINES = 3;

/** Whitespace-normalised text of each top-level helper, keyed by name (copy-paste detection). */
function declaredBodies(source: string): Map<string, string> {
  const lines = source.split('\n');
  const out = new Map<string, string>();
  for (const m of source.matchAll(TOP_LEVEL_DECL)) {
    const name = m[1] ?? m[2] ?? '';
    if (name === '') continue;
    const start = source.slice(0, m.index).split('\n').length - 1;
    let end = start;
    if (!/[;}]\s*$/.test(lines[start] ?? '')) {
      while (
        end < lines.length - 1 &&
        !(/^[})\]]/.test(lines[end + 1] ?? '') && !/\{\s*$/.test(lines[end + 1] ?? ''))
      )
        end++;
      end = Math.min(end + 1, lines.length - 1);
    }
    if (end - start < MIN_HELPER_LINES - 1) continue;
    out.set(
      name,
      lines
        .slice(start, end + 1)
        .join('\n')
        .replace(/^export\s+/, '')
        .replace(/\s+/g, ' '),
    );
  }
  return out;
}

/**
 * N8: a helper copy-pasted into 2+ specs, or copy-pasted from a `support/` export, belongs in
 * `support/`. A same-named helper with a different body is a deliberate specialisation, not a hit.
 */
export function specHelperHits(
  specs: ReadonlyMap<string, string>,
  supports: ReadonlyMap<string, string>,
): Hit[] {
  const hits: Hit[] = [];
  const supportBody = new Map<string, { owner: string; body: string }>();
  for (const [path, source] of supports) {
    for (const [n, body] of declaredBodies(source)) supportBody.set(n, { owner: path, body });
  }
  const copies = new Map<string, string[]>();
  for (const [path, source] of specs) {
    for (const [n, body] of declaredBodies(source)) {
      const shared = supportBody.get(n);
      if (shared && shared.body === body) {
        hits.push({ rule: 'N8', path, message: `'${n}' copies the export of ${shared.owner}` });
      } else {
        const key = `${n}\u0000${body}`;
        copies.set(key, [...(copies.get(key) ?? []), path]);
      }
    }
  }
  for (const [key, paths] of copies) {
    if (paths.length < 2) continue;
    const name = key.slice(0, key.indexOf('\u0000'));
    for (const path of paths) {
      hits.push({
        rule: 'N8',
        path,
        message: `'${name}' is copied in ${paths.length} spec files; move it to support/`,
      });
    }
  }
  return hits;
}

/** N9: `components/` holds `.vue` only; models and helpers live in `lib/`. */
export function componentsTsHit(path: string): Hit | undefined {
  const dirs = path.split('/').slice(0, -1);
  if (!path.endsWith('.ts') || /\.(?:spec|test|d)\.ts$/.test(path)) return undefined;
  if (!/^packages\/[^/]+\/src\//.test(path) || !dirs.includes('components')) return undefined;
  const afterComponents = dirs.slice(dirs.lastIndexOf('components') + 1);
  return afterComponents[0] === 'ui'
    ? undefined
    : { rule: 'N9', path, message: 'components/ holds .vue files only; move to lib/' };
}

/** N10: a `.vue` over 600 lines is split, or is a named engine host / app shell. */
export function oversizeVueHit(path: string, source: string): Hit | undefined {
  if (!path.endsWith('.vue')) return undefined;
  const lines = source.split('\n').length;
  if (lines <= MAX_VUE_LINES) return undefined;
  const name = path.slice(path.lastIndexOf('/') + 1);
  if (OVERSIZE_VUE_OK.has(name) || OVERSIZE_VUE_OK.has(path)) return undefined;
  return { rule: 'N10', path, message: `${lines} lines; split it (limit ${MAX_VUE_LINES})` };
}

/**
 * N11: a menu item's printed shortcut is bound on that surface. `featureSources` is every non-spec
 * `.ts`/`.vue` source under the menu's directory; `scopes` maps shortcut id to scope.
 */
export function menuShortcutHits(
  menuPath: string,
  menuSource: string,
  featureSources: readonly string[],
  scopes: ReadonlyMap<string, string>,
): Hit[] {
  const ids = new Set([...menuSource.matchAll(/\bshortcut:\s*'([^']+)'/g)].map((m) => m[1] ?? ''));
  const binders = featureSources.filter((src) => /\b(?:useShortcut|shortcutFor)\(/.test(src));
  const treeBinders = featureSources.filter((src) => /\buseTreeShortcuts\(/.test(src));
  const treeBound = treeBinders.length > 0;
  const hits: Hit[] = [];
  for (const id of ids) {
    if (scopes.get(id) === 'global' || ENGINE_BOUND_SHORTCUTS.has(id)) continue;
    if (id.startsWith('tree.') && treeBound) continue;
    if (treeBinders.some((src) => src.includes(`'${id}'`))) continue;
    if (binders.some((src) => src.includes(`'${id}'`))) continue;
    hits.push({
      rule: 'N11',
      path: menuPath,
      message: `shortcut '${id}' is printed but no useShortcut/shortcutFor/useTreeShortcuts binds it in this feature dir`,
    });
  }
  return hits;
}

/** N12: a state dir needs 2+ files (one file is `state.ts`); menu builders are `menu.ts`. */
export function stateAndMenuNameHits(paths: readonly string[]): Hit[] {
  const hits: Hit[] = [];
  const stateFiles = new Map<string, number>();
  for (const path of paths) {
    const parts = path.split('/');
    const name = parts.at(-1) ?? '';
    if (!/\.(?:ts|vue)$/.test(name) || /\.(?:spec|test|d)\.ts$/.test(name)) continue;
    if (parts.at(-2) === 'state') {
      const dir = parts.slice(0, -1).join('/');
      stateFiles.set(dir, (stateFiles.get(dir) ?? 0) + 1);
    }
    if (/^(?:menus|menuItems)\.ts$|[a-z]Menu\.ts$/.test(name) && !name.startsWith('use')) {
      hits.push({ rule: 'N12', path, message: 'menu builder file must be named menu.ts' });
    }
  }
  for (const [dir, count] of stateFiles) {
    if (count < 2)
      hits.push({ rule: 'N12', path: dir, message: 'a one-file state dir is state.ts' });
  }
  return hits;
}

function walk(root: string, skip: ReadonlySet<string> = SKIP_DIR): string[] {
  const out: string[] = [];
  const base = join(REPO_ROOT, root);
  if (!existsSync(base)) return out;
  for (const rel of readdirSync(base, { recursive: true, encoding: 'utf8' })) {
    if (rel.split('/').some((seg) => skip.has(seg))) continue;
    out.push(`${root}/${rel}`);
  }
  return out.sort();
}

function readAll(paths: readonly string[]): Map<string, string> {
  return new Map(paths.map((p) => [p, readFileSync(join(REPO_ROOT, p), 'utf8')]));
}

function specHits(): Hit[] {
  const specPaths = SPEC_ROOTS.flatMap((root) =>
    walk(root, new Set(['node_modules'])).filter((p) => p.endsWith('.spec.ts')),
  );
  const supportPaths = SUPPORT_ROOTS.flatMap((root) =>
    walk(root, new Set(['node_modules'])).filter(
      (p) => p.endsWith('.ts') && !p.endsWith('.spec.ts'),
    ),
  );
  return specHelperHits(readAll(specPaths), readAll(supportPaths));
}

function menuHits(allPaths: readonly string[]): Hit[] {
  const scopes = new Map(
    [
      ...readFileSync(join(REPO_ROOT, SHORTCUTS_PATH), 'utf8').matchAll(
        /'([\w.]+)':\s*\{\s*scope:\s*'(\w+)'/g,
      ),
    ].map((m) => [m[1] ?? '', m[2] ?? ''] as const),
  );
  const hits: Hit[] = [];
  for (const menuPath of allPaths.filter((p) => p.endsWith('/menu.ts'))) {
    if (!MENU_ROOTS.some((r) => menuPath.startsWith(`${r}/`))) continue;
    const dir = menuPath.slice(0, menuPath.lastIndexOf('/') + 1);
    const sources = allPaths
      .filter((p) => p.startsWith(dir) && !/\.(?:spec|test)\.ts$/.test(p))
      .map((p) => readFileSync(join(REPO_ROOT, p), 'utf8'));
    hits.push(
      ...menuShortcutHits(
        menuPath,
        readFileSync(join(REPO_ROOT, menuPath), 'utf8'),
        sources,
        scopes,
      ),
    );
  }
  return hits;
}

function fileHits(path: string, dirs: Set<string>): Hit[] {
  const hits: Hit[] = [];
  const read = (): string => readFileSync(join(REPO_ROOT, path), 'utf8');
  const componentsHit = componentsTsHit(path);
  if (componentsHit) hits.push(componentsHit);
  const big = path.endsWith('.vue') ? oversizeVueHit(path, read()) : undefined;
  if (big) hits.push(big);
  const nameHit = fileNameHit(path);
  if (nameHit) hits.push(nameHit);
  const suffixHit = testSuffixHit(path);
  if (suffixHit) hits.push(suffixHit);
  for (const h of folderHits(path)) dirs.add(JSON.stringify(h));
  if (path.endsWith('.ts') && !/\.(?:spec|test)\.ts$/.test(path))
    hits.push(...piniaHits(path, read()));
  if (!/\.(?:spec|test)\.ts$/.test(path)) hits.push(...rawIdHits(path, read()));
  return hits;
}

/** N15: product screenshots are WebP; a tracked PNG under `screenshots/` or `site/` fails. */
function pngHits(tracked: readonly string[]): Hit[] {
  return tracked
    .filter(
      (path) =>
        /^(?:screenshots|site)\//.test(path) &&
        !/^site\/tests\/(?:.*\/)?[^/]*-snapshots\//.test(path),
    )
    .map((path) => ({
      rule: 'N15' as const,
      path,
      message: 'product screenshots are WebP; run bun run screenshots',
    }));
}

function collectHits(): Hit[] {
  const hits: Hit[] = [];
  const dirs = new Set<string>();
  const allPaths: string[] = [];
  for (const root of SOURCE_ROOTS) {
    for (const path of walk(root)) {
      if (!/\.(?:vue|ts)$/.test(path)) continue;
      allPaths.push(path);
      hits.push(...fileHits(path, dirs));
    }
  }
  for (const d of dirs) hits.push(JSON.parse(d) as Hit);
  hits.push(
    ...stateAndMenuNameHits(allPaths.filter((p) => MENU_ROOTS.some((r) => p.startsWith(`${r}/`)))),
  );
  hits.push(...specHits());
  hits.push(...menuHits(allPaths));
  for (const { path, literal } of EVENT_FILES) {
    hits.push(...eventHits(path, readFileSync(join(REPO_ROOT, path), 'utf8'), literal));
  }
  hits.push(
    ...codeParityHits(
      readFileSync(join(REPO_ROOT, GO_CODES_PATH), 'utf8'),
      readFileSync(join(REPO_ROOT, TS_CODES_PATH), 'utf8'),
    ),
  );
  hits.push(
    ...idRegistryHits(
      readFileSync(join(REPO_ROOT, GO_IDS_PATH), 'utf8'),
      readFileSync(join(REPO_ROOT, TS_IDS_PATH), 'utf8'),
    ),
  );
  const listed = Bun.spawnSync(['git', 'ls-files', '-z', '--', '*.png'], { cwd: REPO_ROOT });
  hits.push(...pngHits(new TextDecoder().decode(listed.stdout).split('\0').filter(Boolean)));
  for (const { dir, baseline } of MIGRATION_SETS) {
    const files = new Map<string, string>();
    for (const name of readdirSync(join(REPO_ROOT, dir)).filter((n) => n.endsWith('.sql'))) {
      files.set(name, readFileSync(join(REPO_ROOT, dir, name), 'utf8'));
    }
    hits.push(...migrationHits(dir, baseline, files));
  }
  return hits;
}

function main(): number {
  const hits = collectHits();
  for (const h of hits) console.error(`check-conventions: ${h.path}: ${h.rule} ${h.message}`);
  if (hits.length > 0) {
    console.error(
      `check-conventions: ${hits.length} hit(s); see git show bd0c224688d9:docs/v2.2/plans/P266-consistency-audit.md section 2.10 and P267-ui-leftovers.md section 3`,
    );
    return 1;
  }
  console.log('check-conventions: every convention holds (N1-N15).');
  return 0;
}

if (import.meta.main) process.exit(main());
