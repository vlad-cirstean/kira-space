import { compareNatural } from '@workbench/behaviour/match';
import type { RowKey, TabPending } from './pendingChanges';

// Decision 2: after a commit whose acknowledgement was lost, the staged set is settled against a
// reload of the table. The commit is one transaction, so it applied in full or not at all, unless
// another client touched a row meanwhile. Staged rows the reloaded page shows are compared one by
// one; rows it cannot show borrow the verdict of those it can; with no verdict the user is asked.

/** Canonical row identity: the key's column names in natural order with their values. */
export function rowKeyId(key: RowKey): string {
  const names = Object.keys(key).sort(compareNatural);
  return JSON.stringify(names.map((n) => [n, key[n]]));
}

export interface ReconcilePage {
  rowById: Map<string, number>;
  pkNames: string[];
  /** The reloaded cell text; undefined when the column is absent or the cell is truncated. */
  serverText(row: number, column: string): string | null | undefined;
}

export interface Unresolved {
  edits: string[];
  deletes: string[];
  inserts: string[];
}

export interface Verdict {
  /** Rows the server already holds in their staged form. */
  applied: number;
  /** Rows still holding their loaded values: the commit did not reach them. */
  notApplied: number;
  unresolved: Unresolved;
}

/** Clears staged edits and inserts the page shows as saved, keeps the rest with their loaded
 *  guards and lists what the page cannot show. */
export function settleOnPage(p: TabPending, page: ReconcilePage): Verdict {
  const verdict: Verdict = {
    applied: 0,
    notApplied: 0,
    unresolved: { edits: [], deletes: [], inserts: [] },
  };
  for (const [id, edit] of [...p.edits]) {
    const row = page.rowById.get(id);
    if (row === undefined) {
      verdict.unresolved.edits.push(id);
      continue;
    }
    const columns = Object.keys(edit.changes);
    if (columns.every((c) => page.serverText(row, c) === edit.changes[c])) {
      p.edits.delete(id);
      verdict.applied++;
      continue;
    }
    // Still at its loaded values: the commit did not reach it and the guard stays valid. Any other
    // text is a concurrent change; the guard stays as loaded so the next Commit hits the conflict
    // and asks (decision 3) instead of overwriting it.
    if (columns.every((c) => page.serverText(row, c) === edit.original[c])) verdict.notApplied++;
  }
  for (const id of [...p.deletes.keys()]) {
    const row = page.rowById.get(id);
    if (row === undefined) {
      verdict.unresolved.deletes.push(id);
      continue;
    }
    verdict.notApplied++; // the row is still there; its staged guard stands
  }
  for (const insert of [...p.inserts]) {
    if (insertStored(insert.values, page)) {
      p.inserts = p.inserts.filter((i) => i.id !== insert.id);
      verdict.applied++;
    } else {
      verdict.unresolved.inserts.push(insert.id);
    }
  }
  return verdict;
}

/** An insert is known stored only when it names its whole primary key and the page holds that row. */
function insertStored(values: Record<string, string | null>, page: ReconcilePage): boolean {
  if (page.pkNames.length === 0) return false;
  const key: RowKey = {};
  for (const name of page.pkNames) {
    const value = values[name] ?? null;
    if (value === null) return false;
    key[name] = value;
  }
  const row = page.rowById.get(rowKeyId(key));
  if (row === undefined) return false;
  return Object.entries(values).every(([c, v]) => {
    const text = page.serverText(row, c);
    return v === null || text === undefined || text === v;
  });
}

export function unresolvedCount(u: Unresolved): number {
  return u.edits.length + u.deletes.length + u.inserts.length;
}

export function dropUnresolved(p: TabPending, u: Unresolved): void {
  for (const id of u.edits) p.edits.delete(id);
  for (const id of u.deletes) p.deletes.delete(id);
  p.inserts = p.inserts.filter((i) => !u.inserts.includes(i.id));
}

/** Decides what the unmatched rows become. Saved rows prove the whole commit landed (drop them), a
 *  row still at its loaded values proves it did not (keep them); with neither the user decides.
 *  Returns whether to keep them. */
export async function settleUnresolved(
  verdict: Verdict,
  confirmKeep: (count: number) => Promise<boolean>,
): Promise<boolean> {
  const count = unresolvedCount(verdict.unresolved);
  if (count === 0) return true;
  if (verdict.applied > 0 && verdict.notApplied === 0) return false;
  if (verdict.applied === 0 && verdict.notApplied > 0) return true;
  return confirmKeep(count);
}
