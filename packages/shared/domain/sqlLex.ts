// The quote/comment/dollar-quote scanning rules `sql-split.ts` and `sql-lint.ts` each
// carried as their own independent copy (the duplication `sql-lint.ts`'s own header comment
// already named: "Sibling of sql-split.ts: same lexical states … reused here"), hoisted into one
// shared scanner both now call — closing that duplication, and giving `sql-tokens.ts` a third,
// identical caller rather than a third independent copy.
export interface SqlLexOptions {
  /** Whether a backslash escapes the next character inside a '...'/"..."/`...` run — true for
   *  MySQL/MariaDB/ClickHouse, false for standard-SQL dialects (Postgres, SQLite) where a bare
   *  backslash is not special and only a doubled quote escapes. `resolveLexOptions`
   *  defaults this to true for a caller that doesn't know its
   *  dialect. */
  backslashEscapes: boolean;
  /** Whether `$$.../$tag$...$tag$` opens a Postgres-style dollar-quoted string — a Postgres-only
   *  convention. `$` is a legal (if unusual) identifier character on MySQL/MariaDB, so an
   *  identifier containing two of them reads as an unterminated dollar-quote open tag there and
   *  swallows the rest of the document into one statement. `resolveLexOptions` defaults this to
   *  true for a caller that doesn't know its dialect. */
  dollarQuoting: boolean;
  /** P108 Part 11 F4: MySQL, MariaDB and ClickHouse also start a line comment with `#` — undefined/
   *  false leaves `#` an ordinary character (every other dialect here, and the unknown-dialect
   *  default). */
  hashComments?: boolean;
  /** F4: Mongo's JS-shell console syntax starts a line comment with `//` — no SQL dialect here
   *  opts into this (`//` is not comment syntax in any of them, and floor-division-style code could
   *  collide with it). */
  slashSlashComments?: boolean;
  /** F4: Postgres nests its block comments (an inner `/*` bumps the nesting depth) — every other
   *  dialect here does not: the first close always ends the comment regardless of an inner open. */
  nestedBlockComments?: boolean;
  /** F4: SQLite (and SQL Server) accept a `[bracket]`-quoted identifier — a doubled `]]` inside
   *  escapes a literal `]`, mirroring the doubled-quote escape every other quote kind here uses. */
  bracketIdentifiers?: boolean;
  /** Postgres's `E'...'`/`e'...'` escape-string syntax honours backslash escapes inside it
   *  regardless of `backslashEscapes` — Postgres's own default is `false` for an ordinary `'...'`
   *  literal (standard_conforming_strings), but `E'...'` is the one syntax that always opts back
   *  in. Only examined for a single-quoted run whose `E`/`e` is itself a standalone token (not the
   *  tail of a longer identifier). */
  postgresEscapeStrings?: boolean;
  /** SQLite triggers and MySQL/MariaDB routines, triggers and events carry a `BEGIN ... END` body
   *  whose inner `;` do not end the `CREATE` statement; the splitter keeps the block whole. */
  compoundBlocks?: boolean;
  /** MySQL/MariaDB client `DELIMITER <token>` directive: the splitter consumes it and switches its
   *  statement terminator. Server never sees it. */
  delimiterDirective?: boolean;
}

/** The public, all-optional shape `lintSql`/`splitSqlStatements`/`tokenizeSql`'s own callers pass
 *  in — `resolveLexOptions` below fills in the two defaulted fields. */
export type SqlScanOptions = Partial<SqlLexOptions>;

/**
 * P115 H5: `sql-lint.ts` and `sql-split.ts` each rebuilt this identical object from their own
 * optional options param, one default-application each — hoisted once, alongside `scanSqlSpan`
 * itself (P60b §3.1's own header comment above, the first hoist these fields went through).
 */
