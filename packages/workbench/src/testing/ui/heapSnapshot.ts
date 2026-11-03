import { once } from 'node:events';
import { createReadStream, createWriteStream, rmSync } from 'node:fs';
import type { CDPSession } from '@playwright/test';
import { diff, type JSHeapSnapshotDiff, parse } from 'heap-snapshot-toolkit';

// P480: capture a V8 heap snapshot over CDP, parse it with the DevTools model
// (heap-snapshot-toolkit) and reduce it to named-root retained sizes and per-constructor
// aggregates. Report-only; used by memProbe.ts in the attribution pass.

type Snapshot = Awaited<ReturnType<typeof parse>>;

export interface NamedRoot {
  name: string;
  /** Heap object id from `HeapProfiler.getHeapObjectId`. */
  id: number;
}

export interface ConstructorRow {
  name: string;
  count: number;
  selfKb: number;
  /** Largest single instance, retained. */
  maxRetainedKb: number;
  /** Retained bytes summed over instances whose dominator is another constructor (no nesting double count). */
  retainedKb: number;
}

export interface SnapshotReport {
  totalMb: number;
  nodes: number;
  stats: unknown;
  named: { name: string; retainedKb: number; selfKb: number }[];
  byRetained: ConstructorRow[];
  bySelf: ConstructorRow[];
}

const TOP = 30;
const kb = (n: number) => Math.round((n / 1024) * 10) / 10;

/** Streams `HeapProfiler.takeHeapSnapshot` chunks to `file`. */
export async function takeSnapshotToFile(cdp: CDPSession, file: string): Promise<void> {
  const out = createWriteStream(file);
  const onChunk = (e: { chunk: string }) => {
    out.write(e.chunk);
  };
  // biome-ignore lint/suspicious/noExplicitAny: Playwright types CDP events by literal name
  (cdp as any).on('HeapProfiler.addHeapSnapshotChunk', onChunk);
  try {
    await cdp.send('HeapProfiler.takeHeapSnapshot', { reportProgress: false });
  } finally {
    // biome-ignore lint/suspicious/noExplicitAny: see above
    (cdp as any).off('HeapProfiler.addHeapSnapshotChunk', onChunk);
    out.end();
    await once(out, 'finish');
  }
}

export function parseSnapshot(file: string): Promise<Snapshot> {
  return parse(createReadStream(file, 'utf-8'));
}

/** Node index of the heap object with `id`, or -1. One scan over the node table. */
function nodeIndexById(s: Snapshot, id: number): number {
  const step = s.nodeFieldCount;
  for (let i = 0; i < s.nodes.length; i += step) {
    if (s.nodes.getValue(i + s.nodeIdOffset) === id) return i;
  }
  return -1;
}

export function namedRootSizes(s: Snapshot, roots: readonly NamedRoot[]): SnapshotReport['named'] {
  const wanted = new Map(roots.map((r) => [r.id, r.name]));
  const out: SnapshotReport['named'] = [];
  const step = s.nodeFieldCount;
  for (let i = 0; i < s.nodes.length && out.length < wanted.size; i += step) {
    const name = wanted.get(s.nodes.getValue(i + s.nodeIdOffset));
    if (name === undefined) continue;
    const node = s.createNode(i);
    out.push({ name, retainedKb: kb(node.retainedSize()), selfKb: kb(node.selfSize()) });
  }
  return out;
}

export function constructorRows(s: Snapshot): ConstructorRow[] {
  const agg = s.aggregatesWithFilter({
    minNodeId: undefined,
    maxNodeId: undefined,
    allocationNodeId: undefined,
  } as never);
  const step = s.nodeFieldCount;
  const rows: ConstructorRow[] = [];
  for (const a of Object.values(agg)) {
    let retained = 0;
    for (const idx of a.idxs) {
      const ord = idx / step;
      const dom = s.dominatorsTree[ord] * step;
      if (s.nodes.getValue(dom + s.nodeNameOffset) !== s.nodes.getValue(idx + s.nodeNameOffset)) {
        retained += s.retainedSizes[ord];
      }
    }
    rows.push({
      name: a.name,
      count: a.count,
      selfKb: kb(a.self),
      maxRetainedKb: kb(a.maxRet),
      retainedKb: kb(retained),
    });
  }
  return rows;
}

export function analyse(s: Snapshot, named: readonly NamedRoot[]): SnapshotReport {
  const rows = constructorRows(s);
  return {
    totalMb: Math.round((s.totalSize / (1024 * 1024)) * 100) / 100,
    nodes: s.nodeCount,
    stats: s.getStatistics?.(),
    named: namedRootSizes(s, named),
    byRetained: [...rows].sort((a, b) => b.retainedKb - a.retainedKb).slice(0, TOP),
    bySelf: [...rows].sort((a, b) => b.selfKb - a.selfKb).slice(0, TOP),
  };
}

export interface CaptureOptions {
  named: readonly NamedRoot[];
  /** Keep the `.heapsnapshot` file (for `diffSnapshotFiles`). */
  keep: boolean;
}

export async function captureSnapshot(
  cdp: CDPSession,
  file: string,
  opts: CaptureOptions,
): Promise<SnapshotReport> {
  await takeSnapshotToFile(cdp, file);
  try {
    return analyse(await parseSnapshot(file), opts.named);
  } finally {
    if (!opts.keep) rmSync(file, { force: true });
  }
}

export interface DiffRow {
  name: string;
  countDelta: number;
  sizeDeltaKb: number;
}

/** Walks one retainer chain (nearest to the GC root first) from a node, as `name <-edge- name`. */
export function retainerPath(s: Snapshot, nodeIndex: number, maxHops = 14): string[] {
  const path: string[] = [];
  let idx = nodeIndex;
  const seen = new Set<number>();
  for (let hop = 0; hop < maxHops && !seen.has(idx); hop++) {
    seen.add(idx);
    const node = s.createNode(idx);
    path.push(`${node.type()}:${node.name().slice(0, 60)}`);
    let best: { idx: number; edge: string; dist: number } | undefined;
    for (const it = node.retainers(); it.hasNext(); it.next()) {
      const e = it.item();
      const from = e.node();
      const edgeType = e.type();
      if (edgeType === 'weak') continue;
      const dist = s.nodeDistances[from.ordinal()];
      if (best === undefined || dist < best.dist) {
        best = { idx: from.nodeIndex, edge: `${edgeType}:${String(e.name())}`, dist };
      }
    }
    if (!best || s.createNode(best.idx).isRoot()) break;
    path.push(`  <-${best.edge}-`);
    idx = best.idx;
  }
  return path;
}

/** Constructor growth between two kept snapshots, plus the retainer path of the top grower. */
export async function diffSnapshotFiles(
  fileA: string,
  fileB: string,
): Promise<{ rows: DiffRow[]; topPath: string[] }> {
  const a = await parseSnapshot(fileA);
  const b = await parseSnapshot(fileB);
  const d: JSHeapSnapshotDiff = await diff(a, b);
  const entries = Object.entries(d) as [
    string,
    { name: string; countDelta: number; sizeDelta: number; addedIndexes: number[] },
  ][];
  entries.sort((x, y) => y[1].sizeDelta - x[1].sizeDelta);
  const rows = entries
    .slice(0, 20)
    .map(([, v]) => ({ name: v.name, countDelta: v.countDelta, sizeDeltaKb: kb(v.sizeDelta) }));
  const top = entries.find(([, v]) => v.addedIndexes.length > 0);
  const topPath = top ? retainerPath(b, top[1].addedIndexes[0]) : [];
  return { rows, topPath };
}
