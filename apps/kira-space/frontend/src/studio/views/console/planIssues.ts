import { t } from '@theme/i18n';
import type { PlanIssue, PlanNode, ScanEstimate } from './planModel';

// The pieces every per-dialect parser (planParsers/*.ts) shares once its own
// tree is built — a dialect-specific structural issue (full scan, missing index, filesort, an
// un-narrowed ClickHouse primary key, ...) is attached to a node by that dialect's own parser,
// since only it knows which raw field means what; what's shared is turning the collected scan
// estimates into the single `estimatedRowsRead`/`overThreshold` pair and rolling every node's
// issues up into the plan-level list the panel renders.

/** D14's "wide scan" rule: any scan-type node whose own row estimate meets/exceeds the threshold.
 *  Pushed onto that node directly (not the roll-up alone) so the tree view can point at exactly
 *  which node is the expensive one. */
export function pushWideScanIssue(node: PlanNode, rows: number, thresholdRows: number): void {
  if (rows < thresholdRows) return;
  node.issues.push({
    severity: 'warn',
    code: 'wide-scan',
    message: t('studio.console.plan.issue.wideScan.description', {
      label: node.label,
      count: rows,
    }),
  });
}

/** D14: the widest single read among the dialect's own scan-type nodes — a root/aggregate
 *  estimate is deliberately not used (F11's own example: a root Limit reporting 10 rows over a
 *  Seq Scan reporting 184,153 — the root figure would call that query cheap). */
export function maxEstimatedRows(scans: ScanEstimate[]): number | undefined {
  if (scans.length === 0) return undefined;
  return Math.max(...scans.map((s) => s.rows));
}

/** Depth-first walk collecting every node's own issues into one deduplicated, order-preserving
 *  list (severity+code+message triple) — the panel's issue list above the tree. */
export function rollupIssues(root: PlanNode): PlanIssue[] {
  const seen = new Set<string>();
  const out: PlanIssue[] = [];
  const visit = (node: PlanNode): void => {
    for (const issue of node.issues) {
      const key = `${issue.severity}:${issue.code}:${issue.message}`;
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(issue);
    }
    for (const child of node.children) visit(child);
  };
  visit(root);
  return out;
}

/** Strictly the row-count threshold — a plan can still carry a `warn` issue (a structural
 *  rule, or sqlite with no estimate at all) with `overThreshold` false; the auto-explain strip
 *  trigger is the OR of the two, kept separate here rather than folded together so each stays
 *  independently inspectable in the panel. */
export function isOverThreshold(
  estimatedRowsRead: number | undefined,
  thresholdRows: number,
): boolean {
  return estimatedRowsRead !== undefined && estimatedRowsRead >= thresholdRows;
}

/** P107 closing S1 sweep: mariadb.ts and mysql.ts each had a byte-identical `tableLabel` — same
 *  two fields, both dialects' own `RawTable` differs everywhere else, this pair stays structural. */
export function tableLabel(table: { table_name?: string; access_type?: string }): string {
  const name = table.table_name ?? '?';
  if (table.access_type === 'ALL') return t('studio.console.plan.fullScan.label', { name });
  if (table.access_type)
    return t('studio.console.plan.access.label', { access: table.access_type, name });
  return name;
}

/** mariadb.ts's `tableMetrics` and postgres.ts's `metricsFrom` were byte-identical past the
 *  parameter name — every untyped field of a raw node/table, stringified.
 *  mysql.ts's own version nests a second pass over an object value and stays local — a genuinely
 *  different shape, not this pair. */
export function rawFieldMetrics(
  raw: object,
  typedKeys: Set<string>,
): Array<{ label: string; value: string }> {
  const out: Array<{ label: string; value: string }> = [];
  for (const [key, value] of Object.entries(raw as Record<string, unknown>)) {
    if (typedKeys.has(key) || value === undefined) continue;
    out.push({ label: key, value: Array.isArray(value) ? value.join(', ') : String(value) });
  }
  return out;
}

/** P113 F10: mysql.ts and mariadb.ts each pushed this identical D15 full-scan/unused-index pair
 *  onto a table node — same two rules, same messages; every other raw field (mysql.ts's own
 *  `cost_info` metric expansion included) stays per-engine, since only this fragment matched. */
export function pushIndexIssues(
  node: PlanNode,
  table: { access_type?: string; possible_keys?: string[]; key?: string },
): void {
  if (table.access_type === 'ALL') {
    node.issues.push({
      severity: 'warn',
      code: 'full-scan',
      message: t('studio.console.plan.issue.fullScan.description', {
        relation: node.relation ?? '?',
      }),
    });
  }
  if (table.possible_keys && table.possible_keys.length > 0 && !table.key) {
    node.issues.push({
      severity: 'warn',
      code: 'unused-index',
      message: t('studio.console.plan.issue.unusedIndex.description', {
        relation: node.relation ?? '?',
        indexes: table.possible_keys.join(', '),
      }),
    });
  }
}
