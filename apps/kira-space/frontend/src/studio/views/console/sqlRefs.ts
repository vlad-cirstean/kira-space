import type { SqlDialect } from '../shared/sqlIdent';
import {
  childrenOf,
  isKeyword,
  isNameNode,
  type LNode,
  parseSql,
  splitComposite,
  text,
  unquotedName,
} from './sqlNodes';

// A from-scratch re-derivation of lang-sql's own getAliases — a table
// reference and its optional alias, walked from each statement's FROM/JOIN clause over the exact
// same flat token stream the DDL extractor (ddl.ts) reads. Shared by sqlDiagnostics.ts (unknown
// relation / unknown qualified column) and sqlHover.ts (resolving `alias.column` under the
// cursor), so there is exactly one alias walker in the app, not two that can drift apart.
export interface TableRef {
  schema?: string;
  name: string;
  alias?: string;
  /** The exact node this ref's own name token came from — used to exclude it from the deep
   *  CompositeIdentifier scan diagnostics run (a FROM clause's `schema.table` is not a
   *  `table.column` reference). */
  nodeFrom: number;
}

export interface StatementRefs {
  statement: LNode;
  refs: TableRef[];
  /** Names declared by a leading `WITH … AS (…)` clause — exempt from "unknown relation" (D7). */
  cteNames: Set<string>;
}

// F2: the keywords getAliases stops an alias scan at.
const END_FROM = new Set([
  'where',
  'group',
  'having',
  'order',
  'union',
  'intersect',
  'except',
  'all',
  'distinct',
  'limit',
  'offset',
  'fetch',
  'for',
]);

const JOIN_START = new Set(['join', 'left', 'right', 'inner', 'full', 'cross', 'natural']);

// P94 pass 3 §4.3: folds every keyword-set check refsInStatement's own loop made into one lookup.
function classifyKeyword(t: LNode, source: string): 'end' | 'join' | 'on' | 'using' | undefined {
  if (t.name !== 'Keyword') return undefined;
  const w = text(t, source).toLowerCase();
  if (END_FROM.has(w)) return 'end';
  if (JOIN_START.has(w)) return 'join';
  if (w === 'on') return 'on';
  if (w === 'using') return 'using';
  return undefined;
}

// A join's ON condition, from `toks[i]` (the `on` keyword itself) up to but not including
// whatever ends it — another join's own start keyword, or END_FROM. Its own qualified
// identifiers are not table refs, so the caller skips straight over them.
function skipJoinCondition(toks: readonly LNode[], i: number, source: string): number {
  let j = i + 1;
  while (j < toks.length) {
    const cls = classifyKeyword(toks[j] as LNode, source);
    if (cls === 'end' || cls === 'join') break;
    j++;
  }
  return j;
}

function collectCteNames(toks: readonly LNode[], source: string): Set<string> {
  const names = new Set<string>();
  if (!isKeyword(toks[0], 'with', source)) return names;
  let i = 1;
  if (isKeyword(toks[i], 'recursive', source)) i++;
  for (;;) {
    const nameNode = toks[i];
    if (!nameNode || !isNameNode(nameNode)) break;
    names.add(unquotedName(nameNode, source).toLowerCase());
    i++;
    if (toks[i]?.name === 'Parens') i++; // an explicit column list before AS
    if (!isKeyword(toks[i], 'as', source)) break;
    i++;
    if (toks[i]?.name !== 'Parens') break;
    i++;
    if (toks[i]?.name === 'Punctuation' && text(toks[i] as LNode, source) === ',') {
      i++;
      continue;
    }
    break;
  }
  return names;
}

