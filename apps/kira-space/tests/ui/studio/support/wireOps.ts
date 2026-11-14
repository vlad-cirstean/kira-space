// The stream mock matches a request by its JSON text, so a contract's sorted keys must be put back
// in the order the grid builds them: every row object follows the table's column order.
export function wireOps(ops: unknown[], columns: readonly string[]): unknown[] {
  const inOrder = (row: unknown): unknown => {
    if (typeof row !== 'object' || row === null) return row;
    const rec = row as Record<string, unknown>;
    return Object.fromEntries(columns.filter((c) => c in rec).map((c) => [c, rec[c]]));
  };
  return (ops as Record<string, unknown>[]).map((o) => ({
    kind: o.kind,
    ...(o.key !== undefined ? { key: inOrder(o.key) } : {}),
    ...(o.changes !== undefined ? { changes: inOrder(o.changes) } : {}),
    ...(o.values !== undefined ? { values: inOrder(o.values) } : {}),
    ...(o.expect !== undefined ? { expect: inOrder(o.expect) } : {}),
  }));
}
