// P18 D7/D8: candidate lists for FilterToolbar.vue's WHERE/ORDER BY boxes.
import type { Completion } from '@workbench/editor/fieldCompletion';
import { identNeedsQuoting, quoteIdent, type SqlDialect } from '../shared/sqlIdent';
import { getPage } from './page';
import { useGridViewStore } from './state';

// Curated, not exhaustive (ground rules) — a WHERE box has no use for CREATE/GRANT/VACUUM, and
// including the dialect word list's hundreds of entries would crowd out the column names that
// are the actual deliverable.
const WHERE_KEYWORDS: readonly string[] = [
  'AND',
  'OR',
  'NOT',
  'NULL',
  'IS NULL',
  'IS NOT NULL',
  'IN',
  'LIKE',
  'BETWEEN',
  'EXISTS',
  'TRUE',
  'FALSE',
];
const WHERE_KEYWORDS_POSTGRES: readonly string[] = ['ILIKE', 'SIMILAR TO'];

const ORDER_BY_KEYWORDS: readonly string[] = ['ASC', 'DESC'];
const ORDER_BY_KEYWORDS_POSTGRES: readonly string[] = ['NULLS FIRST', 'NULLS LAST'];

function columnCompletions(tabId: string): Completion[] {
  // meta.columns (runtime[tabId].meta, from loadMeta()'s L1-cached kira:tree:describe) is
  // authoritative — it includes columns hidden by the current projection, which the loaded
  // page's own columns does not (ColumnsMenu.vue reads it exactly this way). The page's columns
  // is the fallback for loadMeta()'s own deliberately silent failure path, so completion still
  // works on a connection whose describe() errored.
  const metaColumns = useGridViewStore().runtime[tabId]?.meta?.columns;
  if (metaColumns) {
    return metaColumns.map((c) => ({ label: c.name, detail: c.dataType, icon: 'symbol-field' }));
  }
  const pageColumns = getPage(tabId)?.columns ?? [];
  return pageColumns.map((c) => ({ label: c.name, detail: c.dataType, icon: 'symbol-field' }));
}

// A column needing quotes is inserted quoted, dialect-correctly; a plain lowercase, non-reserved
// one is inserted bare — auto-quoting every identifier would turn a `stat` + Tab into
// `"status"`, which is correct SQL but not what someone typing a bare word would expect.
function quotedIfNeeded(dialect: SqlDialect | undefined, c: Completion): Completion {
  if (!identNeedsQuoting(dialect, c.label)) return c;
  return { ...c, insert: quoteIdent(dialect, c.label) };
}

function keywordCompletions(keywords: readonly string[]): Completion[] {
  return keywords.map((label) => ({ label, detail: 'keyword', icon: 'symbol-keyword' }));
}

// Columns first, keywords after — a user types a field far more often than they type BETWEEN.
export function whereCandidates(tabId: string, dialect: SqlDialect | undefined): Completion[] {
  const keywords =
    dialect === 'postgres' ? [...WHERE_KEYWORDS, ...WHERE_KEYWORDS_POSTGRES] : WHERE_KEYWORDS;
  return [
    ...columnCompletions(tabId).map((c) => quotedIfNeeded(dialect, c)),
    ...keywordCompletions(keywords),
  ];
}

export function orderByCandidates(tabId: string, dialect: SqlDialect | undefined): Completion[] {
  const keywords =
    dialect === 'postgres'
      ? [...ORDER_BY_KEYWORDS, ...ORDER_BY_KEYWORDS_POSTGRES]
      : ORDER_BY_KEYWORDS;
  return [
    ...columnCompletions(tabId).map((c) => quotedIfNeeded(dialect, c)),
    ...keywordCompletions(keywords),
  ];
}
