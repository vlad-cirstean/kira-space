import type { ConnectionKind } from '@shared/domain/connection';
import { toCodedError } from '@workbench/bridge/codedError';
import { reactive } from 'vue';

// Decision 8: closing a console tab that leaves a transaction open asks first, and confirming
// rolls it back. The engine reports no transaction state, so the console reads it off the
// statements each run executed. A transaction lives on the connection's session, not on a tab:
// every console tab on one connection and database shares it, so it is keyed by that session and
// ended only when the last of those tabs closes.

const SQL_KINDS = new Set<ConnectionKind>(['postgres', 'mysql', 'mariadb', 'sqlite']);
const MYSQL_KINDS = new Set<ConnectionKind>(['mysql', 'mariadb']);

export interface OpenTransaction {
  connectionId: string;
  path: string;
}

/** The session a run used: the connection and its database (the path's first segment). */
function sessionKey(target: OpenTransaction): string {
  return `${target.connectionId}\n${target.path.split('/')[0]}`;
}

const open = reactive(new Map<string, OpenTransaction>());
// The session each console tab last ran on, so a tab knows who it shares a transaction with.
const sessionOfTab = reactive(new Map<string, string>());

function stripLeadingComments(sql: string): string {
  let s = sql.trimStart();
  for (;;) {
    if (s.startsWith('--')) {
      const nl = s.indexOf('\n');
      s = nl < 0 ? '' : s.slice(nl + 1).trimStart();
    } else if (s.startsWith('/*')) {
      const end = s.indexOf('*/');
      s = end < 0 ? '' : s.slice(end + 2).trimStart();
    } else return s;
  }
}

// MySQL and MariaDB commit an open transaction before running these (DDL, account and table
// maintenance, table locks), whether or not the statement then succeeds.
const IMPLICIT_COMMIT =
  /^(create|alter|drop|truncate|rename|grant|revoke|analyze|optimize|repair|flush|lock|unlock|install|uninstall|cache\s+index|load\s+index)\b/;
const TEMPORARY_TABLE = /^(create|drop)\s+temporary\b/;

function commitsImplicitly(s: string): boolean {
  return IMPLICIT_COMMIT.test(s) && !TEMPORARY_TABLE.test(s);
}

/** What one statement does to the open transaction: opens it, ends it, or leaves it alone. */
export function transactionEffect(
  statement: string,
  kind: ConnectionKind = 'postgres',
): 'open' | 'close' | null {
  const s = stripLeadingComments(statement).toLowerCase();
  if (/^(begin|start\s+transaction)\b/.test(s)) return 'open';
  if (/^rollback\b/.test(s) && /\bto\b/.test(s)) return null; // savepoint rollback keeps it open
  // `AND CHAIN` starts the next transaction at once; `AND NO CHAIN` ends it like a plain COMMIT.
  if (/^(commit|rollback)\b.*\band\s+chain\b/.test(s)) return null;
  if (/^(commit|rollback|end|abort)\b/.test(s)) return 'close';
  if (MYSQL_KINDS.has(kind)) {
    // Autocommit off makes the next statement open a transaction; on commits the open one.
    if (/^set\s+(@@(session\.)?|session\s+)?autocommit\s*=\s*(0|off|false)\b/.test(s))
      return 'open';
    if (/^set\s+(@@(session\.)?|session\s+)?autocommit\s*=\s*(1|on|true)\b/.test(s)) return 'close';
    if (commitsImplicitly(s)) return 'close';
  }
  return null;
}

/** The index of the statement the engine stopped at, when the failed run's error names it. */
export function failedStatementIndex(err: unknown): number | undefined {
  const details = toCodedError(err).details;
  if (typeof details !== 'object' || details === null) return undefined;
  const index = (details as { statementIndex?: unknown }).statementIndex;
  return typeof index === 'number' ? index : undefined;
}

/**
 * Applies a run to the session's transaction state. A failed run (`failedAt` is the index of the
 * statement the engine stopped at) applied the statements before it; the failed statement itself
 * changes nothing but for the implicit commit a MySQL DDL statement makes before it runs.
 */
export function trackTransaction(
  tabId: string,
  kind: ConnectionKind,
  target: OpenTransaction,
  statements: readonly string[],
  failedAt?: number,
): void {
  if (!SQL_KINDS.has(kind)) return;
  const key = sessionKey(target);
  sessionOfTab.set(tabId, key);
  const ran = failedAt === undefined ? statements : statements.slice(0, failedAt);
  for (const statement of ran) {
    const effect = transactionEffect(statement, kind);
    if (effect === 'open') open.set(key, target);
    else if (effect === 'close') open.delete(key);
  }
  const failed = failedAt === undefined ? undefined : statements[failedAt];
  if (failed !== undefined && MYSQL_KINDS.has(kind)) {
    if (commitsImplicitly(stripLeadingComments(failed).toLowerCase())) open.delete(key);
  }
}

/** Another console tab still runs on the session, so closing this one leaves the transaction to it. */
function sharedWithAnotherTab(tabId: string, key: string): boolean {
  for (const [id, k] of sessionOfTab) if (id !== tabId && k === key) return true;
  return false;
}

/** Whether closing the tab ends an open transaction: the last console tab on its session does. */
export function hasOpenTransaction(tabId: string): boolean {
  const key = sessionOfTab.get(tabId);
  return key !== undefined && open.has(key) && !sharedWithAnotherTab(tabId, key);
}

/** Forgets the tab, and returns the session's transaction when closing the tab ends it so the
 *  caller can roll it back. */
export function takeOpenTransaction(tabId: string): OpenTransaction | null {
  const key = sessionOfTab.get(tabId);
  sessionOfTab.delete(tabId);
  if (key === undefined || sharedWithAnotherTab(tabId, key)) return null;
  const tx = open.get(key) ?? null;
  open.delete(key);
  return tx;
}

/** The connection dropped: the server ended its session and any transaction on it. */
export function forgetConnectionTransactions(connectionId: string): void {
  for (const [key, tx] of [...open]) if (tx.connectionId === connectionId) open.delete(key);
}
