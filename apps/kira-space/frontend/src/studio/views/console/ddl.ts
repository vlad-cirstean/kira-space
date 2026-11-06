import type { RelationColumns } from '@shared/domain/tree';
import type { EditorCompletion } from '../../editor/completion';
import { identNeedsQuoting, quoteIdent, type SqlDialect } from '../shared/sqlIdent';
import {
  childrenOf,
  isKeyword,
  isNameNode,
  keywordText,
  type LNode,
  parseSql,
  splitComposite,
  TokenCursor,
  text,
  unquotedName,
} from './sqlNodes';

// D9/D10: the model the SQL language service (completion/diagnostics/hover) reads. Built once per
// (connectionId, DDL text) — see state/schemas.ts's memoisation — and never touches the network.
export interface DdlColumn {
  name: string;
  /** Taken as a raw source-text slice, never reconstructed from individual tokens —
   *  `sql-tokens.ts`'s scanner has no dedicated `Type` node kind at all (a declared column type is
   *  an ordinary run of `Identifier`/`Keyword`/`Parens` tokens), so collecting only "type-shaped"
   *  nodes would require guessing at exactly the vocabulary this comment used to worry about
   *  missing. Slicing verbatim between the column name and the first `TYPE_STOP_WORDS` token below
   *  sidesteps that entirely. */
  type: string;
  primaryKey?: boolean;
  notNull?: boolean;
  unique?: boolean;
  /** Set when a `CREATE INDEX` in the same document names this column — surfaced as an INDEXED
   *  flag on the column's hover tooltip (sqlHover.ts's columnFlags, P12 round 1 finding #18). */
  indexed?: boolean;
  references?: { table: string; column?: string };
  /** From a `COMMENT ON COLUMN … IS '…'` statement — the hover's description line (D9). */
  description?: string;
}

export interface DdlTable {
  /** Unqualified table (or view) name. */
  name: string;
  /** The schema (Postgres) or database (MySQL/MariaDB/ClickHouse) qualifier the DDL declared it
   *  with, if any — SQLite tables are never qualified. */
  schema?: string;
  isView?: boolean;
  /** Empty for a view (D9: "the name only, as a completable relation with no columns"). */
  columns: DdlColumn[];
}

export interface DdlSchema {
  tables: DdlTable[];
}

export const EMPTY_DDL_SCHEMA: DdlSchema = { tables: [] };

// Everything that can start a *table-level* constraint clause, not a column — `PRIMARY KEY
// (a, b)`, `KEY idx (x)`, `UNIQUE (x)`, `CONSTRAINT fk FOREIGN KEY … REFERENCES …`, a bare
// `FOREIGN KEY (…)`, `CHECK (…)`, MySQL's `INDEX`/`FULLTEXT`/`SPATIAL` — recognised by their
// leading keyword and consumed whole, never emitted as a phantom column (e.g. one named
// "PRIMARY").
const TABLE_CONSTRAINT_LEADING = new Set([
  'primary',
  'unique',
  'foreign',
  'constraint',
  'key',
  'check',
  'index',
  'fulltext',
  'spatial',
]);

// Where a column's declared-type slice ends. Matched by lowercased text against ANY of
// Keyword/Identifier/Null (not just Keyword) because a dialect's own curated keyword list can
// leave a constraint word un-classified — this repo's in-house ClickHouseDialect has no `default`
// in its keyword string (languages.ts), so `DEFAULT` there tokenises as a plain Identifier.
const TYPE_STOP_WORDS = new Set([
  'not',
  'primary',
  'key',
  'unique',
  'default',
  'references',
  'check',
  'collate',
  'generated',
  'auto_increment',
  'autoincrement',
  'constraint',
  'comment',
  'materialized',
  'alias',
  'codec',
  'ttl',
]);

function isTypeStop(node: LNode, source: string): boolean {
  if (node.name === 'Null') return true;
  if (node.name === 'Keyword' || node.name === 'Identifier') {
    return TYPE_STOP_WORDS.has(text(node, source).toLowerCase());
  }
  return false;
}