export function resolveLexOptions(options?: SqlScanOptions): SqlLexOptions {
  return {
    backslashEscapes: options?.backslashEscapes ?? true,
    dollarQuoting: options?.dollarQuoting ?? true,
    hashComments: options?.hashComments,
    slashSlashComments: options?.slashSlashComments,
    nestedBlockComments: options?.nestedBlockComments,
    bracketIdentifiers: options?.bracketIdentifiers,
    postgresEscapeStrings: options?.postgresEscapeStrings,
    compoundBlocks: options?.compoundBlocks,
    delimiterDirective: options?.delimiterDirective,
  };
}

export type SqlLexSpanKind =
  | 'lineComment'
  | 'blockComment'
  | 'quote'
  | 'dollarQuote'
  | 'bracketIdent';

export interface SqlLexSpan {
  kind: SqlLexSpanKind;
  /** Inclusive start (the opening delimiter's own first character). */
  start: number;
  /** Exclusive end — one past the closing delimiter, or the source length when unterminated. */
  end: number;
  /** False only when the span ran to EOF without a closing delimiter — never a thrown error
   *  (R2): the caller decides what, if anything, that means (sql-lint.ts reports it; sql-split.ts
   *  and sql-tokens.ts simply treat `end` as the span's own boundary either way). */
  closed: boolean;
  /** `kind === 'quote'` only — which of `'`/`"`/`` ` `` opened it, so a caller that cares which
   *  quote character was used (sql-tokens.ts's String-vs-QuotedIdentifier classification) doesn't
   *  have to re-read `source[start]` itself. */
  quoteChar?: string;
}

/** P94 pass 3 §4.3: `scanSqlSpan`'s span kinds, one scanner each — the parent only dispatches on
 *  the first character(s), each scanner keeps its own loop and its own EOF/`closed` handling.
 *  `openerLen` is 1 for `#`, 2 for `--`/`//` — the only thing that differs between them. */
function scanLineComment(source: string, i: number, n: number, openerLen: number): SqlLexSpan {
  let j = i + openerLen;
  while (j < n && source[j] !== '\n') j++;
  return { kind: 'lineComment', start: i, end: j, closed: true };
}

// F4: Postgres nests `/* ... */` — an inner `/*` bumps a depth counter rather than being ignored,
// so `/* a /* b */ c */` closes at the *second* `*/`, not the first. Every other dialect here
// passes `nested: false` and keeps the original non-nesting scan.
function scanBlockComment(source: string, i: number, n: number, nested: boolean): SqlLexSpan {
  let depth = 1;
  let j = i + 2;
  while (j < n && depth > 0) {
    if (nested && source[j] === '/' && source[j + 1] === '*') {
      depth++;
      j += 2;
      continue;
    }
    if (source[j] === '*' && source[j + 1] === '/') {
      depth--;
      j += 2;
      continue;
    }
    j++;
  }
  if (depth > 0) return { kind: 'blockComment', start: i, end: n, closed: false };
  return { kind: 'blockComment', start: i, end: j, closed: true };
}

// F4: SQLite/SQL Server `[bracket]` quoted identifier — `]]` inside doubles as an escaped `]`,
// the same doubled-delimiter escape scanQuotedRun's own '/"/` runs use.
function scanBracketIdent(source: string, i: number, n: number): SqlLexSpan {
  let j = i + 1;
  let closed = false;
  while (j < n) {
    if (source[j] === ']') {
      if (source[j + 1] === ']') {
        j += 2;
        continue;
      }
      j++;
      closed = true;
      break;
    }
    j++;
  }
  return { kind: 'bracketIdent', start: i, end: j, closed };
}

// The dollar-quote-open guard and the E'...' guard both need "is this character part of a
// longer identifier token" — Postgres allows `$` inside an unquoted identifier (`a$b$c`), and an
// `E`/`e` immediately preceding a quote only means the escape-string prefix when it is itself a
// standalone token, not the tail of a longer name (`TABLEE'x'` is one identifier, not `TABLE` then
// an escape string).
function isIdentChar(ch: string | undefined): boolean {
  return ch !== undefined && /[A-Za-z0-9_]/.test(ch);
}

