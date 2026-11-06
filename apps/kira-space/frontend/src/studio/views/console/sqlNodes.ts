// The DDL extractor (ddl.ts) and the alias/reference walk diagnostics and hovers
// both share (sqlRefs.ts) walk the same shallow token tree `@shared/domain/sqlTokens.ts` builds.
// The `sql-tokens.ts` produces the identical `LNode` shape from this app's own
// in-house tokenizer instead (docs/HISTORY.md's library survey), so every helper below — and every
// consumer's own `LNode`-typed code — is unchanged.
import { keywordsFor } from '@shared/domain/sqlKeywords';
import { type LNode, tokenizeSql } from '@shared/domain/sqlTokens';
import { lexOptionsFor, type SqlDialect } from '../shared/sqlIdent';

export type { LNode } from '@shared/domain/sqlTokens';

export function childrenOf(node: LNode): LNode[] {
  const out: LNode[] = [];
  let c = node.firstChild;
  while (c) {
    out.push(c);
    c = c.nextSibling;
  }
  return out;
}

export function text(node: LNode, source: string): string {
  return source.slice(node.from, node.to);
}

const NAME_NODE_KINDS = new Set(['Identifier', 'QuotedIdentifier', 'Keyword']);

// A column or table literally named `id`/`name` tokenises as a bare Keyword under
// Postgres's own 763-word keyword list (kw.includes('id')/('name') === true) — lang-sql's own
// sourceContext accepts Identifier/QuotedIdentifier/Keyword alike in a name position, and this
// extractor must too, or it silently drops the single most common column name in a real schema.
export function isNameNode(node: LNode): boolean {
  return NAME_NODE_KINDS.has(node.name);
}

/** Strips one level of identifier quoting: `"x"`, `` `x` `` (doubled quote escapes) or SQLite `[x]`
 *  (`]]` escapes). Anything else returns unchanged. */
export function unquoteIdent(raw: string): string {
  const q = raw[0];
  if ((q === '"' || q === '`') && raw.length >= 2 && raw.endsWith(q)) {
    return raw.slice(1, -1).replaceAll(q + q, q);
  }
  if (q === '[' && raw.length >= 2 && raw.endsWith(']'))
    return raw.slice(1, -1).replaceAll(']]', ']');
  return raw;
}

export function unquotedName(node: LNode, source: string): string {
  const raw = text(node, source);
  return node.name === 'QuotedIdentifier' ? unquoteIdent(raw) : raw;
}

// A CompositeIdentifier's own children alternate name '.' name ['.' name ...] — every
// non-punctuation child, in order, is one segment (F4's tree shape).
export function splitComposite(node: LNode, source: string): string[] {
  return childrenOf(node)
    .filter(isNameNode)
    .map((n) => unquotedName(n, source));
}

export function isKeyword(node: LNode | undefined, word: string, source: string): boolean {
  return !!node && node.name === 'Keyword' && text(node, source).toLowerCase() === word;
}

export function keywordText(node: LNode | undefined, source: string): string | undefined {
  return node?.name === 'Keyword' ? text(node, source).toLowerCase() : undefined;
}

// A tiny lookahead cursor over one statement's flat token list — every DDL statement shape D9
// understands, and every SELECT's FROM/JOIN clause sqlRefs.ts walks, is a linear scan over
// siblings with a handful of optional keywords, not a real recursive-descent grammar.
export class TokenCursor {
  i = 0;
  constructor(
    private readonly toks: readonly LNode[],
    private readonly source: string,
  ) {}

  peek(offset = 0): LNode | undefined {
    return this.toks[this.i + offset];
  }

  atKeyword(word: string, offset = 0): boolean {
    return isKeyword(this.peek(offset), word, this.source);
  }

  eatKeyword(word: string): boolean {
    if (this.atKeyword(word)) {
      this.i++;
      return true;
    }
    return false;
  }

  next(): LNode | undefined {
    return this.toks[this.i++];
  }

  done(): boolean {
    return this.i >= this.toks.length;
  }
}

// (`editor/languages.ts`'s now-removed `dialectObjectFor`'s direct successor): the
// one place a `SqlDialect` id becomes `sql-tokens.ts`'s own `SqlTokenOptions` — every consumer that
// used to call `dialectObjectFor` + `dialect.language.parser.parse(source)` now calls
// `tokenizeSql(source, tokenOptionsFor(dialect))` instead. Which quote character(s) denote an
// identifier rather than a string, per dialect (mirrors `@codemirror/lang-sql`'s own builtin
// dialects' `identifierQuotes`, restated here since that library is gone): Postgres only `"`;
// MySQL/MariaDB only `` ` `` (double quote is a *string* there, `doubleQuotedStrings`); SQLite and
// this app's own ClickHouse dialect accept either.
const IDENTIFIER_QUOTES: Record<SqlDialect, string> = {
  postgres: '"',
  mysql: '`',
  sqlite: '`"',
  clickhouse: '`"',
};

// Memoised per dialect (not recomputed per call) so repeated calls for the same dialect hand back
// the identical `SqlTokenOptions` object reference — `sql-tokens.ts`'s own memo compares its
// `options` argument by reference, and this is what lets a hover/lint/completion call over the
// same untouched document string share one tokenize.
const optionsCache = new Map<SqlDialect, ReturnType<typeof buildTokenOptions>>();

function buildTokenOptions(dialect: SqlDialect) {
  return {
    ...lexOptionsFor(dialect),
    keywords: keywordsFor(dialect),
    identifierQuotes: IDENTIFIER_QUOTES[dialect],
  };
}

export function tokenOptionsFor(dialect: SqlDialect): ReturnType<typeof buildTokenOptions> {
  let cached = optionsCache.get(dialect);
  if (!cached) {
    cached = buildTokenOptions(dialect);
    optionsCache.set(dialect, cached);
  }
  return cached;
}

/** `tokenizeSql(source, tokenOptionsFor(dialect))`, named for what every former
 *  `dialect.language.parser.parse(source).topNode` call site now reads instead. */
export function parseSql(dialect: SqlDialect, source: string): LNode {
  return tokenizeSql(source, tokenOptionsFor(dialect));
}
