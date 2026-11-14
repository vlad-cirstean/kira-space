// A quote/comment-aware SQL statement splitter for the query console: "Run statement"
// and "Run all" both reduce to "send N statements in one execute() call", so both
// need to split on `;` without being fooled by a semicolon inside a string literal, a quoted
// identifier, or a comment. This is a splitter, not a parser — it tracks just enough lexical
// state (which kind of quote/comment it is inside) to find statement boundaries; it never
// validates SQL syntax.
//
// The quote/comment/dollar-quote scanning itself is `sql-lex.ts`'s `scanSqlSpan` —
// hoisted out of here (and `sql-lint.ts`) into one shared scanner both now call. No behaviour
// change: this file keeps its own `;`-splitting/backslash-escape/dollar-quoting semantics exactly.
import { resolveLexOptions, type SqlScanOptions, scanSqlSpan } from './sqlLex';

export interface SqlStatement {
  text: string;
  /** Offsets into the original source, not `text` — `text` is trimmed, these are not. */
  start: number;
  end: number;
}

/** See `sql-lex.ts`'s own `SqlLexOptions` field doc comments (P115 H5: this used to carry its own
 *  copy of them) — kept under this file's historical name for its own callers. */
export type SplitSqlOptions = SqlScanOptions;

const COMPOUND_CREATE =
  /^create\s+(?:or\s+replace\s+)?(?:definer\s*=\s*\S+\s+)?(?:temp(?:orary)?\s+)?(?:trigger|procedure|function|event)\b/i;
// `END IF`/`END LOOP`/`END WHILE`/`END REPEAT` close an opener that is not counted below.
const END_QUALIFIERS = new Set(['if', 'loop', 'while', 'repeat']);

function isWordChar(c: string | undefined): boolean {
  return c !== undefined && /[A-Za-z0-9_$@.]/.test(c);
}

/** The lowercase word starting at `i`, or undefined when `i` is mid-identifier or not a word. */
function wordAt(source: string, i: number): string | undefined {
  if (isWordChar(source[i - 1]) || !/[A-Za-z_]/.test(source[i] ?? '')) return undefined;
  let j = i;
  while (j < source.length && /[A-Za-z0-9_]/.test(source[j] ?? '')) j++;
  if (isWordChar(source[j])) return undefined;
  return source.slice(i, j).toLowerCase();
}

function nextWord(source: string, from: number): string | undefined {
  return /^\s+([A-Za-z]+)/.exec(source.slice(from, from + 24))?.[1]?.toLowerCase();
}

/** Block depth inside `CREATE TRIGGER|PROCEDURE|FUNCTION|EVENT`, where `;` splits only at depth 0. */
function createBlockTracker(source: string) {
  let compound: boolean | undefined;
  let depth = 0;
  return {
    get depth(): number {
      return depth;
    },
    reset(): void {
      compound = undefined;
      depth = 0;
    },
    /** Consumes the word at `i` when inside a compound statement; returns its length, else 0. */
    step(i: number, c: string): number {
      if (compound === undefined && c !== ';' && /\S/.test(c)) {
        compound = COMPOUND_CREATE.test(source.slice(i, i + 200));
      }
      if (!compound) return 0;
      const word = wordAt(source, i);
      if (word === 'begin' || word === 'case') depth++;
      else if (word === 'end' && !END_QUALIFIERS.has(nextWord(source, i + 3) ?? '')) {
        depth = Math.max(0, depth - 1);
      }
      return word?.length ?? 0;
    },
  };
}

export function splitSqlStatements(source: string, options?: SplitSqlOptions): SqlStatement[] {
  const lexOptions = resolveLexOptions(options);
  const statements: SqlStatement[] = [];
  const block = createBlockTracker(source);
  const n = source.length;
  let i = 0;
  let stmtStart = 0;

  const pushIfNonEmpty = (end: number): void => {
    const text = source.slice(stmtStart, end).trim();
    if (text.length > 0) statements.push({ text, start: stmtStart, end });
  };

  while (i < n) {
    const span = scanSqlSpan(source, i, lexOptions);
    if (span) {
      i = span.end;
      continue;
    }
    const c = source[i] ?? '';
    const wordLength = lexOptions.compoundBlocks ? block.step(i, c) : 0;
    if (wordLength > 0) {
      i += wordLength;
      continue;
    }
    if (c === ';' && block.depth === 0) {
      pushIfNonEmpty(i);
      i++;
      stmtStart = i;
      block.reset();
      continue;
    }
    i++;
  }
  pushIfNonEmpty(n);
  return statements;
}

/** Where `s`'s own trimmed text starts within its raw (untrimmed) `[start, end)` span —
 *  `s.start` alone is not that boundary: it sits right after the *previous* statement's `;`, so it
 *  still includes any blank line/whitespace between the two statements. Exported (as
 *  `trimmedStatementStart`) so a caret placed at a *statement's* own position — never `s.start`
 *  itself — lands somewhere `statementAtOffset` unambiguously attributes to that statement, not
 *  its predecessor (ConsoleView.vue's own onFormat: Format's `;\n\n` join
 *  separator is wide enough that `s.start` sits inside the gap `statementAtOffset` gives to the
 *  *previous* statement, where a typed `;\n` separator was narrow enough this never surfaced). */
export function trimmedStatementStart(source: string, s: SqlStatement): number {
  const raw = source.slice(s.start, s.end);
  return s.start + (raw.length - raw.trimStart().length);
}

/** The statement whose range contains `cursor`, over an already-split statement list — the
 *  position right after a statement's own `;`, and any whitespace up to the next
 *  statement's first non-space character (a blank line between two statements, or trailing
 *  whitespace at the end of the document), belongs to the *preceding* statement. That is where the
 *  caret lands after typing `;` or pressing End, and it must never silently resolve to the
 *  following statement instead. `source` must be the exact text `statements` was split from.
 *  Exported so a caller that already has its own cached split (`ConsoleView.vue`'s
 *  `statementAtCursorText`, kept cheap on every caret move by not re-splitting) can reuse this
 *  logic instead of duplicating it. */
export function statementAtOffset(
  statements: SqlStatement[],
  source: string,
  cursor: number,
): SqlStatement | null {
  for (let i = 0; i < statements.length; i++) {
    const s = statements[i];
    if (!s) continue;
    const next = statements[i + 1];
    const ownedEnd = next ? trimmedStatementStart(source, next) - 1 : source.length;
    if (cursor <= ownedEnd) return s;
  }
  return statements[statements.length - 1] ?? null;
}

/** The statement whose source range contains `cursor`, or the last statement past the end. */
export function statementAtCursor(
  source: string,
  cursor: number,
  options?: SplitSqlOptions,
): SqlStatement | null {
  return statementAtOffset(splitSqlStatements(source, options), source, cursor);
}
