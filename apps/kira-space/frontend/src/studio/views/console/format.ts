import type { ConnectionKind } from '@shared/domain/connection';
import { MONGO_CONSOLE_METHODS } from '@shared/domain/console';
import type { EditorLanguageId } from '@shared/domain/editor';
import { splitSqlStatements } from '@shared/domain/sqlSplit';
import { errorText } from '@workbench/bridge/codedError';
import { beautifyShellText } from '../shared/document/ejson';
import type { CatalogReason } from '../shared/document/rawTree';
import { hashCommentsFor, lexOptionsFor, sqlDialectFor } from '../shared/sqlIdent';
import { findMatchingParen, MONGO_STATEMENT_RE, splitTopLevelArgs } from './mongoStatement';

/** true for the five SQL kinds and MongoDB — the only consoles with a real formatter behind
 *  them (D1/D6). Redis's statements are a flat token list with nothing to reformat, and
 *  kafka/sqs/s3 have no console at all. */
export function canFormatConsole(kind: ConnectionKind | undefined): boolean {
  return kind === 'mongodb' || sqlDialectFor(kind) !== undefined;
}

/** The editor language a console of this kind uses. */
export function consoleLanguageFor(kind: ConnectionKind | undefined): EditorLanguageId {
  if (kind === 'mongodb') return 'mongo';
  if (kind === 'redis') return 'redis';
  if (sqlDialectFor(kind) !== undefined) return 'sql';
  return 'plain';
}

type SqlFormatterModule = typeof import('./sqlFormatterEntry');

// D2: memoised so only the first Format press ever pays the import cost.
let sqlFormatterModule: Promise<SqlFormatterModule> | undefined;
function loadSqlFormatter(): Promise<SqlFormatterModule> {
  sqlFormatterModule ??= import('./sqlFormatterEntry').catch((e: unknown) => {
    sqlFormatterModule = undefined;
    throw e;
  });
  return sqlFormatterModule;
}

// D3: keyed on ConnectionKind, not on sqlIdent.ts's own SqlDialect union — that union
// deliberately collapses MariaDB and MySQL into one 'mysql' member for quoting/grammar reasons
// (sqlIdent.ts:6-15) that don't apply to a formatter, and sql-formatter ships them as separate
// dialects.
function formatterDialectFor(mod: SqlFormatterModule, kind: ConnectionKind) {
  switch (kind) {
    case 'postgres':
      return mod.postgresql;
    case 'mariadb':
      return mod.mariadb;
    case 'mysql':
      return mod.mysql;
    case 'sqlite':
      return mod.sqlite;
    case 'clickhouse':
      return mod.clickhouse;
    default:
      return undefined;
  }
}

// keywordCase stays at the library default 'preserve' rather than the app's own uppercase
// completion house style (languages.ts) — 'upper' rewrites an unquoted identifier that collides
// with a keyword (e.g. a `database` column becomes `DATABASE`), which is silently wrong for
// ClickHouse's case-sensitive identifiers.
const SQL_FORMAT_OPTIONS = {
  tabWidth: 2,
  useTabs: false,
  keywordCase: 'preserve',
  identifierCase: 'preserve',
  linesBetweenQueries: 1,
} as const;

// F5: a parse failure throws an Error whose message is a multi-thousand-character nearley
// expectation dump — only its first line ("Parse error at token: «EOF» at line 1 column 23") is
// ever fit to show a user.
function firstLine(message: string): string {
  const idx = message.indexOf('\n');
  return idx === -1 ? message : message.slice(0, idx);
}

