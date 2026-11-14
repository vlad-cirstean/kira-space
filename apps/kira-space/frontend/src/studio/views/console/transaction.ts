import type { ConnectionKind } from '@shared/domain/connection';
import { reactive } from 'vue';

// Decision 8: a console tab that leaves a transaction open asks before it closes, and confirming
// rolls it back. The engine reports no transaction state, so the console reads it off the
// statements each run executed.

const SQL_KINDS = new Set<ConnectionKind>(['postgres', 'mysql', 'mariadb', 'sqlite']);

export interface OpenTransaction {
  connectionId: string;
  path: string;
}

const open = reactive(new Map<string, OpenTransaction>());

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

/** What one statement does to the open transaction: opens it, ends it, or leaves it alone. */
export function transactionEffect(statement: string): 'open' | 'close' | null {
  const s = stripLeadingComments(statement).toLowerCase();
  if (/^(begin|start\s+transaction)\b/.test(s)) return 'open';
  if (/^rollback\b/.test(s) && /\bto\b/.test(s)) return null; // savepoint rollback keeps it open
  if (/^(commit|rollback|end|abort)\b/.test(s)) return 'close';
  return null;
}

/** Applies a successfully executed batch; the last transaction statement in it decides. */
export function trackTransaction(
  tabId: string,
  kind: ConnectionKind,
  target: OpenTransaction,
  statements: readonly string[],
): void {
  if (!SQL_KINDS.has(kind)) return;
  for (const statement of statements) {
    const effect = transactionEffect(statement);
    if (effect === 'open') open.set(tabId, target);
    else if (effect === 'close') open.delete(tabId);
  }
}

export function hasOpenTransaction(tabId: string): boolean {
  return open.has(tabId);
}

/** Forgets the tab's transaction and returns it so the caller can roll it back. */
export function takeOpenTransaction(tabId: string): OpenTransaction | null {
  const tx = open.get(tabId) ?? null;
  open.delete(tabId);
  return tx;
}
