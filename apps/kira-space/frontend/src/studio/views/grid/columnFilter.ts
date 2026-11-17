import type { TabularPage } from '@shared/protocol/page';
import { matchesQuery } from '@workbench/behaviour/match';
import { resolveColumnOrder } from '../shared/page/columns';

/** Lowercased, whitespace/comma-split query terms; empty when the query is blank. */
function columnQueryTerms(query: string): string[] {
  return query
    .toLowerCase()
    .split(/[\s,]+/)
    .filter((term) => term !== '');
}

function matchesColumnQuery(name: string, terms: readonly string[]): boolean {
  return terms.some((term) => matchesQuery(name, term));
}

// Display order is read per cell, so the result must be reference-stable per (order, terms):
// same O(1) rationale as resolveColumnOrder's own cache.
const visibleCache = new WeakMap<string[], Map<string, string[]>>();

/**
 * Display order narrowed by a column-name query. Primary-key columns stay while the query matches
 * something; a query matching nothing returns the full order (never a zero-column grid).
 */
export function visibleColumnOrder(
  page: TabularPage,
  stored: string[] | null,
  query: string,
): string[] {
  const order = resolveColumnOrder(page, stored);
  const terms = columnQueryTerms(query);
  if (terms.length === 0) return order;
  const key = terms.join('\u0000');
  let perOrder = visibleCache.get(order);
  if (!perOrder) {
    perOrder = new Map();
    visibleCache.set(order, perOrder);
  }
  const cached = perOrder.get(key);
  if (cached) return cached;

  let result = order;
  if (order.some((name) => matchesColumnQuery(name, terms))) {
    const pk = new Set(page.columns.filter((c) => c.isPrimaryKey).map((c) => c.name));
    result = order.filter((name) => pk.has(name) || matchesColumnQuery(name, terms));
  }
  perOrder.set(key, result);
  return result;
}

export interface ColumnFilterSummary {
  shown: number;
  total: number;
  noMatch: boolean;
}

/** Null when the query has no terms. */
export function columnFilterSummary(page: TabularPage, query: string): ColumnFilterSummary | null {
  const terms = columnQueryTerms(query);
  if (terms.length === 0) return null;
  const total = page.columns.length;
  const noMatch = !page.columns.some((c) => matchesColumnQuery(c.name, terms));
  return { shown: noMatch ? total : visibleColumnOrder(page, null, query).length, total, noMatch };
}
