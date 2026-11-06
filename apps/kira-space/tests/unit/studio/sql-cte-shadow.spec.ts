// A CTE named after a real DDL table must resolve to the CTE, not the base table it shadows —
// resolveAliasMap (sqlDiagnostics.ts) and resolveHover's qualifier resolution (sqlHover.ts) both
// used to ignore cteNames and fall straight through to findTable, producing a false-positive
// "unknown column" diagnostic ("a false positive is worse than a missing diagnostic") and a wrong
// hover card for entirely valid SQL.
import { describe, expect, test } from 'bun:test';
import type { DdlSchema } from '../../../frontend/src/studio/views/console/ddl';
import { ddlDiagnostics } from '../../../frontend/src/studio/views/console/sqlDiagnostics';
import { sqlHoverSource } from '../../../frontend/src/studio/views/console/sqlHover';

const SCHEMA: DdlSchema = {
  tables: [
    {
      name: 'orders',
      columns: [
        { name: 'id', type: 'integer' },
        { name: 'total', type: 'numeric' },
      ],
    },
  ],
};

describe('a CTE shadowing a DDL table name (P12 round 2 F7)', () => {
  test('ddlDiagnostics reports no false-positive unknown column for a CTE-only column', () => {
    const sql = 'WITH orders AS (SELECT 1 AS n) SELECT orders.n FROM orders';
    const issues = ddlDiagnostics('postgres', sql, SCHEMA);
    expect(issues).toEqual([]);
  });

  // "id" also exists on the base DDL table — a real column name, chosen so a resolution bug
  // that falls through to the base table produces a *wrong* hover card instead of merely no
  // diagnostic (a column name absent from both wouldn't distinguish "correctly resolved to
  // nothing" from "incorrectly resolved to the base table, which also lacks it").
  const sql = 'WITH orders AS (SELECT 1 AS id) SELECT orders.id FROM orders';

  test('sqlHover resolves the qualifier to nothing (the CTE), not the base table', () => {
    const source = sqlHoverSource('postgres', SCHEMA);
    expect(source).toBeDefined();
    // +1: lands one character inside "id" rather than right at the boundary "." node.
    const pos = sql.indexOf('orders.id') + 'orders.'.length + 1;
    const info = source?.(sql, pos);
    expect(info).toBeNull();
  });
});