function tableKey(schema: string | undefined, name: string): string {
  return `${(schema ?? '').toLowerCase()}\0${name.toLowerCase()}`;
}

// Tracks tables two ways: by full qualified key (schema.name, when qualified) and by bare name
// (there can be several same-named tables across schemas — ALTER/CREATE INDEX/COMMENT ON COLUMN
// fall back to the first bare-name match when they don't repeat the schema qualifier).
class TableIndex {
  private readonly byQualified = new Map<string, DdlTable>();
  private readonly byBareName = new Map<string, DdlTable[]>();

  register(table: DdlTable, tables: DdlTable[]): void {
    const bareKey = table.name.toLowerCase();
    const list = this.byBareName.get(bareKey) ?? [];
    const existing = list.find(
      (t) => (t.schema ?? '').toLowerCase() === (table.schema ?? '').toLowerCase(),
    );
    if (existing) {
      tables[tables.indexOf(existing)] = table;
      list[list.indexOf(existing)] = table;
    } else {
      tables.push(table);
      list.push(table);
      this.byBareName.set(bareKey, list);
    }
    if (table.schema) this.byQualified.set(tableKey(table.schema, table.name), table);
  }

  lookup(schema: string | undefined, name: string): DdlTable | undefined {
    if (schema) {
      const exact = this.byQualified.get(tableKey(schema, name));
      if (exact) return exact;
    }
    return this.byBareName.get(name.toLowerCase())?.[0];
  }
}

interface QualifiedName {
  schema?: string;
  name: string;
}

function readQualifiedName(c: TokenCursor, source: string): QualifiedName | undefined {
  const t = c.peek();
  if (!t) return undefined;
  if (t.name === 'CompositeIdentifier') {
    c.next();
    const segs = splitComposite(t, source);
    if (segs.length === 0) return undefined;
    const name = segs[segs.length - 1] as string;
    return { name, schema: segs.length > 1 ? segs[segs.length - 2] : undefined };
  }
  if (isNameNode(t)) {
    c.next();
    return { name: unquotedName(t, source) };
  }
  return undefined;
}

// One constraint word (`NOT NULL`/`PRIMARY KEY`/`UNIQUE`/`REFERENCES …`) starting at `tail[i]`,
// applied onto `column` in place. Returns how many EXTRA tail slots it consumed (its own keyword
// token at `i` is always accounted for by the caller's own loop increment) — `NOT NULL`/
// `PRIMARY KEY` each consume one more token, `REFERENCES table(col)` doesn't need to (its own
// lookahead is bounded, never chained with another constraint word).
function applyConstraintWord(
  column: DdlColumn,
  tail: readonly LNode[],
  i: number,
  source: string,
): number {
  const t = tail[i] as LNode;
  const w = keywordText(t, source);
  if (w === undefined) return 0;
  if (w === 'not' && tail[i + 1]?.name === 'Null') {
    column.notNull = true;
    return 1;
  }
  if (w === 'primary' && isKeyword(tail[i + 1], 'key', source)) {
    column.primaryKey = true;
    return 1;
  }
  if (w === 'unique') {
    column.unique = true;
    return 0;
  }
  if (w === 'references') {
    const targetNode = tail[i + 1];
    const target =
      targetNode?.name === 'CompositeIdentifier'
        ? splitComposite(targetNode, source)
        : targetNode && isNameNode(targetNode)
          ? [unquotedName(targetNode, source)]
          : undefined;
    if (target && target.length > 0) {
      const refTable = target[target.length - 1] as string;
      const parensNode = tail[i + 2];
      const inner =
        parensNode?.name === 'Parens' ? childrenOf(parensNode).find(isNameNode) : undefined;
      column.references = {
        table: refTable,
        column: inner ? unquotedName(inner, source) : undefined,
      };
    }
    return 0;
  }
  return 0;
}

