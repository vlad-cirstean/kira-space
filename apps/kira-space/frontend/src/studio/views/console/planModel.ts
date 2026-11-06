import type { ConnectionKind } from '@shared/domain/connection';

// One normalized plan model every dialect's differing EXPLAIN output is parsed
// into (planParsers/*.ts) — every dialect's own fields survive it verbatim through `metrics`
// rather than being discarded, and `raw` keeps the exact server text one toggle away (the
// warning that SQLite's EXPLAIN QUERY PLAN format may change between releases is answered by never
// claiming the parse is authoritative).

/** The unit a dialect's own native cost figure is reported in — never compared across dialects
 *  (F17: MariaDB's and MySQL's identically-named `cost` field disagree by three orders of
 *  magnitude for a comparable scan). 'none' covers SQLite and ClickHouse, which report no cost. */
type CostUnit = 'postgres-planner' | 'mysql-cost' | 'mariadb-cost' | 'none';

export interface PlanIssue {
  severity: 'warn' | 'info';
  code: string;
  message: string;
}

export interface PlanNode {
  /** "Seq Scan on t" / "SCAN t" / "ReadFromMergeTree default.t" — the node's own kind plus the
   *  relation it touches, when the dialect names one. */
  label: string;
  relation?: string;
  /** The dialect's own condition/filter text, verbatim (Postgres's Filter, MySQL/MariaDB's
   *  attached_condition, ClickHouse's Indexes[].Condition, SQLite's detail string). */
  detail?: string;
  estimatedRows?: number;
  cost?: { total: number; startup?: number };
  /** Every other field that dialect reported, projected as label/value pairs — nothing the server
   *  said is discarded, even when this model has no typed slot for it. */
  metrics: Array<{ label: string; value: string }>;
  issues: PlanIssue[];
  children: PlanNode[];
}

export interface QueryPlan {
  kind: ConnectionKind;
  root: PlanNode;
  /** D14: max estimated-rows-read over the plan's own scan-type nodes (postgres/mysql/mariadb),
   *  or ClickHouse's EXPLAIN ESTIMATE sum. Absent on sqlite — it reports no estimate at all. */
  estimatedRowsRead?: number;
  nativeCost?: { value: number; unit: CostUnit };
  /** Whole-plan roll-up of every node's own issues, deduplicated by code — this is what the panel
   *  lists above the tree. */
  issues: PlanIssue[];
  /** True only when estimatedRowsRead exists and meets/exceeds the configured threshold — D19's
   *  auto-explain warning trigger is `overThreshold || issues.some(i => i.severity === 'warn')`,
   *  since a structural issue (a full scan, say) can fire with no row estimate at all (sqlite). */
  overThreshold: boolean;
  /** The exact EXPLAIN text the server returned — sqlite has no single blob, so its own parser
   *  reconstructs one line per row in the same id/parent/notused/detail shape it received. */
  raw: string;
}

/** Every scan/read-type PlanNode a dialect's own parser found, together with the row estimate
 *  `estimatedRowsRead` is the max of. Threaded through planIssues.ts so the "wide scan"
 *  issue and the plan-level `estimatedRowsRead`/`overThreshold` fields are always derived
 *  from the exact same node set — never two passes that could disagree. */
export interface ScanEstimate {
  node: PlanNode;
  rows: number;
}
