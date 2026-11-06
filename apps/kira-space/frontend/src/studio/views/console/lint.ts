import type { ConnectionKind } from '@shared/domain/connection';
import { MONGO_CONSOLE_METHODS } from '@shared/domain/console';
import { lintSql } from '@shared/domain/sqlLint';
import { splitSqlStatements } from '@shared/domain/sqlSplit';
import { t } from '@theme/i18n';
import type { ConsoleDiagnostic } from '../../editor/diagnostics';
import { tryParseShellText } from '../shared/document/ejson';
import { lexOptionsFor, type SqlDialect, sqlDialectFor } from '../shared/sqlIdent';
import type { DdlSchema } from './ddl';
import { findMatchingParen, MONGO_STATEMENT_RE, splitTopLevelArgs } from './mongoStatement';
import { ddlDiagnostics } from './sqlDiagnostics';

// lintSql's own lexical checks first, then — only while a non-empty DdlSchema exists —
// the DDL-driven warnings layered on top. Both read the same document; a lexically broken
// statement (an unterminated quote) still gets its own DDL diagnostics for whatever the parser
// managed to make sense of, since Lezer's own error recovery never throws.
function lintSqlConsole(
  dialect: SqlDialect | undefined,
  schema: DdlSchema | undefined,
): (text: string) => ConsoleDiagnostic[] {
  // P108 Part 11 F4: same per-dialect lexical options every other lintSql/splitSqlStatements/
  // tokenizeSql call site now passes (format.ts, sql-split.ts callers, sqlNodes.ts) — a diagnostic
  // must never disagree with what Run/Format/hover consider one statement's own lexical shape.
  const lexOptions = lexOptionsFor(dialect);
  return (text) => {
    const lexical = lintSql(text, lexOptions);
    if (!dialect || !schema || schema.tables.length === 0) return lexical;
    return [...lexical, ...ddlDiagnostics(dialect, text, schema)];
  };
}

const MONGO_BRACKET_PAIRS: Record<string, string> = { ')': '(', ']': '[', '}': '{' };

// A `//` line comment starting at `text[i]` — returns the index just past it (the newline itself,
// if any, is left for the main loop's own default `i++`, matching the pre-extraction behaviour).
function skipLineComment(text: string, i: number): number {
  let j = i;
  while (j < text.length && text[j] !== '\n') j++;
  return j;
}

// A `/* */` block comment starting at `text[i]` — returns the index just past it (EOF if unclosed).
function skipBlockComment(text: string, i: number): number {
  const end = text.indexOf('*/', i + 2);
  return end === -1 ? text.length : end + 2;
}

// Index of the first character that is not whitespace or a `//` / `/* */` comment.
function skipLeadingTrivia(text: string): number {
  let i = 0;
  while (i < text.length) {
    if (/\s/.test(text[i])) i++;
    else if (text[i] === '/' && text[i + 1] === '/') i = skipLineComment(text, i);
    else if (text[i] === '/' && text[i + 1] === '*') i = skipBlockComment(text, i);
    else break;
  }
  return i;
}

// A single/double-quoted run starting at `text[i]` (the opening quote itself) — `\` escapes the
// next character. `next` is one past the closing quote when `closed`, or `text.length` when the
// string ran to EOF unterminated.
function scanStringLiteral(text: string, i: number): { next: number; closed: boolean } {
  const quote = text[i];
  const n = text.length;
  let j = i + 1;
  while (j < n) {
    if (text[j] === '\\') {
      j += 2;
      continue;
    }
    if (text[j] === quote) return { next: j + 1, closed: true };
    j++;
  }
  return { next: n, closed: false };
}

// Exported for its own direct table-driven test (P94 pass 3 §6 item 1) — this function had zero
// prior coverage, direct or indirect, before that pass's refactor.
export function lintMongoBrackets(text: string): ConsoleDiagnostic[] {
  const issues: ConsoleDiagnostic[] = [];
  const stack: Array<{ ch: string; pos: number }> = [];
  const n = text.length;
  let i = 0;
  while (i < n) {
    const c = text[i];
    if (c === '/' && text[i + 1] === '/') {
      i = skipLineComment(text, i);
      continue;
    }
    if (c === '/' && text[i + 1] === '*') {
      i = skipBlockComment(text, i);
      continue;
    }
    if (c === "'" || c === '"') {
      const start = i;
      const { next, closed } = scanStringLiteral(text, i);
      i = next;
      if (!closed) {
        issues.push({
          from: start,
          to: n,
          severity: 'error',
          message: t('shared.sqlLint.unterminatedString.error'),
        });
        return issues;
      }
      continue;
    }
    if (c === '(' || c === '[' || c === '{') {
      stack.push({ ch: c, pos: i });
      i++;
      continue;
    }
    if (c === ')' || c === ']' || c === '}') {
      const top = stack[stack.length - 1];
      if (top && top.ch === MONGO_BRACKET_PAIRS[c]) stack.pop();
      else
        issues.push({
          from: i,
          to: i + 1,
          severity: 'error',
          message: t('studio.console.lint.unmatched.error', { char: c }),
        });
      i++;
      continue;
    }
    i++;
  }
  for (const { pos, ch } of stack) {
    issues.push({
      from: pos,
      to: pos + 1,
      severity: 'error',
      message: t('studio.console.lint.unbalanced.error', { char: ch }),
    });
  }
  return issues;
}

