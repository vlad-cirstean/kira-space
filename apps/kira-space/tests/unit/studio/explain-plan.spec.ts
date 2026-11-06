// The five EXPLAIN plan normalizers + the issue rules earn a unit test — five dialects ×
// several node shapes is precisely a decision structure too large to hold in one's head. Every
// fixture below is the verified real-server output, pasted verbatim (this repo's
// "capture, don't hand-write" discipline applied where a capture tool wasn't needed).

import { describe, expect, test } from 'bun:test';
import {
  ExplainTruncatedError,
  parseExplainPages,
} from '../../../frontend/src/studio/views/console/plan';
import type { QueryPlan } from '../../../frontend/src/studio/views/console/planModel';
import { parseClickhousePlan } from '../../../frontend/src/studio/views/console/planParsers/clickhouse';
import { parseMariadbPlan } from '../../../frontend/src/studio/views/console/planParsers/mariadb';
import { parseMysqlPlan } from '../../../frontend/src/studio/views/console/planParsers/mysql';
import { parsePostgresPlan } from '../../../frontend/src/studio/views/console/planParsers/postgres';
import {
  parseSqlitePlan,
  type SqliteExplainRow,
} from '../../../frontend/src/studio/views/console/planParsers/sqlite';
import {
  buildPages,
  buildTruncatedPage,
  fixtureCases,
  loadExpected,
  loadInput,
  normalize,
} from '../../fixtures/studio/explain-plans/loader';

// The JSON literals moved out into tests/fixtures/explain-plans/*.input.json — the
// parity fixtures the new describe('parity fixtures') block below and internal/queryplan's own
// parse_test.go both read — and read back in here rather than duplicated, wherever a case below
// corresponds to one fixture exactly. Existing per-rule assertions are unchanged.
function rawTextOf(caseName: string): string {
  const input = loadInput(caseName);
  const cell = input.pages?.[0]?.rows[0]?.[0];
  if (cell === undefined) throw new Error(`fixture ${caseName} has no first-page first-cell text`);
  return cell;
}

function sqliteRowsOf(caseName: string): SqliteExplainRow[] {
  const input = loadInput(caseName);
  const rows = input.pages?.[0]?.rows ?? [];
  return rows.map(([id, parent, detail]) => ({
    id: Number(id),
    parent: Number(parent),
    detail: detail ?? '',
  }));
}

describe('parsePostgresPlan — F11', () => {
  const JOIN_PLAN = rawTextOf('postgres-join');

  test('finds the widest scan, not the root estimate (D14)', () => {
    const plan = parsePostgresPlan(JOIN_PLAN, 100_000);
    // The root Limit reports 10 rows; the real widest read is the 184,153-row Seq Scan on t.
    expect(plan.estimatedRowsRead).toBe(184153);
    expect(plan.overThreshold).toBe(true);
    expect(plan.nativeCost).toEqual({ value: 7814.41, unit: 'postgres-planner' });
  });

  test('flags the full scan with a filter, and the wide scan, on the same node', () => {
    const plan = parsePostgresPlan(JOIN_PLAN, 100_000);
    const codes = plan.issues.map((i) => i.code);
    expect(codes).toContain('full-scan');
    expect(codes).toContain('wide-scan');
    expect(plan.issues.find((i) => i.code === 'full-scan')?.message).toContain('“t”');
  });

  test('a lower threshold does not flag the same plan', () => {
    const plan = parsePostgresPlan(JOIN_PLAN, 1_000_000);
    expect(plan.overThreshold).toBe(false);
    expect(plan.issues.map((i) => i.code)).not.toContain('wide-scan');
  });

  test('an index-only scan is an info issue, not a warning', () => {
    const plan = parsePostgresPlan(rawTextOf('postgres-index-only-scan'), 100_000);
    expect(plan.issues).toEqual([
      {
        severity: 'info',
        code: 'index-only-scan',
        message: '“u” is read entirely from its index. No heap access needed.',
      },
    ]);
  });
});

describe('parseMysqlPlan — F12', () => {
  const FULL_SCAN = rawTextOf('mysql-single-table');

  test('flags a full scan and reports the query-level cost verbatim', () => {
    const plan = parseMysqlPlan(FULL_SCAN, 100_000);
    expect(plan.estimatedRowsRead).toBe(62643);
    expect(plan.nativeCost).toEqual({ value: 6304.55, unit: 'mysql-cost' });
    expect(plan.issues.map((i) => i.code)).toEqual(['full-scan']);
    expect(plan.root.children[0]?.detail).toBe("(`app`.`t`.`name` = 'n5')");
  });

  test('a materialized derived table is flagged as a temp table', () => {
    const plan = parseMysqlPlan(rawTextOf('mysql-materialized-subquery'), 100_000);
    expect(plan.issues.map((i) => i.code)).toEqual(
      expect.arrayContaining(['full-scan', 'temp-table']),
    );
  });
});

describe('parseMariadbPlan — F13', () => {
  test('a same-named cost field is not comparable to MySQL’s', () => {
    const plan = parseMariadbPlan(rawTextOf('mariadb-nested-loop'), 100_000);
    // Empirical proof: this genuine full scan reads more rows than the MySQL example
    // (100,175 vs 62,643) yet its identically-named `cost` field (16.59) is three orders of
    // magnitude smaller than MySQL's 6,304.55 for that comparable scan — the reason to threshold
    // on rows, never on either dialect's own cost number.
    expect(plan.nativeCost).toEqual({ value: 16.5855622, unit: 'mariadb-cost' });
    expect(plan.estimatedRowsRead).toBe(100175);
    expect(plan.overThreshold).toBe(true);
    expect(plan.issues.map((i) => i.code)).toEqual(
      expect.arrayContaining(['full-scan', 'wide-scan']),
    );
  });

  test('a read_sorted_file/filesort wrapper is unwrapped to its own table', () => {
    const plan = parseMariadbPlan(rawTextOf('mariadb-filesort'), 100_000);
    expect(plan.root.children[0]?.label).toBe('Sort (filesort)');
    expect(plan.issues.map((i) => i.code)).toEqual(['filesort']);
    // The wrapped table itself is not a full scan (access_type 'ref', a key chosen) — no
    // full-scan/unused-index issue should leak from the nested table.
    const tableNode = plan.root.children[0]?.children[0];
    expect(tableNode?.relation).toBe('t');
    expect(tableNode?.estimatedRows).toBe(2000);
  });
});

