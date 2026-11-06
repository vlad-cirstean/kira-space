import { t } from '@theme/i18n';
import type { ConsoleHoverInfo } from '../../editor/hoverInfo';
import type { SqlDialect } from '../shared/sqlIdent';
import { type DdlColumn, type DdlSchema, type DdlTable, findTable } from './ddl';
import { childrenOf, isNameNode, type LNode, parseSql, unquotedName } from './sqlNodes';
import { statementsWithRefs } from './sqlRefs';

const MAX_TABLE_COLUMNS = 40;

// D8's own "resolves to nothing -> no tooltip at all" is what buildHoverSource's `null` return
// already gives (hover.ts) — every path below that can't resolve returns null, never an empty box.

function findLeafAt(root: LNode, pos: number): { node: LNode; parent: LNode | null } {
  let node = root;
  let parent: LNode | null = null;
  for (;;) {
    const child = childrenOf(node).find((c) => c.from <= pos && pos <= c.to);
    if (!child) return { node, parent };
    parent = node;
    node = child;
  }
}

function columnFlags(col: DdlColumn): string {
  const flags = [
    col.primaryKey && 'PRIMARY KEY',
    col.notNull && 'NOT NULL',
    col.unique && 'UNIQUE',
    // P12 round 1 finding #18: ddl.ts's own handleCreateIndex sets this and nothing consumed it —
    // the obvious home for it, alongside the other three declared-in-DDL flags this hover already
    // surfaces the same way.
    col.indexed && 'INDEXED',
  ].filter((f): f is string => !!f);
  return flags.join(', ');
}

function tableHoverInfo(table: DdlTable, from: number, to: number): ConsoleHoverInfo {
  const qualifiedName = table.schema ? `${table.schema}.${table.name}` : table.name;
  const lines = [
    table.isView ? t('studio.console.hover.view.label', { name: qualifiedName }) : qualifiedName,
  ];
  const shown = table.columns.slice(0, MAX_TABLE_COLUMNS);
  for (const col of shown) lines.push(`  ${col.name}  ${col.type}`);
  if (table.columns.length > shown.length) {
    lines.push(`  ${t('common.more.label', { count: table.columns.length - shown.length })}`);
  }
  return { from, to, lines };
}

function columnHoverInfo(
  table: DdlTable,
  col: DdlColumn,
  from: number,
  to: number,
): ConsoleHoverInfo {
  const lines = [`${table.name}.${col.name} — ${col.type}`];
  const flags = columnFlags(col);
  if (flags) lines.push(flags);
  if (col.description) lines.push(col.description);
  return { from, to, lines };
}

function resolveHover(
  dialect: SqlDialect,
  schema: DdlSchema,
  doc: string,
  pos: number,
): ConsoleHoverInfo | null {
  // Parsed once here, then handed to both statementsWithRefs() calls
  // below (its own `root` param) — each one would otherwise re-parse this exact same string, a
  // provably redundant second parse of text already parsed one line above.
  // `parseSql` calls `tokenizeSql` through `sql-tokens.ts`'s own memo setup
  // (keyed by
  // (dialect, source) reference) — several calls over the same untouched document string in one
  // interaction burst (a hover, then the lint pass, then another hover) share one real tokenize,
  // restoring the property `editor/hover.ts`'s old `syntaxTree(view.state)` handoff existed for
  // (measured up to ~51ms on a 191KB document, over budget with no debounce) without needing a
  // live EditorView to hand a tree through — this lookup is now a plain, pure function of
  // (doc, offset), no tree parameter to thread.
  const root = parseSql(dialect, doc);
  const { node, parent } = findLeafAt(root, pos);
  if (!isNameNode(node)) return null;

  const isQualifiedSegment = parent?.name === 'CompositeIdentifier';
  if (isQualifiedSegment) {
    const segs = childrenOf(parent as LNode).filter(isNameNode);
    const index = segs.findIndex((s) => s.from === node.from && s.to === node.to);
    if (index <= 0) {
      // The leading segment of a qualified name — e.g. hovering "public" in "public.users", or
      // "u" in "u.id" — resolve it as a table/alias, same as the unqualified case below.
    } else {
      const qualifierNode = segs[index - 1] as LNode;
      const qualifierText = unquotedName(qualifierNode, doc);
      const columnText = unquotedName(node, doc);
      const statement = statementsWithRefs(dialect, doc, root).find(
        (s) => s.statement.from <= pos && pos <= s.statement.to,
      );
      // P12 round 2 finding #7: a CTE shadowing a real DDL table name must resolve to nothing
      // here, not the base table it shadows — same "false positive worse than missing" rule
      // sqlDiagnostics.ts's own resolveAliasMap follows, applied to hover instead of a diagnostic.
      let table: DdlTable | undefined;
      if (statement) {
        const ref = statement.refs.find(
          (r) => (r.alias ?? r.name).toLowerCase() === qualifierText.toLowerCase(),
        );
        if (ref && !statement.cteNames.has(ref.name.toLowerCase())) {
          table = findTable(schema, ref.schema, ref.name);
        }
      }
      if (!table && !(statement?.cteNames.has(qualifierText.toLowerCase()) ?? false)) {
        table = findTable(schema, undefined, qualifierText);
      }
      if (!table) return null;
      const col = table.columns.find((c) => c.name.toLowerCase() === columnText.toLowerCase());
      if (!col) return null;
      return columnHoverInfo(table, col, node.from, node.to);
    }
  }

  const word = unquotedName(node, doc);
  const table = findTable(schema, undefined, word);
  if (table) return tableHoverInfo(table, node.from, node.to);

  // D8: an unqualified word that isn't a table — a column hover only when exactly one table
  // *referenced in this statement* declares it (never guessed across the whole schema).
  const statement = statementsWithRefs(dialect, doc, root).find(
    (s) => s.statement.from <= pos && pos <= s.statement.to,
  );
  if (!statement) return null;
  const referencedTables = statement.refs
    .map((r) => findTable(schema, r.schema, r.name))
    .filter((t): t is DdlTable => !!t);
  const matches = referencedTables.filter((t) =>
    t.columns.some((c) => c.name.toLowerCase() === word.toLowerCase()),
  );
  if (matches.length !== 1) return null;
  const onlyTable = matches[0] as DdlTable;
  const col = onlyTable.columns.find((c) => c.name.toLowerCase() === word.toLowerCase());
  if (!col) return null;
  return columnHoverInfo(onlyTable, col, node.from, node.to);
}

/** undefined with no DDL document for this connection — no hover source at all rather than
 *  one that never resolves anything. A plain `(doc, offset) => ConsoleHoverInfo | null` —
 *  `MonacoHost.vue`'s own hover provider plugs a lookup of this exact shape straight
 *  in, so the CodeMirror-only glue `editor/hover.ts`'s old `buildHoverSource` existed for has no
 *  equivalent to keep. */
export function sqlHoverSource(
  dialect: SqlDialect,
  schema: DdlSchema,
): ((doc: string, offset: number) => ConsoleHoverInfo | null) | undefined {
  if (schema.tables.length === 0) return undefined;
  return (doc, offset) => resolveHover(dialect, schema, doc, offset);
}