// D5: one statement's own text, matched, method-checked and argument-beautified the same way
// lintMongoConsole (lint.ts) already walks it — a format refusal here uses the linter's own
// wording so a diagnostic and a format refusal never contradict each other.
function formatMongoStatement(
  stmt: string,
): { text: string; reason?: undefined } | { reason: CatalogReason } {
  const match = MONGO_STATEMENT_RE.exec(stmt);
  if (!match)
    return {
      reason: {
        key: 'studio.console.format.mongoShape.error',
        params: { pattern: 'db.<collection>.<method>(…)' },
      },
    };
  const [full, collection, method] = match;
  if (!MONGO_CONSOLE_METHODS.includes(method)) {
    return {
      reason: {
        key: 'studio.console.format.mongoMethod.error',
        params: { method: `db.${collection}.${method}()` },
      },
    };
  }

  const openParen = match.index + full.length - 1;
  const closeParen = findMatchingParen(stmt, openParen);
  // The return below rebuilds the statement from the parsed collection/method/args
  // alone — anything after the closing paren (a chained `.limit(5)`, a trailing comment) used to be
  // silently dropped, turning `db.c.find({}).limit(5)` into `db.c.find({})`. The Go console rejects
  // that trailing content outright (mongo/console.go's own "unexpected trailing content after
  // statement") — refusing here instead keeps the statement verbatim, the same contract every
  // other refusal in this function already follows, rather than Format silently changing the query.
  if (stmt.slice(closeParen + 1).trim().length > 0) {
    return { reason: { key: 'studio.console.format.trailing.error' } };
  }
  const rawArgs = splitTopLevelArgs(stmt, openParen + 1, closeParen)
    .map((a) => a.text.trim())
    .filter((t) => t.length > 0);

  const args: string[] = [];
  for (const raw of rawArgs) {
    const result = beautifyShellText(raw, 'indented');
    if (!result.ok)
      return { reason: result.reason ?? { key: 'studio.console.format.invalidArgument.error' } };
    args.push(result.text);
  }

  if (args.length === 0) return { text: `db.${collection}.${method}()` };
  if (args.length === 1) return { text: `db.${collection}.${method}(${args[0]})` };

  // Two or more arguments: one per line at indent 2, each argument's own continuation lines
  // shifted by the same two spaces (mongosh's own rendering of an updateOne/aggregate call).
  const indented = args
    .map((arg) =>
      arg
        .split('\n')
        .map((line) => `  ${line}`)
        .join('\n'),
    )
    .join(',\n');
  return { text: `db.${collection}.${method}(\n${indented}\n)` };
}

// P108 Part 11 F9: `stmt.text` never carries its own ';' (D12's own note below) — when a formatted
// statement's own last line holds a line comment (sql-formatter keeps a trailing `--`/`#` comment
// last, and a failed statement is emitted verbatim, comment included), a plain join(';\n\n') places
// the ';' inside that comment. The splitter then sees one statement instead of two on the next Run
// all/Format press — Format silently changed what Run all/Run statement execute, and pressing
// Format again can't undo it (the merged text re-splits the exact same way). Mirrors
// adapters.JoinConsoleStatements' own fix for the identical defect in the op-log join (P108 Part 11
// F5): an extra '\n' before such a statement's own separator moves the ';' onto its own line, out
// of the comment's reach. Over-triggering on a false positive (the prefix text inside a string
// literal on the last line, not actually a comment) only adds a harmless blank line, never changes
// what re-splits into — a cheap substring check, not a real lexical scan, same trade-off
// JoinConsoleStatements makes.
export function joinFormattedStatements(
  statements: readonly string[],
  hashComments: boolean,
): string {
  return statements
    .map((stmt, index) => {
      if (index === statements.length - 1) return stmt;
      const lastLine = stmt.slice(stmt.lastIndexOf('\n') + 1);
      const hasTrailingComment =
        lastLine.includes('--') || (hashComments && lastLine.includes('#'));
      return hasTrailingComment ? `${stmt}\n` : stmt;
    })
    .join(';\n\n');
}

interface FormatFailure {
  /** Which statement, 0-based — the same index D12's caret mapping walks the before/after split
   *  by. */
  index: number;
  reason: CatalogReason;
}