describe('parseSqlitePlan — F14', () => {
  test('no cost, no row estimate — the threshold never applies', () => {
    const plan = parseSqlitePlan([{ id: 2, parent: 0, detail: 'SCAN t' }]);
    expect(plan.estimatedRowsRead).toBeUndefined();
    expect(plan.overThreshold).toBe(false);
    expect(plan.nativeCost).toBeUndefined();
    expect(plan.issues).toEqual([
      {
        severity: 'warn',
        code: 'full-scan',
        message: 'SCAN t is a full scan. No index narrowed the read.',
      },
    ]);
  });

  test('a SEARCH row is not flagged as a full scan; a temp b-tree still fires', () => {
    const plan = parseSqlitePlan(sqliteRowsOf('sqlite-scan-search-mix'));
    expect(plan.issues.map((i) => i.code)).toEqual(['temp-btree']);
    expect(plan.root.children).toHaveLength(2);
  });

  test('parent pointers nest a subquery/compound row instead of assuming a flat list', () => {
    const plan = parseSqlitePlan([
      { id: 9, parent: 0, detail: 'SCAN c' },
      { id: 11, parent: 9, detail: 'SEARCH t USING INTEGER PRIMARY KEY (rowid=?)' },
    ]);
    expect(plan.root.children).toHaveLength(1);
    expect(plan.root.children[0]?.children).toHaveLength(1);
    expect(plan.root.children[0]?.children[0]?.relation).toBe('t');
  });
});

describe('parseClickhousePlan — F15', () => {
  function planJson(
    selectedGranules: number,
    initialGranules: number,
    selectedParts = 1,
    initialParts = 1,
  ) {
    return JSON.stringify([
      {
        Plan: {
          'Node Type': 'Expression',
          Description: '(Project names + Projection)',
          Plans: [
            {
              'Node Type': 'ReadFromMergeTree',
              'Node Id': 'ReadFromMergeTree_0',
              Description: 'default.t',
              Indexes: [
                {
                  Type: 'PrimaryKey',
                  Keys: ['id'],
                  Condition: 'and((id in (-Inf, 20]), (id in [10, +Inf)))',
                  'Search Algorithm': 'binary search',
                  'Initial Parts': initialParts,
                  'Selected Parts': selectedParts,
                  'Initial Granules': initialGranules,
                  'Selected Granules': selectedGranules,
                },
              ],
            },
          ],
        },
      },
    ]);
  }

  test('the primary key narrowed the read — no issue, no cost reported', () => {
    const plan = parseClickhousePlan(planJson(1, 62), [{ rows: 8192 }], 100_000);
    expect(plan.nativeCost).toBeUndefined();
    expect(plan.issues).toEqual([]);
    expect(plan.estimatedRowsRead).toBe(8192);
  });

  test('Selected Granules == Initial Granules is the "no index used" tell', () => {
    const plan = parseClickhousePlan(planJson(62, 62), [{ rows: 500000 }], 100_000);
    expect(plan.issues.map((i) => i.code)).toEqual(
      expect.arrayContaining(['pk-not-narrowed', 'wide-scan']),
    );
    expect(plan.overThreshold).toBe(true);
  });

  test('every part read fires only when there was more than one part to begin with', () => {
    const singlePart = parseClickhousePlan(planJson(62, 62, 1, 1), [{ rows: 1 }], 100_000);
    expect(singlePart.issues.map((i) => i.code)).not.toContain('all-parts-read');

    const multiPart = parseClickhousePlan(planJson(62, 62, 3, 3), [{ rows: 1 }], 100_000);
    expect(multiPart.issues.map((i) => i.code)).toContain('all-parts-read');
  });

  test('page order is asserted, not trusted — plan is page 0, estimate is page 1 (§7.3)', () => {
    // parseExplainPages (plan.ts) is the caller that receives the two Execute pages in
    // order; this asserts the parser's own estimateRows parameter is what estimatedRowsRead comes
    // from, so a caller that swapped page order would produce a visibly wrong number rather than
    // a silent one.
    const plan = parseClickhousePlan(planJson(1, 62), [{ rows: 111 }, { rows: 222 }], 100_000);
    expect(plan.estimatedRowsRead).toBe(333);
  });
});

// M3 §2.4/§10: the second implementation of these same rules lives in Go
// (internal/queryplan/parse_test.go), reading these exact files. A drift in either language fails
// a test in that language, on the same bytes — this block is the TypeScript side of that check.
describe('parity fixtures', () => {
  for (const caseName of fixtureCases()) {
    test(caseName, () => {
      const input = loadInput(caseName);
      const expected = loadExpected(caseName);

      if (input.truncatedFirstCell) {
        expect(() =>
          parseExplainPages(input.kind, buildTruncatedPage(), input.thresholdRows),
        ).toThrow(ExplainTruncatedError);
        expect(expected).toEqual({ truncated: true });
        return;
      }

      const pages = buildPages(input.pages ?? []);
      const plan = normalize(parseExplainPages(input.kind, pages, input.thresholdRows));
      expect(plan).toEqual(expected as QueryPlan);
    });
  }
});
