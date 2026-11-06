import { t } from '@theme/i18n';
import {
  isOverThreshold,
  maxEstimatedRows,
  pushIndexIssues,
  pushWideScanIssue,
  rawFieldMetrics,
  rollupIssues,
  tableLabel,
} from '../planIssues';
import type { PlanNode, QueryPlan, ScanEstimate } from '../planModel';

// MariaDB 11.4's `EXPLAIN FORMAT=JSON` — same statement spelling as MySQL's, a genuinely different
// response schema: every table sits inside a `nested_loop` array (even a single-table query),
// wrapped in `read_sorted_file`/`filesort` for a sort and `block-nl-join` for a block nested-loop
// join; the scalar cost field is named `cost` (≈ seconds under MariaDB 11.x's model) rather than
// MySQL's `cost_info.query_cost`, and `filtered` is a plain number, not MySQL's string.
// `FORMAT=TREE` is a hard error on this engine (ERROR 1791) — never used.
interface RawTable {
  table_name?: string;
  access_type?: string;
  possible_keys?: string[];
  key?: string;
  key_length?: string;
  used_key_parts?: string[];
  ref?: string[];
  loops?: number;
  rows?: number;
  cost?: number;
  filtered?: number;
  attached_condition?: string;
}

interface NestedLoopEntry {
  table?: RawTable;
  read_sorted_file?: {
    filesort?: { sort_key?: string; table?: RawTable; nested_loop?: NestedLoopEntry[] };
  };
  'block-nl-join'?: { nested_loop?: NestedLoopEntry[]; buffer_type?: string };
}

interface RawBlock {
  cost?: number;
  nested_loop?: NestedLoopEntry[];
}

const TABLE_TYPED_KEYS = new Set([
  'table_name',
  'access_type',
  'possible_keys',
  'key',
  'rows',
  'attached_condition',
]);

function tableNode(table: RawTable, scans: ScanEstimate[]): PlanNode {
  const node: PlanNode = {
    label: tableLabel(table),
    relation: table.table_name,
    detail: table.attached_condition,
    estimatedRows: table.rows,
    cost: table.cost !== undefined ? { total: table.cost } : undefined,
    metrics: rawFieldMetrics(table, TABLE_TYPED_KEYS),
    issues: [],
    children: [],
  };

  // D15: full scan / unused index.
  pushIndexIssues(node, table);

  if (node.estimatedRows !== undefined) scans.push({ node, rows: node.estimatedRows });
  return node;
}

function wrap(label: string, children: PlanNode[], issues: PlanNode['issues'] = []): PlanNode {
  return { label, metrics: [], issues, children };
}

function entryNode(entry: NestedLoopEntry, scans: ScanEstimate[]): PlanNode | null {
  if (entry.table) return tableNode(entry.table, scans);

  // D15: filesort — a read_sorted_file/filesort wrapper node is present.
  const filesort = entry.read_sorted_file?.filesort;
  if (filesort) {
    const child = filesort.table
      ? tableNode(filesort.table, scans)
      : filesort.nested_loop
        ? wrap('Nested loop join', entriesToNodes(filesort.nested_loop, scans))
        : null;
    return wrap('Sort (filesort)', child ? [child] : [], [
      {
        severity: 'warn',
        code: 'filesort',
        message: filesort.sort_key
          ? t('studio.console.plan.issue.filesortKey.description', { key: filesort.sort_key })
          : t('studio.console.plan.issue.filesort.description'),
      },
    ]);
  }

  const blockJoin = entry['block-nl-join'];
  if (blockJoin?.nested_loop) {
    return wrap('Block nested loop join', entriesToNodes(blockJoin.nested_loop, scans));
  }

  return null;
}

function entriesToNodes(entries: NestedLoopEntry[], scans: ScanEstimate[]): PlanNode[] {
  return entries.map((e) => entryNode(e, scans)).filter((n): n is PlanNode => n !== null);
}

export function parseMariadbPlan(rawText: string, thresholdRows: number): QueryPlan {
  const parsed = JSON.parse(rawText) as { query_block?: RawBlock };
  const block = parsed.query_block ?? {};
  const scans: ScanEstimate[] = [];
  const children = block.nested_loop ? entriesToNodes(block.nested_loop, scans) : [];
  const root: PlanNode = {
    label: t('studio.console.plan.root.query.label'),
    cost: block.cost !== undefined ? { total: block.cost } : undefined,
    metrics: [],
    issues: [],
    children,
  };
  for (const scan of scans) pushWideScanIssue(scan.node, scan.rows, thresholdRows);

  const estimatedRowsRead = maxEstimatedRows(scans);
  return {
    kind: 'mariadb',
    root,
    estimatedRowsRead,
    nativeCost: block.cost !== undefined ? { value: block.cost, unit: 'mariadb-cost' } : undefined,
    issues: rollupIssues(root),
    overThreshold: isOverThreshold(estimatedRowsRead, thresholdRows),
    raw: rawText,
  };
}
