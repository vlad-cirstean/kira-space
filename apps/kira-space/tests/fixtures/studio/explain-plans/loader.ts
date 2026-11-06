// Shared parity fixtures — one `<case>.input.json`/`<case>.expected.json` pair per case,
// read by both `tests/unit/explain-plan.spec.ts` (this loader) and
// `internal/queryplan/parse_test.go` (its own Go reader). Generated from the TypeScript side first
// — the Go port is what gets pinned to these files, never the reverse.
import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { ConnectionKind } from '@shared/domain/connection';
import { createTabularPageBuilder, type Page } from '@shared/protocol/page';
import type { PlanNode, QueryPlan } from '../../../../frontend/src/studio/views/console/planModel';

const DIR = dirname(fileURLToPath(import.meta.url));

export interface PageSpec {
  columns: string[];
  rows: string[][];
}

export interface FixtureInput {
  kind: ConnectionKind;
  thresholdRows: number;
  // Present for a normal case — built into real Page objects via createTabularPageBuilder.
  pages?: PageSpec[];
  // Present for a truncated-cell case — the harness synthesizes an oversized single cell instead
  // of reading `pages` (§2.4's truncated-cell requirement; page/chunk.go's own MaxCellBytes is
  // what real over-the-wire truncation clips at, so the synthetic cell must exceed it for real).
  truncatedFirstCell?: boolean;
}

// Every case name §2.4 lists — one dialect's own shape per entry, plus one truncated-cell case per
// single-cell dialect (postgres/mysql/mariadb/clickhouse; sqlite returns multiple rows/columns,
// never a single oversized cell, so it is exempt).
export function fixtureCases(): string[] {
  return readdirSync(DIR)
    .filter((f) => f.endsWith('.input.json'))
    .map((f) => f.replace(/\.input\.json$/, ''))
    .sort();
}

export function loadInput(caseName: string): FixtureInput {
  return JSON.parse(readFileSync(join(DIR, `${caseName}.input.json`), 'utf8'));
}

export function loadExpected(caseName: string): unknown {
  return JSON.parse(readFileSync(join(DIR, `${caseName}.expected.json`), 'utf8'));
}

const STUB_COLUMN = {
  dataType: 'text',
  typeClass: 'text' as const,
  nullable: true,
  isPrimaryKey: false,
  generated: false,
};

/** Builds real Page objects out of a fixture's own `pages` spec, the same TabularPage shape
 *  `data.execute` returns — §2.4's own point: the parity test exercises the real page-parsing glue
 *  (cellAt/columnIndex/isTruncated), not a shortcut straight to a dialect's parser function. */
export function buildPages(spec: PageSpec[]): Page[] {
  return spec.map((pageSpec) => {
    const builder = createTabularPageBuilder(
      pageSpec.columns.map((name) => ({ name, ...STUB_COLUMN })),
    );
    for (const row of pageSpec.rows) builder.appendRow(row);
    return builder.finish({
      offset: 0,
      pageSize: pageSpec.rows.length,
      hasMore: false,
      nextToken: null,
      prevToken: null,
      strategy: 'offset',
    });
  });
}

const MAX_CELL_BYTES = 64 << 10;

/** Builds the one-page, one-oversized-cell shape every single-cell dialect's truncated-cell case
 *  needs — `explain-truncated.spec.ts`'s own `truncatedPlanPage` helper, generalized to any single
 *  QUERY-PLAN-shaped column name (harmless — parseExplainPages reads column 0 by position for
 *  every single-cell dialect, never by name). */
export function buildTruncatedPage(): Page[] {
  const builder = createTabularPageBuilder([{ name: 'QUERY PLAN', ...STUB_COLUMN }]);
  const oversized = `[{"Plan":${JSON.stringify({ x: 'y'.repeat(MAX_CELL_BYTES) })}}]`;
  builder.appendRow([oversized]);
  return [
    builder.finish({
      offset: 0,
      pageSize: 1,
      hasMore: false,
      nextToken: null,
      prevToken: null,
      strategy: 'offset',
    }),
  ];
}

/** §2.3: `metrics` order is declared non-contractual — Go maps have no insertion order, so both
 *  sides sort by label before comparing. Applied recursively, since every node in the tree carries
 *  its own `metrics`. */
export function sortMetrics(node: PlanNode): PlanNode {
  return {
    ...node,
    metrics: [...node.metrics].sort((a, b) => a.label.localeCompare(b.label)),
    children: node.children.map(sortMetrics),
  };
}

export function normalize(plan: QueryPlan): QueryPlan {
  return { ...plan, root: sortMetrics(plan.root) };
}
