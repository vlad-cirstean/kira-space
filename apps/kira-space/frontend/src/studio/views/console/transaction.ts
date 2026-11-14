import type { ConnectionKind } from '@shared/domain/connection';
import { toCodedError } from '@workbench/bridge/codedError';
import { reactive } from 'vue';

// Decision 8: closing a console tab that leaves a transaction open asks first, and confirming
// rolls it back. A transaction lives on the connection's session, not on a tab: every console tab
// on one connection and database shares it, so it is keyed by that session and ended only when the
// last of those tabs closes.
//
// Where the engine reports its transaction state (Postgres, MariaDB, SQLite: the execute response
// and a failed run's error details carry it) that state is the truth. MySQL 8.4 reports none, so
// there the console reads it off the statements a run executed; the lexer below also runs for the
// others as the fallback for a run that carries no state.

const SQL_KINDS = new Set<ConnectionKind>(['postgres', 'mysql', 'mariadb', 'sqlite']);
const MYSQL_KINDS = new Set<ConnectionKind>(['mysql', 'mariadb']);

/** The server error number of a statement the parser rejected: no implicit commit ran for it. */
const ER_PARSE_ERROR = 1064;

export interface OpenTransaction {
  connectionId: string;
  path: string;
}

/** What a run reported back. `failed` is present for a failed run: `index` is the statement the
 *  engine stopped at (absent when none ran), `errno` the server error number of MySQL. */
export interface RunOutcome {
  failed?: { index?: number; errno?: number };
  /** The engine's own transaction state after the run, when it reports one. */
  transaction?: 'idle' | 'open';
}

/** What closing a session's transaction runs: the statements, on the session's own connection. */
export interface SessionEnd extends OpenTransaction {
  statements: string[];
}

interface SessionState {
  target: OpenTransaction;
  kind: ConnectionKind;
  open: boolean;
  /** MySQL family: `SET autocommit=0` is on, so the next statement opens a transaction and a
   *  closing ROLLBACK must switch it back on. */
  autocommitOff: boolean;
  /** MySQL family: an XA transaction started and not yet committed, rolled back or prepared. */
  xa: { id: string; ended: boolean } | null;
}

/** The session a run used: the connection and its database (the path's first segment). */
function sessionKey(target: OpenTransaction): string {
  return `${target.connectionId}\n${target.path.split('/')[0]}`;
}

const sessions = reactive(new Map<string, SessionState>());
// The session each console tab last ran on, so a tab knows who it shares a transaction with.
const sessionOfTab = reactive(new Map<string, string>());

/** The failed run's details: the statement it stopped at, the server error number and the engine's
 *  transaction state. */
export function failureOutcome(err: unknown): RunOutcome {
  const details = toCodedError(err).details;
  const d = (typeof details === 'object' && details !== null ? details : {}) as {
    statementIndex?: unknown;
    sqlErrno?: unknown;
    inTransaction?: unknown;
  };
  return {
    failed: {
      index: typeof d.statementIndex === 'number' ? d.statementIndex : undefined,
      errno: typeof d.sqlErrno === 'number' ? d.sqlErrno : undefined,
    },
    transaction:
      typeof d.inTransaction === 'boolean' ? (d.inTransaction ? 'open' : 'idle') : undefined,
  };
}

/** Drops the comments and blanks in front of a statement: `--` and, for the MySQL family, `#` line
 *  comments; `/* *\/` block comments, which nest on Postgres only. */
export function stripLeadingComments(sql: string, kind: ConnectionKind = 'postgres'): string {
  const mysql = MYSQL_KINDS.has(kind);
  let s = sql.trimStart();
  for (;;) {
    if (s.startsWith('--') || (mysql && s.startsWith('#'))) {
      const nl = s.indexOf('\n');
      s = nl < 0 ? '' : s.slice(nl + 1).trimStart();
    } else if (s.startsWith('/*')) {
      const end = blockCommentEnd(s, kind === 'postgres');
      s = end < 0 ? '' : s.slice(end).trimStart();
    } else return s;
  }
}