// D24: the shape check and the method-membership check reuse the exact same regex and the same
// MONGO_CONSOLE_METHODS list mongo/console.ts's own parser and SUPPORTED_METHODS are built from
// (D21), and the unsupported-method message matches mongo/console.ts's own wording verbatim so a
// diagnostic can never contradict the adapter.
//
// D12: once the shape and the method are known good, each top-level argument gets validated
// against this app's own Mongo shell-literal grammar (views/shared/document/ejson.ts's
// tryParseShellText) — not JSON.parse, which would reject valid shell input this console actually
// accepts (unquoted keys, single quotes, ObjectId(…)/ISODate(…) constructor calls).
//
// Statements split exactly as Run all/Format split them (`;`, `//` comments), so each one is
// linted on its own and an error in a later statement is never hidden behind an earlier one.
function lintMongoConsole(text: string): ConsoleDiagnostic[] {
  if (text.trim().length === 0) return [];
  const statements = splitSqlStatements(text, {
    ...lexOptionsFor(undefined),
    slashSlashComments: true,
  });
  const issues: ConsoleDiagnostic[] = [];
  for (const stmt of statements) {
    for (const d of lintMongoStatement(text.slice(stmt.start, stmt.end))) {
      issues.push({ ...d, from: d.from + stmt.start, to: d.to + stmt.start });
    }
  }
  return issues;
}

function lintMongoStatement(raw: string): ConsoleDiagnostic[] {
  const bracketIssues = lintMongoBrackets(raw);
  if (bracketIssues.length > 0) return bracketIssues;

  const lead = skipLeadingTrivia(raw);
  if (lead >= raw.length) return [];
  const text = raw.slice(lead);
  const shift = (d: ConsoleDiagnostic): ConsoleDiagnostic => ({
    ...d,
    from: d.from + lead,
    to: d.to + lead,
  });
  return lintMongoShape(text).map(shift);
}

function lintMongoShape(text: string): ConsoleDiagnostic[] {
  const match = MONGO_STATEMENT_RE.exec(text);
  if (!match) {
    return [
      {
        from: 0,
        to: text.length,
        severity: 'error',
        message: t('studio.console.format.mongoShape.error', {
          pattern: 'db.<collection>.<method>(…)',
        }),
      },
    ];
  }
  const [full, collection, method] = match;
  if (!MONGO_CONSOLE_METHODS.includes(method)) {
    const from = full.lastIndexOf(method);
    return [
      {
        from,
        to: from + method.length,
        severity: 'error',
        message: t('studio.console.format.mongoMethod.error', {
          method: `db.${collection}.${method}()`,
        }),
      },
    ];
  }

  const openParen = match.index + full.length - 1;
  const closeParen = findMatchingParen(text, openParen);
  for (const arg of splitTopLevelArgs(text, openParen + 1, closeParen)) {
    const leading = arg.text.search(/\S/);
    if (leading === -1) continue; // an empty argument (or trailing comma) is not this check's job
    const trimmed = arg.text.trim();
    const result = tryParseShellText(trimmed);
    if (!result.ok) {
      const from = arg.from + leading;
      return [
        {
          from,
          to: from + trimmed.length,
          severity: 'error',
          message: t('studio.console.lint.invalidArgument.error', { offset: result.offset }),
        },
      ];
    }
  }
  return [];
}

// D24's Redis rule set: an unterminated quoted string reuses redis/console.ts's own tokenizer
// error wording; the multi-line warning documents F10's splitter bug (out of scope to fix here)
// rather than pretending the console already handles it.
function lintRedisConsole(text: string): ConsoleDiagnostic[] {
  const n = text.length;
  let i = 0;
  while (i < n) {
    const c = text[i];
    if (c === "'" || c === '"') {
      const quote = c;
      const start = i;
      i++;
      let closed = false;
      while (i < n) {
        if (text[i] === '\\') {
          i += 2;
          continue;
        }
        if (text[i] === quote) {
          i++;
          closed = true;
          break;
        }
        i++;
      }
      if (!closed) {
        return [
          {
            from: start,
            to: n,
            severity: 'error',
            message: t('studio.console.lint.unterminatedQuote.error'),
          },
        ];
      }
      continue;
    }
    i++;
  }

  const issues: ConsoleDiagnostic[] = [];
  let offset = 0;
  for (const stmt of text.split(';')) {
    const nonEmptyLines = stmt.split('\n').filter((l) => l.trim().length > 0);
    if (nonEmptyLines.length > 1) {
      issues.push({
        from: offset,
        to: offset + stmt.length,
        severity: 'warning',
        message: t('studio.console.lint.newlineSeparated.error'),
      });
    }
    offset += stmt.length + 1;
  }
  return issues;
}

/** undefined for any connection kind with no console at all — a mounted ConsoleView never has
 *  one (caps.sql gates the tab), so this only ever actually returns a function. `schema` is only
 *  consulted for a SQL kind (D7). */
export function consoleLintSource(
  kind: ConnectionKind | undefined,
  schema?: DdlSchema,
): ((doc: string) => ConsoleDiagnostic[]) | undefined {
  if (kind === 'mongodb') return lintMongoConsole;
  if (kind === 'redis') return lintRedisConsole;
  const dialect = sqlDialectFor(kind);
  if (dialect) return lintSqlConsole(dialect, schema);
  return undefined;
}