function parseColumnDef(segment: LNode[], source: string): DdlColumn | undefined {
  const nameNode = segment[0];
  if (!nameNode) return undefined;
  const name = unquotedName(nameNode, source);
  const rest = segment.slice(1);

  let typeEnd = 0;
  while (typeEnd < rest.length && !isTypeStop(rest[typeEnd] as LNode, source)) typeEnd++;
  if (typeEnd === 0) return undefined; // no type at all — can't model this column, drop it silently.
  const typeTokens = rest.slice(0, typeEnd);
  const first = typeTokens[0] as LNode;
  const last = typeTokens[typeTokens.length - 1] as LNode;
  const type = source.slice(first.from, last.to);

  const column: DdlColumn = { name, type };
  const tail = rest.slice(typeEnd);
  for (let i = 0; i < tail.length; i++) {
    i += applyConstraintWord(column, tail, i, source);
  }
  return column;
}

function parseColumnDefs(parens: LNode, source: string, table: DdlTable): void {
  const inner = childrenOf(parens).filter((n) => n.name !== '(' && n.name !== ')');
  const segments: LNode[][] = [];
  let seg: LNode[] = [];
  for (const n of inner) {
    if (n.name === 'Punctuation' && text(n, source) === ',') {
      segments.push(seg);
      seg = [];
    } else if (n.name === 'LineComment' || n.name === 'BlockComment') {
      // comments are noise between/within defs — dropped, matching F4's own tokenisation.
    } else {
      seg.push(n);
    }
  }
  if (seg.length > 0) segments.push(seg);

  for (const segment of segments) {
    const head = segment[0];
    if (!head) continue;
    if (head.name === 'Keyword' && TABLE_CONSTRAINT_LEADING.has(text(head, source).toLowerCase())) {
      continue; // D9: a table-level constraint, not a column.
    }
    if (!isNameNode(head)) continue; // can't identify a column name — skip this segment silently.
    const column = parseColumnDef(segment, source);
    if (column) table.columns.push(column);
  }
}

function handleCreateTable(
  c: TokenCursor,
  source: string,
  tables: DdlTable[],
  index: TableIndex,
): void {
  while (c.atKeyword('if') || c.atKeyword('not') || c.atKeyword('exists')) c.i++;
  const qname = readQualifiedName(c, source);
  if (!qname) return;
  const parens = c.peek();
  if (parens?.name !== 'Parens') return; // no column list — nothing this extractor can model.
  const table: DdlTable = { name: qname.name, schema: qname.schema, columns: [] };
  parseColumnDefs(parens, source, table);
  index.register(table, tables);
}

function handleCreateView(
  c: TokenCursor,
  source: string,
  tables: DdlTable[],
  index: TableIndex,
): void {
  while (c.atKeyword('if') || c.atKeyword('not') || c.atKeyword('exists')) c.i++;
  const qname = readQualifiedName(c, source);
  if (!qname) return;
  index.register({ name: qname.name, schema: qname.schema, columns: [], isView: true }, tables);
}

function handleCreateIndex(c: TokenCursor, source: string, index: TableIndex): void {
  const idxNameNode = c.peek();
  if (!idxNameNode || !isNameNode(idxNameNode)) return;
  c.next();
  if (!c.eatKeyword('on')) return;
  const qname = readQualifiedName(c, source);
  if (!qname) return;
  const table = index.lookup(qname.schema, qname.name);
  const parens = c.peek();
  if (!table || !parens || parens.name !== 'Parens') return;
  for (const n of childrenOf(parens)) {
    if (!isNameNode(n)) continue;
    const colName = unquotedName(n, source).toLowerCase();
    const col = table.columns.find((cc) => cc.name.toLowerCase() === colName);
    if (col) col.indexed = true;
  }
}