export interface FormatResult {
  text: string;
  /** False only when NOTHING formatted (the all-broken behaviour, preserved for
   *  tests/ui/console-format.spec.ts's all-broken case: text untouched, one reason shown). True
   *  whenever at least one statement formatted, even if others didn't — a broken neighbour no
   *  longer takes the whole press down with it. */
  ok: boolean;
  /** Present only when `ok` is false, mirroring BeautifyResult's own `reason` — the P13-era
   *  single-statement callers (formatConsoleText's own unit tests) still read this. */
  reason?: CatalogReason;
  /** One entry per statement Format could not touch — empty when every statement formatted (the
   *  common case) or when the whole document is whitespace-only. */
  failures: FormatFailure[];
}

// Every statement that formats, formats; one the grammar rejects is emitted VERBATIM, in place,
// never dropped — which is what keeps the caret-by-index mapping exact (the statement count
// survives regardless of how many failed). Splits with the same splitter and options Run all
// already uses (ConsoleView.runAll's own splitSqlStatements call), so "what Format treats as a
// statement" and "what Run all treats as a statement" can never disagree, and rejoins with `;\n\n`
// — the same separator formatMongo already produced and what sql-formatter's own
// linesBetweenQueries: 1 produces for a single multi-statement call.
export async function formatConsoleText(kind: ConnectionKind, text: string): Promise<FormatResult> {
  if (text.trim().length === 0) return { text, ok: true, failures: [] };

  const dialect = sqlDialectFor(kind);
  // P108 Part 11 F4: the same per-dialect lexical options ConsoleView.vue's own splitOptionsFor
  // pairs for Run all — Format must never split a document differently than the run it formats
  // for, or the caret-by-index mapping below (D12) points at the wrong statement.
  const statements = splitSqlStatements(text, {
    ...lexOptionsFor(dialect),
    slashSlashComments: kind === 'mongodb',
  });
  if (statements.length === 0) return { text, ok: true, failures: [] };

  // One statement's own text in, its formatted text or a failure reason out — resolved once,
  // outside the map below, so the SQL branch's mod/dialect lookup (an async import, a per-kind
  // table lookup) happens exactly once regardless of how many statements there are.
  let formatOne: (
    stmtText: string,
  ) => { text: string; reason?: undefined } | { reason: CatalogReason };
  if (kind === 'mongodb') {
    formatOne = formatMongoStatement;
  } else {
    const mod = await loadSqlFormatter();
    const sqlDialectObject = formatterDialectFor(mod, kind);
    if (!sqlDialectObject) {
      return {
        text,
        ok: false,
        reason: { key: 'studio.console.format.nothingToFormat.error' },
        failures: statements.map((_, index) => ({
          index,
          reason: { key: 'studio.console.format.nothingToFormat.error' },
        })),
      };
    }
    formatOne = (stmtText) => {
      try {
        return {
          text: mod.formatDialect(stmtText, { dialect: sqlDialectObject, ...SQL_FORMAT_OPTIONS }),
        };
      } catch (err) {
        const message = errorText(err);
        return {
          reason: {
            key: 'studio.console.format.engine.error',
            params: { message: firstLine(message) },
          },
        };
      }
    };
  }

  const failures: FormatFailure[] = [];
  const out = statements.map((stmt, index) => {
    const result = formatOne(stmt.text);
    if (result.reason !== undefined) {
      failures.push({ index, reason: result.reason });
      return stmt.text;
    }
    return result.text;
  });

  if (failures.length === statements.length) {
    return { text, ok: false, reason: failures[0]?.reason, failures };
  }
  // splitSqlStatements' own pushIfNonEmpty slices up to, not through, each statement's terminator
  // (sql-split.ts) — so `stmt.text` never carries its own ';'. A plain join(';\n\n')
  // therefore emits N-1 semicolons for N statements, silently deleting the document's last one on
  // every press: a regression against the whole-document formatDialect call, which preserved every
  // ';' it was given. The terminator is now a property of the SOURCE, not of the join: a document
  // that ended in ';' still does, one that did not still does not.
  const endedWithTerminator = /;\s*$/.test(text);
  const joined = joinFormattedStatements(out, hashCommentsFor(dialect));
  return { text: endedWithTerminator ? `${joined};` : joined, ok: true, failures };
}
