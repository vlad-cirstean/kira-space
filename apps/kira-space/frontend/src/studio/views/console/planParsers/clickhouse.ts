import { t } from '@theme/i18n';
import { pushWideScanIssue, rollupIssues } from '../planIssues';
import type { PlanNode, QueryPlan } from '../planModel';

// ClickHouse's `EXPLAIN PLAN json = 1, indexes = 1, description = 1`
// plus a second `EXPLAIN ESTIMATE` call. Neither reports a cost, so `nativeCost` stays
// unset; ESTIMATE's own row count is the *only* size figure available, and the plan's own
// `Indexes[]` entries are the only index-usage signal — `Selected Granules == Initial Granules`
// (with `Condition: "true"`) is ClickHouse's own "no index narrowed the read" tell.
interface RawIndex {
  Type?: string;
  Keys?: string[];
  Condition?: string;
  'Search Algorithm'?: string;
  'Initial Parts'?: number;
  'Selected Parts'?: number;
  'Initial Granules'?: number;
  'Selected Granules'?: number;
}

interface RawNode {
  'Node Type'?: string;
  'Node Id'?: string;
  Description?: string;
  Indexes?: RawIndex[];
  Plans?: RawNode[];
}

// Only `rows` is ever read (D14's estimatedRowsRead) — database/table/parts/marks are real
// columns EXPLAIN ESTIMATE returns (F15) but nothing in this model needs them.
export interface EstimateRow {
  rows: number;
}

function indexMetrics(indexes: RawIndex[]): Array<{ label: string; value: string }> {
  const out: Array<{ label: string; value: string }> = [];
  for (const idx of indexes) {
    const type = idx.Type ?? 'Index';
    if (idx.Keys)
      out.push({
        label: t('studio.console.plan.metric.keys.label', { type }),
        value: idx.Keys.join(', '),
      });
    if (idx.Condition !== undefined)
      out.push({
        label: t('studio.console.plan.metric.condition.label', { type }),
        value: idx.Condition,
      });
    if (idx['Search Algorithm'])
      out.push({
        label: t('studio.console.plan.metric.search.label', { type }),
        value: idx['Search Algorithm'],
      });
    if (idx['Initial Parts'] !== undefined || idx['Selected Parts'] !== undefined) {
      out.push({
        label: t('studio.console.plan.metric.parts.label', { type }),
        value: `${idx['Selected Parts'] ?? '?'} / ${idx['Initial Parts'] ?? '?'} selected`,
      });
    }
    if (idx['Initial Granules'] !== undefined || idx['Selected Granules'] !== undefined) {
      out.push({
        label: t('studio.console.plan.metric.granules.label', { type }),
        value: `${idx['Selected Granules'] ?? '?'} / ${idx['Initial Granules'] ?? '?'} selected`,
      });
    }
  }
  return out;
}

function buildNode(raw: RawNode, mergeTreeNodes: PlanNode[]): PlanNode {
  const nodeType = raw['Node Type'] ?? 'Node';
  const node: PlanNode = {
    label: raw.Description ? `${nodeType}: ${raw.Description}` : nodeType,
    relation: nodeType === 'ReadFromMergeTree' ? raw.Description : undefined,
    metrics: raw.Indexes ? indexMetrics(raw.Indexes) : [],
    issues: [],
    children: (raw.Plans ?? []).map((child) => buildNode(child, mergeTreeNodes)),
  };

  if (nodeType === 'ReadFromMergeTree') {
    mergeTreeNodes.push(node);
    const pk = raw.Indexes?.find((idx) => idx.Type === 'PrimaryKey');
    if (pk) {
      // D15: the primary key did not narrow the read.
      if (
        pk['Selected Granules'] !== undefined &&
        pk['Selected Granules'] === pk['Initial Granules']
      ) {
        node.issues.push({
          severity: 'warn',
          code: 'pk-not-narrowed',
          message: t('studio.console.plan.issue.pkNotNarrowed.description', {
            relation: node.relation ?? '?',
          }),
        });
      }
      // D15: every part read.
      if (
        pk['Selected Parts'] !== undefined &&
        pk['Selected Parts'] === pk['Initial Parts'] &&
        (pk['Initial Parts'] ?? 0) > 1
      ) {
        node.issues.push({
          severity: 'warn',
          code: 'all-parts-read',
          message: t('studio.console.plan.issue.allParts.description', {
            relation: node.relation ?? '?',
            count: pk['Initial Parts'] ?? 0,
          }),
        });
      }
    }
  }

  return node;
}

export function parseClickhousePlan(
  planRawText: string,
  estimateRows: EstimateRow[],
  thresholdRows: number,
): QueryPlan {
  const parsed = JSON.parse(planRawText) as Array<{ Plan: RawNode }>;
  const mergeTreeNodes: PlanNode[] = [];
  const root = buildNode(parsed[0]?.Plan ?? {}, mergeTreeNodes);

  // D14: ClickHouse has no per-node row estimate at all — EXPLAIN ESTIMATE's own summed `rows` is
  // the only figure available, so the "wide read" issue (D15) is attached to the ReadFromMergeTree
  // node(s) the plan actually found rather than derived from the tree itself.
  const estimatedRowsRead = estimateRows.reduce((sum, r) => sum + r.rows, 0);
  const target = mergeTreeNodes[0] ?? root;
  pushWideScanIssue(target, estimatedRowsRead, thresholdRows);

  const raw = `${planRawText}\n\n-- EXPLAIN ESTIMATE --\n${JSON.stringify(estimateRows)}`;
  return {
    kind: 'clickhouse',
    root,
    estimatedRowsRead: estimateRows.length > 0 ? estimatedRowsRead : undefined,
    nativeCost: undefined,
    issues: rollupIssues(root),
    overThreshold: estimateRows.length > 0 && estimatedRowsRead >= thresholdRows,
    raw,
  };
}
