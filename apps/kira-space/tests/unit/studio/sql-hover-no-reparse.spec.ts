// No redundant full parse on the hover path — measured up to ~51ms on a 191KB document, over the
// app's own 50ms interaction budget, with no debounce (unlike the lint path). `editor/hover.ts`'s
// `buildHoverSource` read CodeMirror's own incrementally-maintained `syntaxTree(view.state)` once
// and handed it through; that tree no longer exists (Monaco keeps none). `sql-tokens.ts`'s own D2
// memo — module-level, keyed by (options, source) both compared by reference, size 2 — is what
// carries the property forward: several calls over the same untouched document string in one
// interaction burst share one real tokenize.
//
// Keep the rewrite rather than delete the spec — the underlying requirement is still
// real. Proved here by reference identity, not a call counter: an ES module's own named export
// can't be reassigned from outside to count calls (the old spec's `dialect.language.parser.parse =
// …` monkeypatch worked only because that was a mutable property on a library object, not a module
// binding) — a memo hit is instead observable as "the exact same tree object comes back", which a
// fresh scan could never produce (it always builds new node objects).
import { describe, expect, test } from 'bun:test';
import { tokenizeSql } from '@shared/domain/sqlTokens';
import type { DdlSchema } from '../../../frontend/src/studio/views/console/ddl';
import { ddlDiagnostics } from '../../../frontend/src/studio/views/console/sqlDiagnostics';
import { sqlHoverSource } from '../../../frontend/src/studio/views/console/sqlHover';
import { tokenOptionsFor } from '../../../frontend/src/studio/views/console/sqlNodes';

describe('tokenizeSql — D2 memo: same (options, source) reference pair returns the cached tree', () => {
  test('two calls with the same dialect options and the same source string return the identical tree', () => {
    const sql = 'SELECT users.id FROM users';
    const options = tokenOptionsFor('postgres'); // memoised per dialect — same reference each call
    expect(tokenOptionsFor('postgres')).toBe(options); // sanity: the reference memo this relies on

    const first = tokenizeSql(sql, options);
    const second = tokenizeSql(sql, options);
    expect(second).toBe(first); // the memo hit — a fresh scan would build a new root object

    const third = tokenizeSql('SELECT 1', options);
    expect(third).not.toBe(first); // a genuinely different document is never served the wrong tree
  });

  test('a third distinct document evicts the oldest cache entry (size 2, FIFO)', () => {
    const options = tokenOptionsFor('mysql');
    const a = tokenizeSql('SELECT 1', options);
    tokenizeSql('SELECT 2', options);
    tokenizeSql('SELECT 3', options); // evicts 'SELECT 1's entry
    const aAgain = tokenizeSql('SELECT 1', options);
    expect(aAgain).not.toBe(a); // no longer cached — a fresh (but still correct) tree
  });
});

describe('sqlHover.ts / sqlDiagnostics.ts still resolve correctly across a hover/lint/hover burst', () => {
  const SCHEMA: DdlSchema = {
    tables: [{ name: 'users', columns: [{ name: 'id', type: 'integer' }] }],
  };

  test('one real scan (by the identity test above) still serves every caller correctly', () => {
    const sql = 'SELECT users.id FROM users';
    const source = sqlHoverSource('postgres', SCHEMA);
    expect(source).toBeDefined();
    const pos = sql.indexOf('users.id') + 'users.'.length + 1; // inside "id"

    expect(source?.(sql, pos)).not.toBeNull();
    expect(ddlDiagnostics('postgres', sql, SCHEMA)).toEqual([]);
    expect(source?.(sql, pos)).not.toBeNull();
  });
});
