import { z } from 'zod';

// P5 D5: the one spelling of the kind list — nodeKindSchema (below, for the tiers that actually
// validate untrusted input with it) and NODE_KIND_SET (decodePath/pathTail's own membership
// check, C5) both derive from this array rather than each carrying their own copy.
const NODE_KINDS = [
  'connection',
  'database',
  'schema',
  'table',
  'view',
  'matview',
  'function',
  'sequence',
  'column',
  'collection', // P8: mongo's table-equivalent
  'namespace', // P9: an intermediate ':'-delimited level in a redis key tree
  'key', // P9: a leaf redis key, opened as a key/value tab
  'topic', // P10: a kafka topic, opened as a stream tab
  'partition', // P10: a browse-only leaf under a kafka topic
  'consumerGroup', // P10: a browse-only, informational leaf under a kafka topic
  'queue', // P10: an sqs queue, opened as a stream tab
  'bucket', // P17: an s3 bucket — the root container, redis's 'database' equivalent
  'prefix', // P17: an intermediate '/'-delimited level in an s3 bucket, redis's 'namespace' equivalent
  'object', // P17: a leaf s3 object, opened as a key/value tab (redis's 'key' equivalent)
] as const;

export const nodeKindSchema = /*#__PURE__*/ z.enum(NODE_KINDS);
export type NodeKind = z.infer<typeof nodeKindSchema>;

// decodePath and pathTail run on every tree row (ProjectTree.vue's row building,
// MainView.vue's per-entry icon choice, SlickGridHost.vue's qualifiedName()) to answer a question
// that is membership in this fixed, small literal set — not validation of untrusted input, which is
// what nodeKindSchema (still exported, still used by the wire-boundary tiers that do validate
// untrusted data) is for. A `safeParse` there cost 1 443 ns/call (three segments) measured on this
// render path; a Set lookup is the same answer for orders of magnitude less.
const NODE_KIND_SET: ReadonlySet<string> = new Set(NODE_KINDS);

function isNodeKind(value: string): value is NodeKind {
  return NODE_KIND_SET.has(value);
}

export interface PathSegment {
  kind: NodeKind;
  name: string;
}

export interface NodePath {
  connectionId: string;
  segments: PathSegment[];
}

// 'schema:public/table:order%2Fitems' — the connection id is not part of the string (D6).
export function encodePath(segments: PathSegment[]): string {
  return segments.map((s) => `${s.kind}:${encodeURIComponent(s.name)}`).join('/');
}

export function decodePath(connectionId: string, encoded: string): NodePath {
  if (encoded === '') return { connectionId, segments: [] };
  const segments = encoded.split('/').map((raw) => {
    const sep = raw.indexOf(':');
    if (sep < 0) throw new Error(`malformed path segment: ${raw}`);
    const kind = raw.slice(0, sep);
    const name = raw.slice(sep + 1);
    if (!isNodeKind(kind)) throw new Error(`unknown node kind in path segment: ${kind}`);
    return { kind, name: decodeURIComponent(name) };
  });
  return { connectionId, segments };
}

export function pathParent(encoded: string): string | null {
  const idx = encoded.lastIndexOf('/');
  if (idx < 0) return encoded === '' ? null : '';
  return encoded.slice(0, idx);
}

export function pathTail(encoded: string): PathSegment | null {
  if (encoded === '') return null;
  const idx = encoded.lastIndexOf('/');
  const raw = idx < 0 ? encoded : encoded.slice(idx + 1);
  const sep = raw.indexOf(':');
  if (sep < 0) return null;
  const kind = raw.slice(0, sep);
  if (!isNodeKind(kind)) return null;
  return { kind, name: decodeURIComponent(raw.slice(sep + 1)) };
}

export const treeNodeSchema = /*#__PURE__*/ z.object({
  kind: nodeKindSchema,
  name: z.string(), // the raw identifier, used to build SQL and to copy
  path: z.string(), // encoded, relative to the connection
  hasChildren: z.boolean(),
  detail: z.string().optional(), // muted right-aligned text: type, row estimate, signature
  badges: /*#__PURE__*/ z.array(z.string()).optional(), // e.g. ['PK'], ['UNIQUE']
});
export type TreeNode = z.infer<typeof treeNodeSchema>;

export const columnMetaSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  position: z.number(),
  dataType: z.string(),
  nullable: z.boolean(),
  defaultExpr: z.string().nullable(),
  isPrimaryKey: z.boolean(),
  comment: z.string().nullable(),
});
export type ColumnMeta = z.infer<typeof columnMetaSchema>;

export const indexMetaSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  columns: /*#__PURE__*/ z.array(z.string()),
  unique: z.boolean(),
  primary: z.boolean(),
  partial: z.boolean().optional(), // WHERE predicate: uniqueness holds only inside it
  method: z.string().nullable(),
});
export type IndexMeta = z.infer<typeof indexMetaSchema>;

export const foreignKeyMetaSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  columns: /*#__PURE__*/ z.array(z.string()),
  referencedPath: z.string(), // encoded path of the referenced table (P7)
  referencedColumns: /*#__PURE__*/ z.array(z.string()),
  onDelete: z.string().nullable(),
  onUpdate: z.string().nullable(),
});
export type ForeignKeyMeta = z.infer<typeof foreignKeyMetaSchema>;

export const objectMetaSchema = /*#__PURE__*/ z.object({
  path: z.string(),
  kind: nodeKindSchema,
  name: z.string(),
  qualifiedName: z.string(),
  columns: /*#__PURE__*/ z.array(columnMetaSchema),
  primaryKey: /*#__PURE__*/ z.array(z.string()).nullable(),
  foreignKeys: /*#__PURE__*/ z.array(foreignKeyMetaSchema),
  referencedBy: /*#__PURE__*/ z.array(foreignKeyMetaSchema), // D17
  indexes: /*#__PURE__*/ z.array(indexMetaSchema),
  rowEstimate: z.number().nullable(),
  comment: z.string().nullable(),
});
export type ObjectMeta = z.infer<typeof objectMetaSchema>;

// SchemaColumns' own element — one relation's columns, reusing columnMetaSchema verbatim
// (never a second, parallel column shape) so a completion source built from this can never
// disagree with Describe about the same column's type. Deliberately smaller than ObjectMeta: no
// primaryKey/foreignKeys/referencedBy/indexes/rowEstimate/object-level comment — SchemaColumns
// exists to spell a column name and its type across a whole container in one round trip, not to
// fetch a schema's entire constraint graph.
export const relationColumnsSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  kind: nodeKindSchema,
  columns: /*#__PURE__*/ z.array(columnMetaSchema),
});
export type RelationColumns = z.infer<typeof relationColumnsSchema>;