// One table reference at toks[i]: `[schema.]name [[AS] alias]`, or a table-valued function call
// (`name(...)`, no ref emitted) — returns the number of tokens consumed, always >= 1.
function readOneRef(
  toks: readonly LNode[],
  i: number,
  source: string,
): { ref?: TableRef; consumed: number } {
  const nameNode = toks[i];
  if (!nameNode) return { consumed: 1 };
  let schema: string | undefined;
  let name: string;
  if (nameNode.name === 'CompositeIdentifier') {
    const segs = splitComposite(nameNode, source);
    if (segs.length === 0) return { consumed: 1 };
    name = segs[segs.length - 1] as string;
    schema = segs.length > 1 ? segs[segs.length - 2] : undefined;
  } else if (isNameNode(nameNode)) {
    name = unquotedName(nameNode, source);
  } else {
    return { consumed: 1 };
  }

  // A table-valued function call, e.g. `FROM generate_series(1, 10)` — not a relation to resolve.
  if (toks[i + 1]?.name === 'Parens') return { consumed: 2 };

  if (isKeyword(toks[i + 1], 'as', source) && toks[i + 2] && isNameNode(toks[i + 2] as LNode)) {
    const alias = unquotedName(toks[i + 2] as LNode, source);
    return { ref: { schema, name, alias, nodeFrom: nameNode.from }, consumed: 3 };
  }
  const next = toks[i + 1];
  if (next && isNameNode(next)) {
    return {
      ref: { schema, name, alias: unquotedName(next, source), nodeFrom: nameNode.from },
      consumed: 2,
    };
  }
  return { ref: { schema, name, nodeFrom: nameNode.from }, consumed: 1 };
}

function refsInStatement(toks: readonly LNode[], source: string): TableRef[] {
  const refs: TableRef[] = [];
  let i = toks.findIndex((t) => isKeyword(t, 'from', source));
  if (i < 0) return refs;
  i++;
  while (i < toks.length) {
    const t = toks[i] as LNode;
    const cls = classifyKeyword(t, source);
    if (cls === 'end') break;
    if (cls === 'on') {
      i = skipJoinCondition(toks, i, source);
      continue;
    }
    if (cls === 'join' || cls === 'using') {
      i++;
      continue;
    }
    if (t.name === 'Punctuation' && text(t, source) === ',') {
      i++;
      continue;
    }
    if (isNameNode(t) || t.name === 'CompositeIdentifier') {
      const { ref, consumed } = readOneRef(toks, i, source);
      if (ref) refs.push(ref);
      i += consumed;
      continue;
    }
    i++;
  }
  return refs;
}

/** Every top-level Statement in `source`, with its FROM/JOIN table refs and any WITH-declared CTE
 *  names. Subqueries are not walked — this is a best-effort binder, not a real one (D7's own
 *  false-positive rule: under-detect rather than guess). */
// `root` lets a caller that has already parsed `source` (sqlHover.ts's resolveHover) pass the tree
// through instead of paying for a second, redundant parse of the exact same string — measured
// ~5-13ms per hover on a moderately large script, half of it provably redundant. Omitted, this
// parses `source` itself, unchanged from before.
export function statementsWithRefs(
  dialect: SqlDialect,
  source: string,
  root?: LNode,
): StatementRefs[] {
  const parsedRoot = root ?? parseSql(dialect, source);
  const out: StatementRefs[] = [];
  for (const stmt of childrenOf(parsedRoot)) {
    if (stmt.name !== 'Statement') continue;
    const toks = childrenOf(stmt).filter((n) => n.name !== ';');
    out.push({
      statement: stmt,
      refs: refsInStatement(toks, source),
      cteNames: collectCteNames(toks, source),
    });
  }
  return out;
}

/** Every CompositeIdentifier node anywhere under `node` (including inside nested Parens) —
 *  candidate `alias.column`/`table.column` references for D7's unknown-qualified-column rule and
 *  D8's hover resolution. */
export function deepCompositeIdentifiers(node: LNode): LNode[] {
  const out: LNode[] = [];
  const walk = (n: LNode): void => {
    if (n.name === 'CompositeIdentifier') out.push(n);
    for (const child of childrenOf(n)) walk(child);
  };
  for (const child of childrenOf(node)) walk(child);
  return out;
}
