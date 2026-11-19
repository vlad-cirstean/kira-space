import ts from 'typescript';
import { jaccard, stableKey, THRESHOLDS } from './cluster';
import { loadFiles, type SourceFile } from './files';
import { type ChannelResult, type Cluster, type Site, sortClusters } from './report';
import { scriptBlocks, visit } from './sfc';
import { type TNode, templateNodes } from './templates';

const SHARED_HOMES = [
  /^packages\/theme\/src\/components\/[^/]+\.vue$/,
  /^packages\/workbench\/src\/components\/[^/]+\.vue$/,
];

function isSharedHome(path: string): boolean {
  return SHARED_HOMES.some((r) => r.test(path));
}

interface ComponentApi {
  props: string[];
  emits: string[];
}

function memberNames(members: ts.NodeArray<ts.TypeElement>): string[] {
  const out: string[] = [];
  for (const m of members) {
    if ((ts.isPropertySignature(m) || ts.isMethodSignature(m)) && m.name) {
      out.push(m.name.getText().replace(/['"]/g, ''));
    }
  }
  return out;
}

function emitNames(members: ts.NodeArray<ts.TypeElement>): string[] {
  const out: string[] = [];
  for (const m of members) {
    if (ts.isCallSignatureDeclaration(m)) {
      const first = m.parameters[0]?.type;
      if (first && ts.isLiteralTypeNode(first) && ts.isStringLiteral(first.literal)) {
        out.push(first.literal.text);
      }
    } else if (ts.isPropertySignature(m) && m.name) {
      out.push(m.name.getText().replace(/['"]/g, ''));
    }
  }
  return out;
}

function literalMembers(
  members: ts.NodeArray<ts.TypeElement> | undefined,
  emits: boolean,
): string[] {
  if (!members) return [];
  return emits ? emitNames(members) : memberNames(members);
}

function runtimeNames(arg: ts.Expression | undefined): string[] {
  if (arg && ts.isArrayLiteralExpression(arg)) {
    return arg.elements.filter(ts.isStringLiteral).map((e) => e.text);
  }
  if (arg && ts.isObjectLiteralExpression(arg)) {
    return arg.properties.flatMap((p) => (p.name ? [p.name.getText().replace(/['"]/g, '')] : []));
  }
  return [];
}

/** Interface and type-literal member lists of one script block, by name. */
function declaredShapes(sf: ts.SourceFile): Map<string, ts.NodeArray<ts.TypeElement>> {
  const decls = new Map<string, ts.NodeArray<ts.TypeElement>>();
  visit(sf, (n) => {
    if (ts.isInterfaceDeclaration(n)) decls.set(n.name.text, n.members);
    if (ts.isTypeAliasDeclaration(n) && ts.isTypeLiteralNode(n.type)) {
      decls.set(n.name.text, n.type.members);
    }
  });
  return decls;
}

/** Prop and emit names a component declares through `defineProps` and `defineEmits`. */
function componentApi(file: SourceFile): ComponentApi {
  const props = new Set<string>();
  const emits = new Set<string>();
  for (const b of scriptBlocks(file)) {
    const decls = declaredShapes(b.sf);
    visit(b.sf, (n) => {
      if (!ts.isCallExpression(n) || !ts.isIdentifier(n.expression)) return;
      const name = n.expression.text;
      if (name !== 'defineProps' && name !== 'defineEmits') return;
      const type = n.typeArguments?.[0];
      let members: ts.NodeArray<ts.TypeElement> | undefined;
      if (type && ts.isTypeLiteralNode(type)) members = type.members;
      else if (type && ts.isTypeReferenceNode(type)) members = decls.get(type.typeName.getText());
      const names = type
        ? literalMembers(members, name === 'defineEmits')
        : runtimeNames(n.arguments[0]);
      for (const x of names) (name === 'defineProps' ? props : emits).add(x);
    });
  }
  return { props: [...props].sort(), emits: [...emits].sort() };
}

function baseName(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1).replace(/\.(vue|ts)$/, '');
}

/** Whole-subtree match when both roots hold enough elements, root-only otherwise. */
function treeMatch(a: TNode, b: TNode): number | undefined {
  const big = a.count >= THRESHOLDS.subtreeMinElements && b.count >= THRESHOLDS.subtreeMinElements;
  if (big) {
    if (a.skeleton !== b.skeleton) return undefined;
    const j = jaccard(a.tokens, b.tokens);
    return j >= THRESHOLDS.templateNearJaccard ? j : undefined;
  }
  if (a.tag !== b.tag || a.own.classes.length < 4 || b.own.classes.length < 4) return undefined;
  const j = jaccard(a.own.classes, b.own.classes);
  return j >= THRESHOLDS.templateNearJaccard ? j : undefined;
}

function propsOverlap(a: string[], b: string[]): { shared: string[]; ratio: number } | undefined {
  if (a.length < 3 || b.length < 3) return undefined;
  const sb = new Set(b);
  const shared = a.filter((p) => sb.has(p));
  const ratio = shared.length / Math.min(a.length, b.length);
  return shared.length >= 2 && ratio >= THRESHOLDS.propsOverlap ? { shared, ratio } : undefined;
}

function nearMissClusters(files: SourceFile[]): Cluster[] {
  const { roots } = templateNodes();
  const shared = files.filter((f) => f.ext === 'vue' && isSharedHome(f.path));
  const modules = files.filter((f) => f.ext === 'vue' && !isSharedHome(f.path));
  const apis = new Map<string, ComponentApi>();
  for (const f of [...shared, ...modules]) apis.set(f.path, componentApi(f));
  const out: Cluster[] = [];
  for (const s of shared) {
    const name = baseName(s.path);
    const sRoot = roots.get(s.path);
    const sApi = apis.get(s.path) ?? { props: [], emits: [] };
    const sites: Site[] = [];
    for (const m of modules) {
      if (new RegExp(`\\b${name}\\b`).test(m.source)) continue;
      const reasons: string[] = [];
      const mRoot = roots.get(m.path);
      const tj = sRoot && mRoot ? treeMatch(sRoot, mRoot) : undefined;
      if (tj !== undefined) reasons.push(`template ${tj.toFixed(2)}`);
      const po = propsOverlap(sApi.props, apis.get(m.path)?.props ?? []);
      if (po) reasons.push(`props ${po.shared.join(',')}`);
      if (reasons.length > 0) {
        sites.push({
          file: m.path,
          line: mRoot?.line ?? 1,
          module: m.module,
          note: reasons.join('; '),
        });
      }
    }
    if (sites.length === 0) continue;
    out.push({
      key: stableKey(`nearmiss|${s.path}`),
      title: `near-miss of shared \`${name}\``,
      size: sites.length,
      sizeUnit: 'components',
      sites,
      note: `Shared piece ${s.path}; props ${sApi.props.join(', ') || 'none'}; emits ${sApi.emits.join(', ') || 'none'}.`,
    });
  }
  return out;
}

const COMPOSABLE = /^use[A-Z]/;

function collisionClusters(files: SourceFile[]): Cluster[] {
  const byName = new Map<string, SourceFile[]>();
  for (const f of files) {
    const name = baseName(f.path);
    if (name === 'index' || !(f.ext === 'vue' || COMPOSABLE.test(name))) continue;
    const key = `${f.ext}|${name}`;
    const l = byName.get(key);
    if (l) l.push(f);
    else byName.set(key, [f]);
  }
  const out: Cluster[] = [];
  for (const [key, list] of byName) {
    const dirs = new Set(list.map((f) => f.path.slice(0, f.path.lastIndexOf('/'))));
    if (dirs.size < 2) continue;
    out.push({
      key: stableKey(`collision|${key}`),
      title: `basename collision \`${key.split('|')[1]}\``,
      size: list.length,
      sizeUnit: 'files',
      sites: list.map((f) => ({ file: f.path, line: 1, module: f.module })),
    });
  }
  return out;
}

export function runNearMiss(): ChannelResult {
  const files = loadFiles();
  return {
    id: 'c4',
    title: 'Shared-piece near-miss',
    clusters: sortClusters([...nearMissClusters(files), ...collisionClusters(files)]),
  };
}
