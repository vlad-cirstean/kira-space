import { z } from 'zod';

export const opKindSchema = /*#__PURE__*/ z.enum([
  'connect',
  'disconnect',
  'children',
  'describe',
  'definition',
  'test',
  'read',
  'count',
  'mutate',
  'execute',
  // P33: a bulk-bytes file transfer (an S3 download) — distinct from 'read' so a multi-hundred-MB
  // transfer's op-log row is legible as a file transfer, not a mysteriously slow read.
  'transfer',
  // P2: an outbound HTTP request/response exchange (internal/httpclient), the op log's first
  // connectionless op kind — RunOp already tolerates a nil connection id end to end (F10), so
  // this joins the same op log rather than getting one of its own (D3).
  'http',
  // P11 D7: a gRPC unary or server-streaming call (internal/grpcclient) — the same connectionless
  // op shape 'http' already established, joining the same op log rather than a second scheduler.
  'grpc',
  // P108 Part 6 F3: Router.SchemaColumns' schema-wide column fetch (P22c) and Router.KeyTypes
  // (P63) — mirrors model.opKinds' own Go-side addition, both already reaching RunOp with these
  // kinds well before this schema recognized them.
  'schemaColumns',
  'keyTypes',
  // P307: the pre-connect script's startup wait, shown so its progress and timer are visible.
  'preconnect',
  // P440: a Kubernetes cluster call, from the module or MCP; connectionless like 'http'.
  'kubernetes',
]);
export type OpKind = z.infer<typeof opKindSchema>;

export const opStatusSchema = /*#__PURE__*/ z.enum(['running', 'ok', 'error', 'cancelled']);
export type OpStatus = z.infer<typeof opStatusSchema>;

// The app-agnostic op-log fields, shared by Kira Studio's DB op log and Kira Space's git op log
// — `kind` stays a bare string here since each app has its own closed vocabulary (opKindSchema
// below is Studio's). packages/workbench's OpLogPanel.vue and createOpLogStore are generic over
// this base; each app extends it with its own fields.
export const opLogRecordSchema = /*#__PURE__*/ z.object({
  id: z.string(),
  startedAt: z.string(),
  durationMs: z.number().nullable(),
  kind: z.string(),
  status: opStatusSchema,
  command: z.string().nullable(),
  error: z.string().nullable(),
});
export type OpLogRecord = z.infer<typeof opLogRecordSchema>;

// P328: who started the op — the user through the UI, or an MCP client's tool call.
export const opSourceSchema = /*#__PURE__*/ z.enum(['user', 'mcp']);
export type OpSource = z.infer<typeof opSourceSchema>;

export const opRecordSchema = /*#__PURE__*/ opLogRecordSchema.extend({
  connectionId: z.string().nullable(),
  tabId: z.string().nullable(),
  kind: opKindSchema,
  rows: z.number().nullable(),
  // Set when command was truncated at storage time (op_log's own 64 KiB per-row cap)
  // — no longer the whole script that ran, so Re-run must refuse rather than replay a prefix.
  // Optional: a record built before this field existed (or a hand-written test fixture) has no
  // opinion, which OperationsPanel.vue treats as "not truncated" via plain falsy access.
  commandTruncated: z.boolean().optional(),
  // P108 Part 11 F5: the encoded console path (database/schema) this op ran against — null for
  // every op kind that isn't a console execute() batch and for a pre-F5 row (op_log.path). Nullable
  // (Go's *string always marshals the key) and optional (a hand-written test fixture predating this
  // field has no opinion) — OperationsPanel.vue's re-run treats either as "no recorded path".
  path: z.string().nullable().optional(),
  // P307 Part 2: a preconnect op's configured wait in ms (live only, absent otherwise).
  delayMs: z.number().optional(),
  source: opSourceSchema,
});
export type OpRecord = z.infer<typeof opRecordSchema>;