// Single/double/back-quoted runs: '' or "" or `` doubles the quote as an escape (every SQL
// dialect here honours that); a backslash escaping the next character too is dialect-specific
// (sql-split.ts's own SplitSqlOptions.backslashEscapes doc comment).
function scanQuotedRun(
  source: string,
  i: number,
  n: number,
  quote: string,
  backslashEscapes: boolean,
): SqlLexSpan {
  let j = i + 1;
  let closed = false;
  while (j < n) {
    if (backslashEscapes && source[j] === '\\') {
      j += 2;
      continue;
    }
    if (source[j] === quote) {
      if (source[j + 1] === quote) {
        j += 2;
        continue;
      }
      j++;
      closed = true;
      break;
    }
    j++;
  }
  // A trailing backslash-escape at the very end of source (j at n-1) steps j to n+1 above — clamp
  // back to n (finding #15, M6): end must never exceed source.length, the same "the source length
  // when unterminated" contract every other arm here already honours.
  return { kind: 'quote', start: i, end: Math.min(j, n), closed, quoteChar: quote };
}

// Postgres dollar-quoting: $$ ... $$ or $tag$ ... $tag$. `null` when `source[i]` isn't the start
// of one (the caller falls through to "no span here").
function scanDollarQuote(source: string, i: number, n: number): SqlLexSpan | null {
  const match = /^\$([A-Za-z_][A-Za-z0-9_]*)?\$/.exec(source.slice(i));
  if (!match) return null;
  const tag = match[0];
  const closeIdx = source.indexOf(tag, i + tag.length);
  if (closeIdx < 0) return { kind: 'dollarQuote', start: i, end: n, closed: false };
  return { kind: 'dollarQuote', start: i, end: closeIdx + tag.length, closed: true };
}

/** Recognises and scans one of the four lexical spans this SQL lexer family agrees on — a line
 *  comment, a block comment, a quoted run (`'...'`/`"..."`/`` `...` ``), or a Postgres-style
 *  dollar-quoted string — starting at `source[i]`. Returns `null` when `i` doesn't start one of
 *  these (the caller handles the character itself: punctuation, an identifier, a paren, …). Never
 *  throws: an unterminated comment/quote/dollar-quote spans to EOF with `closed: false`. */
export function scanSqlSpan(source: string, i: number, options: SqlLexOptions): SqlLexSpan | null {
  const n = source.length;
  const c = source[i];

  if (c === '-' && source[i + 1] === '-') return scanLineComment(source, i, n, 2);
  if (options.hashComments && c === '#') return scanLineComment(source, i, n, 1);
  if (c === '/' && source[i + 1] === '*') {
    return scanBlockComment(source, i, n, options.nestedBlockComments === true);
  }
  if (options.slashSlashComments && c === '/' && source[i + 1] === '/') {
    return scanLineComment(source, i, n, 2);
  }
  if (c === "'" || c === '"' || c === '`') {
    // Postgres's E'...'/e'...' escape string — backslash escapes are honoured inside it
    // regardless of the dialect's own backslashEscapes setting. `source[i - 1]` is the 'E'/'e'
    // itself; `source[i - 2]` must not be an identifier character, or this 'E' is the tail of a
    // longer name, not the escape-string prefix.
    const postgresEscape =
      c === "'" &&
      options.postgresEscapeStrings === true &&
      (source[i - 1] === 'E' || source[i - 1] === 'e') &&
      !isIdentChar(source[i - 2]);
    return scanQuotedRun(source, i, n, c, postgresEscape || options.backslashEscapes);
  }
  if (options.bracketIdentifiers && c === '[') return scanBracketIdent(source, i, n);
  // F4: a dollar-quote can only open where a new token starts — Postgres allows `$` inside an
  // unquoted identifier (`a$b$c`), which otherwise reads as an unterminated dollar-quote open tag
  // and swallows the rest of the document.
  if (c === '$' && options.dollarQuoting && !isIdentChar(source[i - 1])) {
    return scanDollarQuote(source, i, n);
  }
  return null;
}