/** The index just past the block comment `s` starts with, or -1 when it never closes. */
function blockCommentEnd(s: string, nested: boolean): number {
  let depth = 0;
  let i = 0;
  while (i < s.length - 1) {
    const pair = s.slice(i, i + 2);
    if (pair === '/*' && (nested || depth === 0)) {
      depth++;
      i += 2;
    } else if (pair === '*/') {
      depth--;
      i += 2;
      if (depth === 0) return i;
    } else i++;
  }
  return -1;
}

// MySQL and MariaDB commit an open transaction before running these (DDL, account and table
// maintenance, replication control, table locks), whether or not the statement then succeeds.
const IMPLICIT_COMMIT =
  /^(create|alter|drop|truncate|rename|grant|revoke|analyze|optimize|repair|check\s+table|checksum|flush|reset|lock|unlock|install|uninstall|cache\s+index|load\s+index|set\s+password|(start|stop)\s+(replica|slave|group_replication)|change\s+(master|replication)|purge\s+(binary|master)|(create|alter|drop)\s+user)\b/;
const TEMPORARY_TABLE = /^(create|drop)\s+temporary\b/;

function commitsImplicitly(s: string): boolean {
  return IMPLICIT_COMMIT.test(s) && !TEMPORARY_TABLE.test(s);
}

// With autocommit off the first statement that reads or writes a table opens a transaction.
// `CALL` is counted too: a procedure can leave one open and the statement text cannot say.
const TOUCHES_TABLES =
  /^(insert|update|delete|replace|table|values|load\s+data|call|handler|do)\b|^(select|with)\b[\s\S]*\bfrom\b/;

// The XA id keeps its case: `XA END` and `XA ROLLBACK` must name it exactly.
const XA_START = /^xa\s+(?:start|begin)\s+([\s\S]*?)(?:\s+(?:join|resume))?\s*;?\s*$/i;

/** What one statement does to the session's transaction state. */
function applyStatement(st: SessionState, statement: string, kind: ConnectionKind): void {
  const s = stripLeadingComments(statement, kind).toLowerCase().trimEnd();
  if (/^(begin|start\s+transaction)\b/.test(s)) {
    st.open = true;
    return;
  }
  if (/^rollback\b/.test(s) && /\bto\b/.test(s)) return; // savepoint rollback keeps it open
  // `AND CHAIN` starts the next transaction at once; `AND NO CHAIN` ends it like a plain COMMIT.
  if (/^(commit|rollback)\b.*\band\s+chain\b/.test(s)) return;
  if (/^(commit|rollback|end|abort)\b/.test(s)) {
    st.open = false;
    return;
  }
  if (!MYSQL_KINDS.has(kind)) {
    // A savepoint outside a transaction opens one (SQLite, Postgres).
    if (/^savepoint\b/.test(s)) st.open = true;
    return;
  }
  const xaStart = XA_START.exec(stripLeadingComments(statement, kind));
  if (xaStart) {
    st.open = true;
    st.xa = { id: xaStart[1], ended: false };
    return;
  }
  if (/^xa\s+end\b/.test(s)) {
    if (st.xa) st.xa.ended = true;
    return;
  }
  if (/^xa\s+(commit|rollback|prepare)\b/.test(s)) {
    st.xa = null;
    st.open = false;
    return;
  }
  if (/^set\s+(@@(session\.)?|session\s+)?autocommit\s*=\s*(0|off|false)\b/.test(s)) {
    st.autocommitOff = true;
    return;
  }
  if (/^set\s+(@@(session\.)?|session\s+)?autocommit\s*=\s*(1|on|true)\b/.test(s)) {
    st.autocommitOff = false;
    st.open = false; // switching autocommit on commits
    return;
  }
  if (commitsImplicitly(s)) {
    st.open = false;
    return;
  }
  if (st.autocommitOff && TOUCHES_TABLES.test(s)) st.open = true;
}

