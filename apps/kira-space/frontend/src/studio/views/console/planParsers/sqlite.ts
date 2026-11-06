import { t } from '@theme/i18n';
import { rollupIssues } from '../planIssues';
import type { PlanNode, QueryPlan } from '../planModel';

// SQLite's `EXPLAIN QUERY PLAN`. N rows × 4 columns
// (id, parent, notused, detail), no cost and no row estimate whatsoever, so `estimatedRowsRead`
// and `overThreshold` stay unset/false here — the settings panel says the threshold does
// not apply on this dialect rather than showing a silent zero. `parent` is 0 for nearly every row
// (nesting only appears for a subquery/compound query), so this builds a real id→children tree
// rather than assuming a flat list.
//
// sqlite.org/eqp.html: "The output format may change between SQLite releases. Applications
// should not depend on the output format of the EXPLAIN QUERY PLAN command." — taken as binding:
// `detail` is matched on a small set of leading tokens for issue detection and otherwise
// shown verbatim, never re-rendered from a deeper parse.
export interface SqliteExplainRow {
  id: number;
  parent: number;
  detail: string;
}

const RELATION_RE = /^(?:SCAN|SEARCH) (\S+)/;

function nodeForRow(detail: string): PlanNode {
  const relation = RELATION_RE.exec(detail)?.[1];
  const node: PlanNode = { label: detail, relation, detail, metrics: [], issues: [], children: [] };

  // D15: full scan — SCAN (as opposed to SEARCH, which narrows via an index or rowid).
  if (detail.startsWith('SCAN ')) {
    node.issues.push({
      severity: 'warn',
      code: 'full-scan',
      message: t('studio.console.plan.issue.sqliteFullScan.description', { detail }),
    });
  }
  if (detail.includes('USE TEMP B-TREE FOR ')) {
    node.issues.push({ severity: 'warn', code: 'temp-btree', message: detail });
  }
  // D15 (info): a covering index avoids a second lookup into the table itself.
  if (detail.includes('USING COVERING INDEX')) {
    node.issues.push({ severity: 'info', code: 'covering-index', message: detail });
  }
  return node;
}

export function parseSqlitePlan(rows: SqliteExplainRow[]): QueryPlan {
  const nodes = new Map<number, PlanNode>();
  for (const row of rows) nodes.set(row.id, nodeForRow(row.detail));

  const root: PlanNode = {
    label: t('studio.console.plan.root.sqlite.label'),
    metrics: [],
    issues: [],
    children: [],
  };
  for (const row of rows) {
    const node = nodes.get(row.id);
    if (!node) continue;
    const parent = row.parent !== 0 ? nodes.get(row.parent) : undefined;
    (parent ?? root).children.push(node);
  }

  return {
    kind: 'sqlite',
    root,
    estimatedRowsRead: undefined,
    nativeCost: undefined,
    issues: rollupIssues(root),
    overThreshold: false,
    raw: rows.map((r) => `${r.id}\t${r.parent}\t${r.detail}`).join('\n'),
  };
}
