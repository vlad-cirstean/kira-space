import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import ts from 'typescript';
import { rowVariants } from '../../packages/theme/src/lib/rowVariants';
import { nearClusters, normaliseClasses, stableKey, THRESHOLDS } from './cluster';
import { loadFiles, REPO_ROOT, type SourceFile } from './files';
import { type ChannelResult, type Cluster, type Site, sortClusters } from './report';
import {
  parseExpression,
  scriptBlocks,
  staticAttr,
  templateExpressions,
  visit,
  walkElements,
} from './sfc';

interface Occurrence {
  tokens: string[];
  site: Site;
  file: SourceFile;
}

const CLASS_CALLS = new Set(['cn', 'cva', 'clsx', 'cx', 'twMerge']);

function literalValue(n: ts.Node): string | undefined {
  if (ts.isStringLiteral(n) || ts.isNoSubstitutionTemplateLiteral(n)) return n.text;
  return undefined;
}

/** String literals under a node: values, plus object keys when `keys` is set. */
function literalsUnder(root: ts.Node, keys: boolean): { text: string; node: ts.Node }[] {
  const out: { text: string; node: ts.Node }[] = [];
  visit(root, (n) => {
    const v = literalValue(n);
    if (v !== undefined) out.push({ text: v, node: n });
    if (keys && ts.isPropertyAssignment(n) && ts.isIdentifier(n.name)) {
      out.push({ text: n.name.text, node: n.name });
    }
  });
  return out;
}

function calleeName(call: ts.CallExpression): string {
  return ts.isIdentifier(call.expression) ? call.expression.text : '';
}

function collectFromCode(
  file: SourceFile,
  sf: ts.SourceFile,
  lineOf: (n: ts.Node) => number,
  occ: Occurrence[],
  classBinding: boolean,
): void {
  const push = (text: string, node: ts.Node): void => {
    const tokens = normaliseClasses(text);
    if (tokens.length > 0) {
      occ.push({
        tokens,
        file,
        site: { file: file.path, line: lineOf(node), module: file.module },
      });
    }
  };
  if (classBinding) {
    for (const l of literalsUnder(sf, true)) push(l.text, l.node);
    return;
  }
  const walk = (n: ts.Node): void => {
    if (ts.isCallExpression(n) && CLASS_CALLS.has(calleeName(n))) {
      const name = calleeName(n);
      for (const arg of n.arguments) {
        for (const l of literalsUnder(arg, name !== 'cva')) push(l.text, l.node);
      }
      return;
    }
    if (
      ts.isVariableDeclaration(n) &&
      ts.isIdentifier(n.name) &&
      /(_CLASS|_CLASSES)$/.test(n.name.text) &&
      n.initializer
    ) {
      for (const l of literalsUnder(n.initializer, false)) push(l.text, l.node);
      return;
    }
    ts.forEachChild(n, walk);
  };
  walk(sf);
}

function collect(files: SourceFile[]): Occurrence[] {
  const occ: Occurrence[] = [];
  for (const f of files) {
    for (const b of scriptBlocks(f)) collectFromCode(f, b.sf, b.lineOf, occ, false);
    if (f.ext !== 'vue') continue;
    walkElements(f, (el) => {
      const cls = staticAttr(el, 'class');
      if (cls?.value) {
        const tokens = normaliseClasses(cls.value.content);
        if (tokens.length > 0) {
          occ.push({
            tokens,
            file: f,
            site: { file: f.path, line: cls.loc.start.line, module: f.module },
          });
        }
      }
    });
    for (const e of templateExpressions(f)) {
      if (e.directive !== 'bind:class') continue;
      const p = parseExpression(e);
      collectFromCode(f, p.sf, p.lineOf, occ, true);
    }
  }
  return occ;
}

interface SharedConstant {
  name: string;
  tokens: string[];
  definedIn: string;
  /** Identifier a consumer file must mention to count as importing it. */
  ident: string;
}

/** Shared class constants: exported strings of theme `lib/*.ts` (frames, rowVariants) and the
 *  `rowVariants` recipe outputs. */
