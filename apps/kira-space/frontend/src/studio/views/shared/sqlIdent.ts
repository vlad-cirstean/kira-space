// quoteIdent moves here from grid/menu.ts to gain a second consumer
// (grid/filterCompletion.ts) — one shared definition beats two that can drift on the
// backtick/double-quote split. Same trust boundary grid/menu.ts's own comment already states:
// generated as literal SQL text once, never validated against the column's type.
//
// A quoting-and-grammar *family*, not a product — MariaDB and MySQL share one dialect
// ('mysql', since languages.ts already maps MariaDB to CodeMirror's own `MySQL` lang-sql dialect;
// the family is the concept the code was already reaching for). Before this, the twelve call
// sites across the renderer each hand-wrote
// `record?.kind === 'postgres' || record?.kind === 'mariadb' ? record.kind : undefined`, and any
// kind missing from that inline check silently fell to `undefined` — which quoteIdent below reads
// as "double-quote it", not "this isn't SQL". A MySQL connection that wasn't added to every one
// of those twelve sites would emit invalid double-quoted identifiers into `grid/menu.ts`'s
// generated *Filter by this value* and FK-navigation predicates. sqlDialectFor is the
// one place that decision is made now.
import type { ConnectionKind } from '@shared/domain/connection';
import type { SqlLexOptions } from '@shared/domain/sqlLex';

// SQLite is its own member, not folded into 'mysql' — it double-quotes identifiers
// (already quoteIdent's own default branch below) but is a genuinely different grammar with its
// own CodeMirror lang-sql dialect (languages.ts), so mapping it to 'postgres' would be a lie the
// next reader has to decode.
// ClickHouse is a fourth member for the same reason SQLite is — it backtick-quotes like
// mysql (BACKTICK_DIALECTS below) but is its own grammar, with its own languages.ts dialect.
export type SqlDialect = 'postgres' | 'mysql' | 'sqlite' | 'clickhouse';

/** undefined for a kind with no SQL surface (mongodb, redis, kafka, sqs, s3) or no connection. */
export function sqlDialectFor(kind: ConnectionKind | undefined): SqlDialect | undefined {
  if (kind === 'postgres') return 'postgres';
  if (kind === 'mariadb' || kind === 'mysql') return 'mysql';
  if (kind === 'sqlite') return 'sqlite';
  if (kind === 'clickhouse') return 'clickhouse';
  return undefined;
}

// P36 D29: ClickHouse quotes identifiers with backticks too (its own create_table_query output
// confirms it, F28) — a set rather than a second `=== 'mysql'` check, so a future backtick dialect
// only ever needs one line added here.
const BACKTICK_DIALECTS = new Set<SqlDialect>(['mysql', 'clickhouse']);

export function quoteIdent(dialect: SqlDialect | undefined, name: string): string {
  if (dialect && BACKTICK_DIALECTS.has(dialect)) {
    // ClickHouse's lexer reads a backtick-quoted identifier with the same
    // backslash-escape rules as a string literal (clickhouse/read.go's own quoteIdent) — MySQL
    // does not, so doubling backticks alone (the pre-fix behaviour, still correct for MySQL) left
    // a ClickHouse name containing `\` able to escape its own closing backtick, corrupting the
    // query. Escape `\` first, matching Go, before doubling backticks. A NUL byte has no quoted
    // spelling in either grammar (Go panics on it); stripped here rather than thrown, since this
    // builds a generated SQL string for the user to see, not a request that can fail loudly.
    const safe =
      dialect === 'clickhouse' ? name.split('\u0000').join('').replace(/\\/g, '\\\\') : name;
    return `\`${safe.replace(/`/g, '``')}\``;
  }
  return `"${name.replace(/"/g, '""')}"`;
}

// A backslash inside a standard SQL single-quoted string literal is an ordinary
// character — only MySQL/MariaDB (and ClickHouse) treat it as an escape character by default.
// sql-split.ts/sqlLint.ts's shared quote-scanning loop needs to know which regime it's
// in, or a Postgres/SQLite literal ending in a literal backslash (e.g. `'foo\'`) gets its closing
// quote mistaken for an escaped character and the scan runs on past the literal's real end.
const BACKSLASH_ESCAPE_DIALECTS = new Set<SqlDialect>(['mysql', 'clickhouse']);

/** true when `dialect`'s string literals treat a backslash as an escape character — undefined
 *  (no SQL dialect, e.g. a Mongo/Redis console) keeps the default of `true` since
 *  nothing about that case was in scope for this fix.
 *  Module-private since lexOptionsFor below became the only caller. */
function backslashEscapesFor(dialect: SqlDialect | undefined): boolean {
  return dialect === undefined || BACKSLASH_ESCAPE_DIALECTS.has(dialect);
}