function handleAlterTable(c: TokenCursor, source: string, index: TableIndex): void {
  c.i++; // consume 'alter'
  if (!c.eatKeyword('table')) return;
  const qname = readQualifiedName(c, source);
  if (!qname) return;
  const table = index.lookup(qname.schema, qname.name);
  if (!table) return;
  if (!c.eatKeyword('add')) return;
  c.eatKeyword('column');
  const nameNode = c.peek();
  if (!nameNode || !isNameNode(nameNode)) return;
  c.next();
  const rest: LNode[] = [];
  while (!c.done()) rest.push(c.next() as LNode);
  const column = parseColumnDef([nameNode, ...rest], source);
  if (column) table.columns.push(column);
}

function handleCommentOnColumn(c: TokenCursor, source: string, index: TableIndex): void {
  c.i++; // consume 'comment'
  if (!c.eatKeyword('on')) return;
  if (!c.eatKeyword('column')) return;
  const target = c.peek();
  if (target?.name !== 'CompositeIdentifier') return;
  c.next();
  const segs = splitComposite(target, source);
  if (segs.length < 2) return;
  const colName = segs[segs.length - 1] as string;
  const tableName = segs[segs.length - 2] as string;
  const schemaName = segs.length > 2 ? segs[segs.length - 3] : undefined;
  const table = index.lookup(schemaName, tableName);
  if (!table) return;
  if (!c.eatKeyword('is')) return;
  const strNode = c.peek();
  if (strNode?.name !== 'String') return;
  const raw = text(strNode, source);
  const description = raw.slice(1, -1).replaceAll("''", "'");
  const col = table.columns.find((cc) => cc.name.toLowerCase() === colName.toLowerCase());
  if (col) col.description = description;
}

function handleStatement(stmt: LNode, source: string, tables: DdlTable[], index: TableIndex): void {
  const toks = childrenOf(stmt).filter((n) => n.name !== ';');
  const c = new TokenCursor(toks, source);
  if (c.eatKeyword('create')) {
    if (c.atKeyword('or') && c.atKeyword('replace', 1)) c.i += 2;
    c.eatKeyword('unique');
    if (c.eatKeyword('table')) {
      handleCreateTable(c, source, tables, index);
    } else if (c.eatKeyword('view')) {
      handleCreateView(c, source, tables, index);
    } else if (c.eatKeyword('index')) {
      handleCreateIndex(c, source, index);
    } // else: CREATE FUNCTION/MATERIALIZED VIEW/DICTIONARY/… (D9) — skipped silently.
    return;
  }
  if (c.atKeyword('alter')) {
    handleAlterTable(c, source, index);
  } else if (c.atKeyword('comment')) {
    handleCommentOnColumn(c, source, index);
  }
  // else: SET/GRANT/INSERT/… (D9) — a schema file is pasted, not authored; skipped silently.
}

/** Parses user-supplied DDL text with `dialect`'s own tokenizer into the table/column model the
 *  language service reads. Never throws — an unterminated or unrecognised statement simply
 *  contributes nothing (D9). */
export function parseDdl(dialect: SqlDialect, source: string): DdlSchema {
  const tables: DdlTable[] = [];
  const index = new TableIndex();
  const root = parseSql(dialect, source);
  for (const stmt of childrenOf(root)) {
    if (stmt.name === 'Statement') handleStatement(stmt, source, tables, index);
  }
  return { tables };
}

// SchemaNamespace replaces lang-sql's own SQLNamespace — the same recursive
// name -> (columns | nested namespace) object `toSqlNamespace` already built, typed for this app's
// own EditorCompletion (`editor/completion.ts`) instead of `@codemirror/autocomplete`'s Completion.
// `sqlSchemaCompletion.ts` is the one consumer.
export type SchemaNamespace = { [name: string]: SchemaNamespace | readonly EditorCompletion[] };

function columnCompletions(table: DdlTable): readonly EditorCompletion[] {
  return table.columns.map((col) => ({
    label: col.name,
    type: 'property',
    detail: col.type,
    // D10: primary-key columns sort first — `id` is the column most queries reach for.
    boost: col.primaryKey ? 1 : 0,
  }));
}

