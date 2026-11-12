import type { Board, Branch, Pair, Task } from '../wire';

/** Relations between planner branches, derived once per snapshot and shared by the timeline and the
 *  action rules. Mirrors mockup `renderVals()` lines 1504-1522 on the v2 wire. */

interface BranchConflict {
  with: string;
  files: string[];
}

export type GraphInput = Pick<Board, 'tasks' | 'branches' | 'plan' | 'pairs'>;

export interface BranchGraph {
  byBranch: ReadonlyMap<string, Branch>;
  byTask: ReadonlyMap<string, Task>;
  /** Stacking parent: `plan.queuedAfter` first, else a base that is itself a planner branch. */
  parentOf: ReadonlyMap<string, string>;
  kids: ReadonlyMap<string, readonly string[]>;
  conflicts: ReadonlyMap<string, readonly BranchConflict[]>;
  /** Branches with a pair whose conflict check still runs. */
  pairChecking: ReadonlySet<string>;
  ancestors(id: string): string[];
  rootOf(id: string): string;
  /** Files two branches both change, `[]` when the pair is unknown. */
  shared(a: string, b: string): readonly string[];
}

export function isDraft(b: Branch): boolean {
  return b.name === '';
}

function buildConflicts(
  planBranches: readonly Branch[],
  ancestors: (id: string) => string[],
  pairs: ReadonlyMap<string, Pair>,
  pairKey: (a: string, b: string) => string,
  checking: Set<string>,
): Map<string, BranchConflict[]> {
  const conflicts = new Map<string, BranchConflict[]>();
  const push = (id: string, c: BranchConflict): void => {
    const list = conflicts.get(id);
    if (list) list.push(c);
    else conflicts.set(id, [c]);
  };
  for (const b of planBranches) {
    if (b.kind !== 'mine' || b.mergedIntoMain || isDraft(b)) continue;
    const anc = ancestors(b.id);
    for (const r of planBranches) {
      if (r.kind !== 'review' || r.codeRepoId !== b.codeRepoId || anc.includes(r.id)) continue;
      const pair = pairs.get(pairKey(b.id, r.id));
      if (pair?.checking) {
        checking.add(b.id);
        checking.add(r.id);
      }
      const files = pair?.conflicts ?? [];
      if (!files.length) continue;
      push(b.id, { with: r.id, files: [...files] });
      push(r.id, { with: b.id, files: [...files] });
    }
  }
  return conflicts;
}

export function buildBranchGraph(board: GraphInput): BranchGraph {
  const byBranch = new Map<string, Branch>();
  for (const b of board.branches) byBranch.set(b.id, b);
  const byTask = new Map<string, Task>();
  for (const t of board.tasks) byTask.set(t.id, t);

  const parentOf = new Map<string, string>();
  const kids = new Map<string, string[]>();
  const planBranches: Branch[] = [];
  for (const t of board.tasks) {
    for (const id of t.branchIds) {
      const b = byBranch.get(id);
      if (b) planBranches.push(b);
    }
  }
  for (const b of planBranches) kids.set(b.id, []);
  for (const b of planBranches) {
    const queued = board.plan.queuedAfter[b.id];
    const p = queued || (b.baseBranchId && byBranch.has(b.baseBranchId) ? b.baseBranchId : '');
    if (!p) continue;
    parentOf.set(b.id, p);
    kids.get(p)?.push(b.id);
  }

  const ancestors = (id: string): string[] => {
    const out: string[] = [];
    const seen = new Set<string>([id]);
    for (let p = parentOf.get(id); p !== undefined && !seen.has(p); p = parentOf.get(p)) {
      out.push(p);
      seen.add(p);
    }
    return out;
  };
  const rootOf = (id: string): string => ancestors(id).at(-1) ?? id;

  const pairKey = (a: string, b: string): string => `${a}\u0000${b}`;
  const pairs = new Map<string, Pair>();
  for (const p of board.pairs) {
    pairs.set(pairKey(p.a, p.b), p);
    pairs.set(pairKey(p.b, p.a), p);
  }
  const shared = (a: string, b: string): readonly string[] =>
    pairs.get(pairKey(a, b))?.shared ?? [];

  const pairChecking = new Set<string>();
  const conflicts = buildConflicts(planBranches, ancestors, pairs, pairKey, pairChecking);

  return { byBranch, byTask, parentOf, kids, conflicts, pairChecking, ancestors, rootOf, shared };
}

/** `feat/x` -> `x`: the branch name without its first path segment. */
export function shortBranchName(name: string): string {
  return name.replace(/^[^/]+\//, '');
}
