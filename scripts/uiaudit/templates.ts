import type { ElementNode } from '@vue/compiler-core';
import { NodeTypes } from '@vue/compiler-core';
import {
  components,
  nearPairs,
  normaliseClasses,
  qualifiesSubtree,
  stableKey,
  THRESHOLDS,
} from './cluster';
import { loadFiles, type SourceFile } from './files';
import { type ChannelResult, type Cluster, sortClusters } from './report';
import { childElements, directiveName, staticAttr, templateRoot } from './sfc';

export interface TNode {
  file: SourceFile;
  line: number;
  tag: string;
  parent: TNode | null;
  /** Elements in the subtree, itself included. */
  count: number;
  /** Hash of the full normalised subtree. */
  exact: string;
  /** Hash of the tag skeleton alone. */
  skeleton: string;
  /** Static class tokens of the whole subtree, prefixed by relative position. */
  tokens: string[];
  /** Own tag, classes and prop names. */
  own: { classes: string[]; props: string[] };
}

/** Normalised signature of one element: tag, sorted static class tokens, sorted prop names.
 *  Text, interpolations and bound values are dropped; `data-*` names carry test ids only. */
function elementSignature(el: ElementNode): { classes: string[]; props: string[] } {
  const cls = staticAttr(el, 'class')?.value?.content ?? '';
  const props: string[] = [];
  for (const p of el.props) {
    if (p.type === NodeTypes.ATTRIBUTE) {
      if (p.name !== 'class' && !p.name.startsWith('data-')) props.push(p.name);
    } else {
      const name = directiveName(p);
      if (!name.startsWith('bind:data-')) props.push(name);
    }
  }
  return { classes: normaliseClasses(cls), props: [...new Set(props)].sort() };
}

interface Built {
  count: number;
  exact: string;
  skeleton: string;
  tokens: string[];
}

function build(file: SourceFile, el: ElementNode, parent: TNode | null, out: TNode[]): Built {
  const own = elementSignature(el);
  const node: TNode = {
    file,
    line: el.loc.start.line,
    tag: el.tag,
    parent,
    count: 1,
    exact: '',
    skeleton: '',
    tokens: [],
    own,
  };
  const kids = childElements(el).map((c) => build(file, c, node, out));
  const tokens = own.classes.map((t) => `.|${t}`);
  kids.forEach((k, i) => {
    for (const t of k.tokens) tokens.push(`${i}.${t}`);
  });
  const count = 1 + kids.reduce((n, k) => n + k.count, 0);
  const exact = stableKey(
    `${el.tag}|${own.classes.join(' ')}|${own.props.join(',')}(${kids.map((k) => k.exact).join(',')})`,
  );
  const skeleton = stableKey(`${el.tag}(${kids.map((k) => k.skeleton).join(',')})`);
  Object.assign(node, { count, exact, skeleton, tokens: qualifiesSubtree(count) ? tokens : [] });
  out.push(node);
  return { count, exact, skeleton, tokens };
}

let cache: { nodes: TNode[]; roots: Map<string, TNode> } | undefined;

/** Every element node of every template, plus each file's root element node. */
export function templateNodes(): { nodes: TNode[]; roots: Map<string, TNode> } {
  if (cache) return cache;
  const nodes: TNode[] = [];
  const roots = new Map<string, TNode>();
  for (const f of loadFiles()) {
    const root = templateRoot(f);
    if (!root) continue;
    const top = childElements(root)[0];
    if (!top) continue;
    const before = nodes.length;
    build(f, top, null, nodes);
    const rootNode = nodes.slice(before).find((n) => n.parent === null);
    if (rootNode) roots.set(f.path, rootNode);
  }
  cache = { nodes, roots };
  return cache;
}

function toSite(n: TNode) {
  return { file: n.file.path, line: n.line, module: n.file.module };
}

function exactClusters(big: TNode[]): Cluster[] {
  const byHash = new Map<string, TNode[]>();
  for (const n of big) {
    const list = byHash.get(n.exact);
    if (list) list.push(n);
    else byHash.set(n.exact, [n]);
  }
  const repeated = (h: string): TNode[] | undefined => {
    const l = byHash.get(h);
    return l && new Set(l.map((n) => n.file.path)).size >= 2 ? l : undefined;
  };
  const out: Cluster[] = [];
  for (const [hash, list] of byHash) {
    if (!repeated(hash)) continue;
    // Maximal only: a child repeats only because its parent does, unless it repeats more.
    const free = list.filter((n) => {
      const pl = n.parent ? byHash.get(n.parent.exact) : undefined;
      return !(pl && repeated(n.parent?.exact ?? '') && pl.length >= list.length);
    });
    if (new Set(free.map((n) => n.file.path)).size < 2) continue;
    out.push({
      key: hash,
      title: `identical subtree <${free[0].tag}>`,
      size: free[0].count,
      sizeUnit: 'elements',
      sites: free.map(toSite),
    });
  }
  return out;
}

function nearClustersOf(big: TNode[], exact: Cluster[]): Cluster[] {
  const bySkeleton = new Map<string, TNode[]>();
  for (const n of big) {
    const list = bySkeleton.get(n.skeleton);
    if (list) list.push(n);
    else bySkeleton.set(n.skeleton, [n]);
  }
  interface Comp {
    skeleton: string;
    nodes: TNode[];
  }
  const comps: Comp[] = [];
  for (const [skeleton, members] of bySkeleton) {
    if (new Set(members.map((m) => m.file.module)).size < 2) continue;
    const reps = new Map<string, TNode[]>();
    for (const m of members) {
      const l = reps.get(m.exact);
      if (l) l.push(m);
      else reps.set(m.exact, [m]);
    }
    const groups = [...reps.values()].filter((g) => g[0].tokens.length >= 3);
    if (groups.length < 2) continue;
    const items = groups.map((g) => ({ tokens: g[0].tokens }));
    const pairs = nearPairs(items, THRESHOLDS.templateNearJaccard);
    for (const idx of components(groups.length, pairs)) {
      comps.push({ skeleton, nodes: idx.flatMap((i) => groups[i]) });
    }
  }
  const compOf = new Map<TNode, Comp>();
  for (const c of comps) for (const n of c.nodes) compOf.set(n, c);
  const exactKeys = new Set(exact.map((c) => c.key));
  const out: Cluster[] = [];
  for (const c of comps) {
    const free = c.nodes.filter((n) => {
      const pc = n.parent ? compOf.get(n.parent) : undefined;
      return !(pc && pc.nodes.length >= c.nodes.length);
    });
    const mods = new Set(free.map((n) => n.file.module));
    if (mods.size < 2 || new Set(free.map((n) => n.file.path)).size < 2) continue;
    const distinct = [...new Set(free.map((n) => n.exact))].sort();
    if (distinct.length < 2) continue;
    out.push({
      key: stableKey(`near|${c.skeleton}|${distinct.join(',')}`),
      title: `near subtree <${free[0].tag}> (${distinct.length} variants)`,
      size: free[0].count,
      sizeUnit: 'elements',
      sites: free.map(toSite),
      note: distinct.every((h) => exactKeys.has(h))
        ? 'every variant also appears as an exact cluster'
        : undefined,
    });
  }
  return out;
}

export function runTemplates(): ChannelResult {
  const big = templateNodes().nodes.filter((n) => qualifiesSubtree(n.count));
  const exact = exactClusters(big);
  const near = nearClustersOf(big, exact);
  return { id: 'c1', title: 'Template subtrees', clusters: sortClusters([...exact, ...near]) };
}