// sql-split.ts/sqlLint.ts's own dollar-quote scanning ($$.../$tag$...$tag$) used
// to run unconditionally, a Postgres-only convention applied to every dialect. A MySQL identifier
// containing two `$` (legal there — `$` is a valid, if unusual, identifier character) reads as a
// dollar-quote open tag with no closing match, and the scanner then swallows the rest of the
// document into one statement — "Run all" silently runs one statement instead of two. Only
// Postgres gets dollar-quoting; undefined (dialect unknown) keeps the pre-fix universal behaviour,
// matching backslashEscapesFor's own "unknown defaults to the more permissive reading" choice.
function dollarQuotingFor(dialect: SqlDialect | undefined): boolean {
  return dialect === undefined || dialect === 'postgres';
}

// The four new sql-lex.ts options, one helper each, same shape as
// backslashEscapesFor/dollarQuotingFor above. Unlike those two, none of these four has a pre-fix
// universal behaviour to preserve — they are new lexical forms, previously unrecognised by every
// dialect — so an unknown dialect (undefined) defaults to false/off for each, not true.
const HASH_COMMENT_DIALECTS = new Set<SqlDialect>(['mysql', 'clickhouse']);

/** MySQL, MariaDB and ClickHouse also start a line comment with `#`. */
export function hashCommentsFor(dialect: SqlDialect | undefined): boolean {
  return dialect !== undefined && HASH_COMMENT_DIALECTS.has(dialect);
}

/** Postgres nests its block comments; no other dialect here does. */
function nestedBlockCommentsFor(dialect: SqlDialect | undefined): boolean {
  return dialect === 'postgres';
}

/** SQLite (and SQL Server) accept a `[bracket]`-quoted identifier. */
function bracketIdentifiersFor(dialect: SqlDialect | undefined): boolean {
  return dialect === 'sqlite';
}

/** Postgres's `E'...'`/`e'...'` escape-string syntax. */
function postgresEscapeStringsFor(dialect: SqlDialect | undefined): boolean {
  return dialect === 'postgres';
}

/** The six lookups above, gathered into the one per-dialect lexical-options literal
 *  every split/lint/format/tokenize call site here used to build by hand. Every field here is a
 *  real, resolved boolean (never `undefined`) — `SqlLexOptions`, not the all-optional
 *  `SqlScanOptions` `resolveLexOptions` fills defaults into. A caller whose own console adds a
 *  dialect-independent option on top (OperationsPanel.vue/format.ts/ConsoleView.vue's own
 *  `slashSlashComments` for a MongoDB console) spreads this, then adds it. */
export function lexOptionsFor(dialect: SqlDialect | undefined): SqlLexOptions {
  return {
    backslashEscapes: backslashEscapesFor(dialect),
    dollarQuoting: dollarQuotingFor(dialect),
    hashComments: hashCommentsFor(dialect),
    nestedBlockComments: nestedBlockCommentsFor(dialect),
    bracketIdentifiers: bracketIdentifiersFor(dialect),
    postgresEscapeStrings: postgresEscapeStringsFor(dialect),
  };
}

// Every generated string literal (Filter by this value, FK navigation, Copy as
// INSERT) used to escape only `'`, never consulting backslashEscapesFor even though that helper
// already exists and is already used for sql-split.ts/sqlLint.ts's own quote scanning. On
// MySQL/MariaDB/ClickHouse a value containing a backslash either silently fails to match (`\n`
// read as a newline) or produces an unterminated literal (a value ending in `\`) — the exact class
// already fixed for *identifiers*; this is the literal half of the same fix.
export function quoteLiteral(dialect: SqlDialect | undefined, value: string): string {
  const escaped = backslashEscapesFor(dialect) ? value.replace(/\\/g, '\\\\') : value;
  return `'${escaped.replace(/'/g, "''")}'`;
}

// A short, curated set of the reserved words most likely to collide with a real column name —
// not exhaustive (a full per-dialect reserved-word list runs to hundreds of entries and shifts
// with engine version); the same "curated, not exhaustive" call the WHERE/ORDER BY
// vocabularies make.
const COMMON_RESERVED = new Set([
  'select',
  'from',
  'where',
  'order',
  'group',
  'by',
  'and',
  'or',
  'not',
  'null',
  'table',
  'index',
  'key',
  'primary',
  'foreign',
  'references',
  'default',
  'check',
  'as',
  'in',
  'is',
  'like',
  'between',
  'exists',
  'union',
  'all',
  'distinct',
  'insert',
  'update',
  'delete',
  'create',
  'drop',
  'alter',
  'grant',
  'revoke',
  'user',
  'column',
  'value',
  'values',
  'limit',
  'offset',
  'join',
  'on',
  'case',
  'when',
  'then',
  'else',
  'end',
  'true',
  'false',
]);

const BARE_SAFE_RE = /^[a-z_][a-z0-9_]*$/;

/** False for a bare-safe, non-reserved lowercase identifier — those are inserted unquoted by a
 *  completion accept. The bare-identifier grammar itself doesn't vary between postgres/mysql
 *  (only the quote character quoteIdent picks does), so `dialect` is accepted for symmetry with
 *  quoteIdent and future divergence, not used today. */
export function identNeedsQuoting(_dialect: SqlDialect | undefined, name: string): boolean {
  return !BARE_SAFE_RE.test(name) || COMMON_RESERVED.has(name.toLowerCase());
}
