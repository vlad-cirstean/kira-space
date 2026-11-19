import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import ts from 'typescript';
import { stableKey, THRESHOLDS } from './cluster';
import { isScanned, loadFiles, REPO_ROOT, type SourceFile } from './files';
import { moduleOf } from './modules';
import { type ChannelResult, type Cluster, type Site, sortClusters } from './report';
import { scriptBlocks, textOf, visit } from './sfc';

interface Clone {
  firstFile: { name: string; start: number };
  secondFile: { name: string; start: number };
  fragment: string;
  tokens: number;
}

/** jscpd names a file relative to the scanned root's parent (`apps/` or `packages/`). */
function resolveName(name: string): string | undefined {
  for (const root of ['apps', 'packages']) {
    const rel = `${root}/${name}`;
    if (existsSync(join(REPO_ROOT, rel))) return rel;
  }
  return undefined;
}

function cloneClusters(): Cluster[] {
  const out = mkdtempSync(join(tmpdir(), 'uiaudit-jscpd-'));
  try {
    const bin = join(REPO_ROOT, 'node_modules/.bin/jscpd');
    const proc = Bun.spawnSync(
      [
        bin,
        '-k',
        String(THRESHOLDS.cloneMinTokens),
        '-f',
        'typescript,vue',
        '-i',
        '**/node_modules/**,**/dist/**',
        '-r',
        'json',
        '-o',
        out,
        'apps',
        'packages',
      ],
      { cwd: REPO_ROOT, stdout: 'ignore', stderr: 'pipe' },
    );
    if (proc.exitCode !== 0) {
      throw new Error(`uiaudit: jscpd failed: ${proc.stderr.toString().slice(0, 300)}`);
    }
    const report = JSON.parse(readFileSync(join(out, 'jscpd-report.json'), 'utf8')) as {
      duplicates: Clone[];
    };
    const byFragment = new Map<string, { tokens: number; sites: Map<string, Site> }>();
    for (const c of report.duplicates) {
      const a = resolveName(c.firstFile.name);
      const b = resolveName(c.secondFile.name);
      if (!a || !b || a === b || !isScanned(a) || !isScanned(b)) continue;
      const key = c.fragment.replace(/\s+/g, ' ').trim();
      const g = byFragment.get(key) ?? { tokens: c.tokens, sites: new Map<string, Site>() };
      for (const [file, line] of [
        [a, c.firstFile.start],
        [b, c.secondFile.start],
      ] as const) {
        g.sites.set(`${file}:${line}`, { file, line, module: moduleOf(file) });
      }
      byFragment.set(key, g);
    }
    return [...byFragment].map(([fragment, g]) => ({
      key: stableKey(`clone|${fragment}`),
      title: `duplicated fragment: \`${fragment.slice(0, 70)}\``,
      size: g.tokens,
      sizeUnit: 'tokens',
      sites: [...g.sites.values()],
    }));
  } finally {
    rmSync(out, { recursive: true, force: true });
  }
}

const STATE_NAME = /^(is)?(loading|busy|pending|saving|submitting|error|err|loadError)$/i;
const REACTIVE = new Set(['watch', 'watchEffect', 'computed']);

interface Shape {
  file: SourceFile;
  line: number;
  bridge: string[];
}

/** One component: state refs, a watch or computed, and calls into the bridge. */
function shapeOf(file: SourceFile): Shape | undefined {
  let line = 0;
  let hasRef = false;
  let hasReactive = false;
  const bridgeIdents = new Set<string>();
  const callees = new Set<string>();
  for (const b of scriptBlocks(file)) {
    for (const st of b.sf.statements) {
      if (
        ts.isImportDeclaration(st) &&
        ts.isStringLiteral(st.moduleSpecifier) &&
        /(^|\/)bridge(\/|$)/.test(st.moduleSpecifier.text) &&
        !/bridge\/(codedError|log|fireAndForget)$/.test(st.moduleSpecifier.text) &&
        st.importClause?.namedBindings &&
        ts.isNamedImports(st.importClause.namedBindings)
      ) {
        for (const e of st.importClause.namedBindings.elements) bridgeIdents.add(e.name.text);
        if (st.importClause.name) bridgeIdents.add(st.importClause.name.text);
      }
    }
    visit(b.sf, (n) => {
      if (ts.isVariableDeclaration(n) && ts.isIdentifier(n.name) && STATE_NAME.test(n.name.text)) {
        if (n.initializer && /^(ref|shallowRef)\b/.test(n.initializer.getText(b.sf))) {
          hasRef = true;
          line = line || b.lineOf(n);
        }
      }
      if (ts.isCallExpression(n)) {
        const callee = textOf(n.expression, b.sf);
        const root = callee.split(/[.(]/)[0];
        if (REACTIVE.has(root) && callee === root) hasReactive = true;
        if (bridgeIdents.has(root)) callees.add(callee);
      }
    });
  }
  if (!hasRef || !hasReactive || callees.size === 0) return undefined;
  return { file, line, bridge: [...callees].sort() };
}

function shapeClusters(files: SourceFile[]): Cluster[] {
  const bySig = new Map<string, Shape[]>();
  for (const f of files) {
    if (f.ext !== 'vue') continue;
    const s = shapeOf(f);
    if (!s) continue;
    const k = s.bridge.join(',');
    const l = bySig.get(k);
    if (l) l.push(s);
    else bySig.set(k, [s]);
  }
  const out: Cluster[] = [];
  for (const [sig, list] of bySig) {
    if (list.length < 2) continue;
    out.push({
      key: stableKey(`shape|${sig}`),
      title: 'repeated state shape: state ref + watch/computed + same bridge calls',
      size: list.length,
      sizeUnit: 'components',
      sites: list.map((s) => ({ file: s.file.path, line: s.line, module: s.file.module })),
      note: `Bridge calls: ${sig}. Candidate composable, Pinia store or TanStack query.`,
    });
  }
  return out;
}

export function runScripts(): ChannelResult {
  const files = loadFiles();
  return {
    id: 'c3',
    title: 'Script duplicates',
    clusters: sortClusters([...cloneClusters(), ...shapeClusters(files)]),
  };
}
