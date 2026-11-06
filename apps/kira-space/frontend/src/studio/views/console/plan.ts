import type { ConnectionKind } from '@shared/domain/connection';
import { cellText, isNull, isTruncated, type Page, type TabularPage } from '@shared/protocol/page';
import { t } from '@theme/i18n';
import type { QueryPlan } from './planModel';
import { parseClickhousePlan } from './planParsers/clickhouse';
import { parseMariadbPlan } from './planParsers/mariadb';
import { parseMysqlPlan } from './planParsers/mysql';
import { parsePostgresPlan } from './planParsers/postgres';
import { parseSqlitePlan, type SqliteExplainRow } from './planParsers/sqlite';

const decoder = new TextDecoder();

// Postgres/MySQL/MariaDB's JSON-format EXPLAIN returns the whole plan as
// one cell — a plan over page/chunk.go's 64 KiB MaxCellBytes arrives with that cell clipped mid-
// JSON, `isTruncated` set, and would otherwise fail JSON.parse with an opaque "Unexpected end of
// JSON input". A distinct, named condition lets each caller show something specific instead: the
// manual Explain button a real message, auto-explain a "couldn't check" strip rather than nothing.
export class ExplainTruncatedError extends Error {
  constructor() {
    super(t('studio.console.plan.tooLarge.error'));
    this.name = 'ExplainTruncatedError';
  }
}

function cellAt(page: TabularPage, row: number, col: number): string | null {
  const chunk = page.chunks[col];
  if (col < 0 || !chunk || isNull(chunk, row)) return null;
  return cellText(chunk, row, decoder);
}

/** True when the single cell `parseExplainPages`'s postgres/mysql/mariadb branches parse the
 *  whole plan from was clipped on the wire (page/chunk.go's MaxCellBytes) — the JSON it holds is
 *  necessarily incomplete, so parsing it further would only produce a confusing parser error. */
function firstCellTruncated(page: TabularPage): boolean {
  const chunk = page.chunks[0];
  return !!chunk && isTruncated(chunk, 0);
}

function columnIndex(page: TabularPage, name: string): number {
  return page.columns.findIndex((c) => c.name === name);
}

/** Turns the `TabularPage`(s) `explain()`'s own `data.execute` call returned back into
 *  one normalized `QueryPlan` — the glue between the statement composer and the per-dialect
 *  parsers. Throws on a page shape that doesn't match what was verified against
 *  real servers; both callers (the Explain button, auto-explain) run this inside a try/catch —
 *  a failure here must never surface as an error for the auto-explain
 *  path, only degrade silently to "run normally". */
export function parseExplainPages(
  kind: ConnectionKind,
  pages: Page[],
  thresholdRows: number,
): QueryPlan {
  const first = pages[0];
  if (first?.kind !== 'tabular') {
    throw new Error(t('studio.console.plan.notTabular.error'));
  }

  // A page the console cap stopped is an incomplete plan.
  if (pages.some((p) => p.kind === 'tabular' && p.position.truncated))
    throw new ExplainTruncatedError();

  switch (kind) {
    case 'postgres':
      if (firstCellTruncated(first)) throw new ExplainTruncatedError();
      return parsePostgresPlan(cellAt(first, 0, 0) ?? '', thresholdRows);
    case 'mysql':
      if (firstCellTruncated(first)) throw new ExplainTruncatedError();
      return parseMysqlPlan(cellAt(first, 0, 0) ?? '', thresholdRows);
    case 'mariadb':
      if (firstCellTruncated(first)) throw new ExplainTruncatedError();
      return parseMariadbPlan(cellAt(first, 0, 0) ?? '', thresholdRows);
    case 'sqlite': {
      const idCol = columnIndex(first, 'id');
      const parentCol = columnIndex(first, 'parent');
      const detailCol = columnIndex(first, 'detail');
      const rows: SqliteExplainRow[] = [];
      for (let r = 0; r < first.rowCount; r++) {
        rows.push({
          id: Number(cellAt(first, r, idCol) ?? 0),
          parent: Number(cellAt(first, r, parentCol) ?? 0),
          detail: cellAt(first, r, detailCol) ?? '',
        });
      }
      return parseSqlitePlan(rows);
    }
    case 'clickhouse': {
      // ClickHouse's EXPLAIN also returns its whole plan as one cell
      // (same truncation risk as postgres/mysql/mariadb above) — missing this check produced a
      // raw "JSON Parse error: Unterminated string" for the manual button and silently showed
      // nothing for auto-explain.
      if (firstCellTruncated(first)) throw new ExplainTruncatedError();
      // The second Execute page is EXPLAIN ESTIMATE's own result — absent only if the
      // server returned fewer pages than requested, which parseExplainPages treats as "no
      // estimate" rather than throwing (page order is asserted in the unit test, not
      // trusted blindly at runtime).
      const estimatePage = pages[1];
      const estimateRows: Array<{ rows: number }> = [];
      if (estimatePage?.kind === 'tabular') {
        const rowsCol = columnIndex(estimatePage, 'rows');
        for (let r = 0; r < estimatePage.rowCount; r++) {
          estimateRows.push({ rows: Number(cellAt(estimatePage, r, rowsCol) ?? 0) });
        }
      }
      return parseClickhousePlan(cellAt(first, 0, 0) ?? '', estimateRows, thresholdRows);
    }
    default:
      throw new Error(`no EXPLAIN parser for connection kind "${kind}"`);
  }
}
