import { t } from '@theme/i18n';
import type { ConsoleDiagnostic } from '../../editor/diagnostics';
import type { SqlDialect } from '../shared/sqlIdent';
import { type DdlSchema, type DdlTable, findTable } from './ddl';
import { childrenOf, type LNode, unquotedName } from './sqlNodes';
import { deepCompositeIdentifiers, statementsWithRefs, type TableRef } from './sqlRefs';

// Two diagnostics, both warnings, both bounded by what the DDL can prove —
// "a false positive is worse than a missing diagnostic". Deliberately not implemented:
// ambiguous-unqualified-column, which needs a real binder (USING, natural joins, lateral scopes)
// this app has no reason to build.

// P12 round 2 finding #7: a CTE named after a real DDL table must resolve to the CTE (unprovable
// from DDL alone, so "null" — no diagnostic, D7's own "false positive is worse than missing" rule)
// rather than silently falling through to the base table it shadows.
function resolveAliasMap(
  refs: readonly TableRef[],
  cteNames: ReadonlySet<string>,
  schema: DdlSchema,
): Map<string, DdlTable | null> {
  const map = new Map<string, DdlTable | null>();
  for (const ref of refs) {
    const table = cteNames.has(ref.name.toLowerCase())
      ? null
      : (findTable(schema, ref.schema, ref.name) ?? null);
    if (ref.alias) map.set(ref.alias.toLowerCase(), table);
    map.set(ref.name.toLowerCase(), table);
  }
  return map;
}

/** D7 rule 1: an identifier in a FROM/JOIN position that is not a known table, not a declared
 *  alias target, not a CTE name, and not a table-valued function call (sqlRefs.ts's readOneRef
 *  never emits a ref for one). */
function unknownRelationDiagnostics(
  refs: readonly TableRef[],
  cteNames: ReadonlySet<string>,
  schema: DdlSchema,
): ConsoleDiagnostic[] {
  const out: ConsoleDiagnostic[] = [];
  for (const ref of refs) {
    if (cteNames.has(ref.name.toLowerCase())) continue;
    if (findTable(schema, ref.schema, ref.name)) continue;
    const qualified = ref.schema ? `${ref.schema}.${ref.name}` : ref.name;
    out.push({
      from: ref.nodeFrom,
      to: ref.nodeFrom + qualified.length,
      severity: 'warning',
      message: t('studio.console.diagnostics.unknownTable.error', { table: qualified }),
    });
  }
  return out;
}

/** D7 rule 2: `alias.column`/`table.column` where the alias/table *is* known and the column is
 *  not — only qualified references, never a bare unqualified name (that needs a real binder). */
function unknownColumnDiagnostics(
  aliasMap: ReadonlyMap<string, DdlTable | null>,
  statementNode: LNode,
  consumedFroms: ReadonlySet<number>,
  source: string,
  cteNames: ReadonlySet<string>,
  schema: DdlSchema,
): ConsoleDiagnostic[] {
  const out: ConsoleDiagnostic[] = [];
  for (const node of deepCompositeIdentifiers(statementNode)) {
    if (consumedFroms.has(node.from)) continue;
    const segs = childrenOf(node).filter((n) => n.name !== '.');
    if (segs.length < 2) continue;
    const columnNode = segs[segs.length - 1];
    const qualifierNode = segs[segs.length - 2];
    if (!columnNode || !qualifierNode) continue;
    const qualifierText = unquotedName(qualifierNode, source);
    const columnText = unquotedName(columnNode, source);

    let table = aliasMap.get(qualifierText.toLowerCase());
    // P12 round 2 finding #7: same CTE-shadow guard as resolveAliasMap — this fallback path
    // resolves a qualifier the alias map doesn't carry (a table referenced without going through
    // a FROM/JOIN ref), so it needs its own check.
    if (table === undefined && !cteNames.has(qualifierText.toLowerCase())) {
      table = findTable(schema, undefined, qualifierText) ?? undefined;
    }
    if (!table) continue; // the qualifier itself doesn't resolve — rule 1's job, or not resolvable at all.

    if (table.columns.some((c) => c.name.toLowerCase() === columnText.toLowerCase())) continue;
    out.push({
      from: columnNode.from,
      to: columnNode.to,
      severity: 'warning',
      message: t('studio.console.diagnostics.unknownColumn.error', {
        table: table.name,
        column: columnText,
      }),
    });
  }
  return out;
}

/** DDL-driven diagnostics over `text` — empty when `schema` has no tables (no DDL document,
 *  no diagnostics beyond the lexical ones lintSql already gives). `root`: an already-parsed tree
 *  (same optional third argument sqlHover.ts's own statementsWithRefs calls take) — a fresh parse
 *  only happens when none is supplied. Currently always undefined at this module's one call site
 *  (lint.ts), which is deliberately debounced 400ms and not (yet) worth the CodeMirror-internals
 *  plumbing hover.ts's own redundant-parse fix needed. */
export function ddlDiagnostics(
  dialect: SqlDialect,
  text: string,
  schema: DdlSchema,
  root?: LNode,
): ConsoleDiagnostic[] {
  if (schema.tables.length === 0) return [];
  const out: ConsoleDiagnostic[] = [];
  for (const { statement, refs, cteNames } of statementsWithRefs(dialect, text, root)) {
    out.push(...unknownRelationDiagnostics(refs, cteNames, schema));
    const aliasMap = resolveAliasMap(refs, cteNames, schema);
    const consumedFroms = new Set(refs.map((r) => r.nodeFrom));
    out.push(
      ...unknownColumnDiagnostics(aliasMap, statement, consumedFroms, text, cteNames, schema),
    );
  }
  return out;
}