function sharedConstants(): SharedConstant[] {
  const out: SharedConstant[] = [];
  const libDir = join(REPO_ROOT, 'packages/theme/src/lib');
  for (const name of readdirSync(libDir).filter(
    (n) => n.endsWith('.ts') && !n.endsWith('.spec.ts'),
  )) {
    const src = readFileSync(join(libDir, name), 'utf8');
    const re = /export const (\w+)\s*(?::[^=]+)?=\s*(['"`])([^'"`]+)\2/g;
    for (let m = re.exec(src); m; m = re.exec(src)) {
      out.push({
        name: m[1],
        tokens: normaliseClasses(m[3]),
        definedIn: `packages/theme/src/lib/${name}`,
        ident: m[1],
      });
    }
  }
  for (const layout of ['menu', 'tree', 'double', 'nav'] as const) {
    for (const selected of [false, true]) {
      out.push({
        name: `rowVariants(layout:${layout},selected:${selected})`,
        tokens: normaliseClasses(rowVariants({ layout, selected })),
        definedIn: 'packages/theme/src/lib/rowVariants.ts',
        ident: 'rowVariants',
      });
    }
  }
  return out.filter((c) => c.tokens.length >= 3);
}

function exactClusters(occ: Occurrence[]): Cluster[] {
  const bySet = new Map<string, Occurrence[]>();
  for (const o of occ) {
    if (o.tokens.length < 2) continue;
    const k = o.tokens.join(' ');
    const l = bySet.get(k);
    if (l) l.push(o);
    else bySet.set(k, [o]);
  }
  const out: Cluster[] = [];
  for (const [set, list] of bySet) {
    if (list.length < THRESHOLDS.classExactUses) continue;
    if (new Set(list.map((o) => o.file.path)).size < 2) continue;
    out.push({
      key: stableKey(`set|${set}`),
      title: `exact class set \`${set}\``,
      size: list[0].tokens.length,
      sizeUnit: 'tokens',
      sites: list.map((o) => o.site),
    });
  }
  return out;
}

function nearClusterList(occ: Occurrence[]): Cluster[] {
  const bySet = new Map<string, Occurrence[]>();
  for (const o of occ) {
    if (o.tokens.length < THRESHOLDS.classNearMinTokens) continue;
    const k = o.tokens.join(' ');
    const l = bySet.get(k);
    if (l) l.push(o);
    else bySet.set(k, [o]);
  }
  const groups = [...bySet.entries()];
  const idx = nearClusters(
    groups.map(([, l]) => ({ tokens: l[0].tokens })),
    THRESHOLDS.classNearJaccard,
    THRESHOLDS.classNearMinTokens,
  );
  const out: Cluster[] = [];
  for (const g of idx) {
    const members = g.flatMap((i) => groups[i][1]);
    if (new Set(members.map((o) => o.file.module)).size < 2) continue;
    const sets = g.map((i) => groups[i][0]).sort();
    out.push({
      key: stableKey(`near|${sets.join('|')}`),
      title: `near class sets (${sets.length} variants)`,
      size: members[0].tokens.length,
      sizeUnit: 'tokens',
      sites: members.map((o) => ({ ...o.site, note: `\`${o.tokens.join(' ')}\`` })),
    });
  }
  return out;
}

function sharedConstantClusters(occ: Occurrence[], consts: SharedConstant[]): Cluster[] {
  const out: Cluster[] = [];
  for (const c of consts) {
    const sites: Site[] = [];
    for (const o of occ) {
      if (o.file.path === c.definedIn) continue;
      if (o.tokens.length < c.tokens.length) continue;
      const have = new Set(o.tokens);
      if (!c.tokens.every((t) => have.has(t))) continue;
      if (new RegExp(`\\b${c.ident}\\b`).test(o.file.source)) continue;
      sites.push({ ...o.site, note: o.tokens.length === c.tokens.length ? 'equal' : 'contains' });
    }
    if (sites.length === 0) continue;
    out.push({
      key: stableKey(`const|${c.name}`),
      title: `shared constant \`${c.name}\` re-spelled without import`,
      size: c.tokens.length,
      sizeUnit: 'tokens',
      sites,
      note: `Defined in ${c.definedIn}: \`${c.tokens.join(' ')}\``,
    });
  }
  return out;
}

export function runClasses(): ChannelResult {
  const files = loadFiles();
  const occ = collect(files);
  const clusters = [
    ...exactClusters(occ),
    ...nearClusterList(occ),
    ...sharedConstantClusters(occ, sharedConstants()),
  ];
  return { id: 'c2', title: 'Class strings', clusters: sortClusters(clusters) };
}