/** The SchemaNamespace `sqlSchemaCompletion.ts` consumes. A qualified table is emitted both
 *  nested (`schema.table`) and flat (`table`) — SchemaNamespace is a plain object, so duplicating a
 *  reference costs nothing, and it's what makes a bare table name complete even when the DDL
 *  qualified it. */
export function toSqlNamespace(schema: DdlSchema): SchemaNamespace {
  const ns: SchemaNamespace = {};
  for (const table of schema.tables) {
    const columns = columnCompletions(table);
    if (table.schema) {
      const existing = ns[table.schema];
      const schemaNs: SchemaNamespace =
        existing !== undefined && !Array.isArray(existing) ? (existing as SchemaNamespace) : {};
      schemaNs[table.name] = columns;
      ns[table.schema] = schemaNs;
    }
    ns[table.name] = columns;
  }
  return ns;
}

/** P22c D4: RelationColumns[] (the metadata cache's own supply, F5) -> the same SchemaNamespace
 *  shape toSqlNamespace produces from a parsed DdlSchema — so sqlSchemaCompletion.ts cannot tell
 *  which supply it got, and alias resolution / `table.` completion / qualified paths all work
 *  identically. Unqualified only: a schema-wide fetch is scoped to one container already, so there
 *  is no second schema/database level to nest under the way a DDL document's own `table.schema`
 *  can produce.
 *
 *  P4: labels stay bare (matching/highlighting wants the plain name); a column needing quotes
 *  (case-sensitive, a reserved word) gets `insert` set to its quoted form — the same
 *  identNeedsQuoting/quoteIdent call views/grid/filterCompletion.ts already makes for the data
 *  view's own filter box, so accepting `"order"` here and accepting it there produce the same
 *  bytes. Without this, a cache-fed completion (unlike the DDL-document branch, which was never in
 *  scope for this fix) inserted such a column bare and produced invalid SQL. */
export function namespaceFromCached(
  relations: readonly RelationColumns[],
  dialect: SqlDialect,
): SchemaNamespace {
  const ns: SchemaNamespace = {};
  for (const rc of relations) {
    ns[rc.name] = rc.columns.map((col) => {
      const completion: EditorCompletion = {
        label: col.name,
        type: 'property',
        detail: col.dataType,
        // D10's own precedent: primary-key columns sort first.
        boost: col.isPrimaryKey ? 1 : 0,
      };
      return identNeedsQuoting(dialect, col.name)
        ? { ...completion, insert: quoteIdent(dialect, col.name) }
        : completion;
    });
  }
  return ns;
}

/** Looks a table up by (optional) qualifier + name, case-insensitively — preferring an
 *  exact schema-qualified match, falling back to the first table with that bare name. Shared by
 *  the diagnostics and hover providers, which resolve a FROM-clause/alias reference against the
 *  same schema the completion source already offers. */
export function findTable(
  schema: DdlSchema,
  qualifier: string | undefined,
  name: string,
): DdlTable | undefined {
  const lowerName = name.toLowerCase();
  if (qualifier) {
    const lowerQualifier = qualifier.toLowerCase();
    const exact = schema.tables.find(
      (t) =>
        (t.schema ?? '').toLowerCase() === lowerQualifier && t.name.toLowerCase() === lowerName,
    );
    if (exact) return exact;
  }
  return schema.tables.find((t) => t.name.toLowerCase() === lowerName);
}

/** D10: `'public'` for Postgres, or the connection's own configured database for MySQL/MariaDB/
 *  ClickHouse — but only when the DDL actually qualifies at least one table with it, so passing
 *  a schema the document never used is a harmless no-op rather than a made-up default. */
export function defaultSchemaFor(
  schema: DdlSchema,
  dialect: SqlDialect,
  database: string | null | undefined,
): string | undefined {
  const candidate = dialect === 'postgres' ? 'public' : (database ?? undefined);
  if (!candidate) return undefined;
  const lower = candidate.toLowerCase();
  return schema.tables.some((t) => (t.schema ?? '').toLowerCase() === lower)
    ? candidate
    : undefined;
}
