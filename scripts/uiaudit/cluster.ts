import { createHash } from 'node:crypto';
import { cn } from '../../packages/theme/src/lib/utils';

/** Thresholds from the P674 plan; the one place they live. */
export const THRESHOLDS = {
  subtreeMinElements: 4,
  templateNearJaccard: 0.7,
  classExactUses: 3,
  classNearJaccard: 0.8,
  classNearMinTokens: 4,
  propsOverlap: 0.6,
  cloneMinTokens: 40,
} as const;

const foldCache = new Map<string, string[]>();

/** Class tokens as the app renders them: `cn` conflict folding, deduped, sorted. */
export function normaliseClasses(raw: string): string[] {
  const hit = foldCache.get(raw);
  if (hit) return hit;
  const merged = raw.trim() ? cn(raw) : '';
  const tokens = [...new Set(merged.split(/\s+/).filter(Boolean))].sort();
  foldCache.set(raw, tokens);
  return tokens;
}

export function jaccard(a: readonly string[], b: readonly string[]): number {
  if (a.length === 0 && b.length === 0) return 1;
  const sb = new Set(b);
  let inter = 0;
  for (const t of new Set(a)) if (sb.has(t)) inter++;
  const union = new Set(a).size + sb.size - inter;
  return union === 0 ? 1 : inter / union;
}

/** First 10 hex of the sha1 of an already normalised signature. */
export function stableKey(signature: string): string {
  return createHash('sha1').update(signature).digest('hex').slice(0, 10);
}

/** All index pairs with Jaccard >= threshold. Prefix filtering over rarest-first tokens, so the
 *  cost stays far below n^2 on thousands of sets. Items must hold deduped tokens. */
export function nearPairs(
  items: readonly { tokens: readonly string[] }[],
  threshold: number,
): [number, number][] {
  const freq = new Map<string, number>();
  for (const it of items) for (const t of it.tokens) freq.set(t, (freq.get(t) ?? 0) + 1);
  const order = items
    .map((it, i) => ({
      i,
      tokens: [...it.tokens].sort(
        (a, b) => (freq.get(a) ?? 0) - (freq.get(b) ?? 0) || (a < b ? -1 : 1),
      ),
    }))
    .filter((e) => e.tokens.length > 0)
    .sort((a, b) => a.tokens.length - b.tokens.length);
  const index = new Map<string, number[]>();
  const pairs: [number, number][] = [];
  for (const e of order) {
    const n = e.tokens.length;
    const prefix = n - Math.ceil(threshold * n - 1e-9) + 1;
    const seen = new Set<number>();
    for (let p = 0; p < prefix; p++) {
      for (const j of index.get(e.tokens[p]) ?? []) {
        if (seen.has(j)) continue;
        seen.add(j);
        if (items[j].tokens.length < threshold * n - 1e-9) continue;
        if (jaccard(items[j].tokens, items[e.i].tokens) >= threshold - 1e-9) {
          pairs.push([j, e.i]);
        }
      }
    }
    for (let p = 0; p < prefix; p++) {
      const list = index.get(e.tokens[p]);
      if (list) list.push(e.i);
      else index.set(e.tokens[p], [e.i]);
    }
  }
  return pairs;
}

/** Connected components of the pair graph, singletons dropped. */
export function components(size: number, pairs: readonly [number, number][]): number[][] {
  const parent = Array.from({ length: size }, (_, i) => i);
  const find = (x: number): number => {
    let r = x;
    while (parent[r] !== r) r = parent[r];
    let c = x;
    while (parent[c] !== r) {
      const n = parent[c];
      parent[c] = r;
      c = n;
    }
    return r;
  };
  for (const [a, b] of pairs) parent[find(a)] = find(b);
  const groups = new Map<number, number[]>();
  for (let i = 0; i < size; i++) {
    const r = find(i);
    const g = groups.get(r);
    if (g) g.push(i);
    else groups.set(r, [i]);
  }
  return [...groups.values()].filter((g) => g.length > 1);
}

/** Near clusters of token sets: components of the pair graph at `threshold`, sets below
 *  `minTokens` ignored. Returns indices into `items`. */
export function nearClusters(
  items: readonly { tokens: readonly string[] }[],
  threshold: number,
  minTokens = 1,
): number[][] {
  const kept = items.map((it, i) => ({ it, i })).filter((e) => e.it.tokens.length >= minTokens);
  const pairs = nearPairs(
    kept.map((e) => e.it),
    threshold,
  );
  return components(
    kept.length,
    pairs.map(([a, b]) => [a, b] as [number, number]),
  ).map((g) => g.map((k) => kept[k].i));
}

/** A subtree is hashed only from the element-count floor up. */
export function qualifiesSubtree(elementCount: number): boolean {
  return elementCount >= THRESHOLDS.subtreeMinElements;
}