function liveSession(target: OpenTransaction, kind: ConnectionKind): SessionState {
  const key = sessionKey(target);
  return sessions.get(key) ?? { target, kind, open: false, autocommitOff: false, xa: null };
}

/**
 * Applies a run to the session's transaction state. A failed run applied the statements before the
 * one the engine stopped at; that statement changes nothing, but for the implicit commit a MySQL
 * DDL statement makes before it runs (not when the parser rejected it). The engine's own state,
 * when the run carries it, overrides what the statements imply.
 */
export function trackTransaction(
  tabId: string,
  kind: ConnectionKind,
  target: OpenTransaction,
  statements: readonly string[],
  outcome: RunOutcome = {},
): void {
  if (!SQL_KINDS.has(kind)) return;
  const key = sessionKey(target);
  sessionOfTab.set(tabId, key);
  const st = { ...liveSession(target, kind), target, kind };
  const failed = outcome.failed;
  const ran = failed ? statements.slice(0, failed.index ?? 0) : statements;
  for (const statement of ran) applyStatement(st, statement, kind);
  const failedStatement = failed?.index === undefined ? undefined : statements[failed.index];
  if (
    failedStatement !== undefined &&
    MYSQL_KINDS.has(kind) &&
    failed?.errno !== undefined &&
    failed.errno !== ER_PARSE_ERROR &&
    commitsImplicitly(stripLeadingComments(failedStatement, kind).toLowerCase())
  ) {
    st.open = false;
  }
  if (outcome.transaction !== undefined) {
    st.open = outcome.transaction === 'open';
    if (!st.open) st.xa = null;
  }
  if (st.open || st.autocommitOff || st.xa) sessions.set(key, st);
  else sessions.delete(key);
}

/** Another console tab still runs on the session, so closing this one leaves the transaction to it. */
function sharedWithAnotherTab(tabId: string, key: string): boolean {
  for (const [id, k] of sessionOfTab) if (id !== tabId && k === key) return true;
  return false;
}

/** Whether closing the tab ends an open transaction: the last console tab on its session does. */
export function hasOpenTransaction(tabId: string): boolean {
  const key = sessionOfTab.get(tabId);
  if (key === undefined) return false;
  const st = sessions.get(key);
  return Boolean(st && (st.open || st.xa) && !sharedWithAnotherTab(tabId, key));
}

/** The statements that end a session's transaction and restore what the console changed. */
function endStatements(st: SessionState): string[] {
  const out: string[] = [];
  if (st.xa) {
    if (!st.xa.ended) out.push(`XA END ${st.xa.id}`);
    out.push(`XA ROLLBACK ${st.xa.id}`);
  } else if (st.open) {
    out.push('ROLLBACK');
  }
  // The rollback leaves `autocommit=0` on the pinned session: later grid commits on it would
  // never commit.
  if (st.autocommitOff) out.push('SET autocommit=1');
  return out;
}

/** Forgets the tab, and returns what ends the session's transaction when closing the tab ends it
 *  so the caller can run it. */
export function takeOpenTransaction(tabId: string): SessionEnd | null {
  const key = sessionOfTab.get(tabId);
  sessionOfTab.delete(tabId);
  if (key === undefined || sharedWithAnotherTab(tabId, key)) return null;
  const st = sessions.get(key);
  sessions.delete(key);
  if (!st) return null;
  const statements = endStatements(st);
  return statements.length > 0 ? { ...st.target, statements } : null;
}

/** The connection dropped: the server ended its session and any transaction on it. */
export function forgetConnectionTransactions(connectionId: string): void {
  for (const [key, st] of [...sessions])
    if (st.target.connectionId === connectionId) sessions.delete(key);
}
