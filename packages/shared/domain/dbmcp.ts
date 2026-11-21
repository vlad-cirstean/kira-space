import { z } from 'zod';
import { type ConnectionKind, ROW_ESTIMATE_KINDS } from './connection';
import { keptSiteSchema, keptStagedSchema, keptUseSchema } from './kept';

// M1 §6.2: the Database MCP section's own domain — bridge/dbmcp.go's DbMcpStatus/
// DbMcpInstallResult wire projections. command is the copy-paste registration text naming the
// headersHelper script path; it holds no secret.
export const dbMcpStatusSchema = /*#__PURE__*/ z.object({
  running: z.boolean(),
  command: z.string(),
  agentAvailable: z.boolean(),
  probed: z.array(z.string()),
  // The current token's own expiry, RFC 3339, "" when nothing is running or the record has not
  // yet been stamped.
  expiresAt: z.string(),
  error: z.string(),
});
export type DbMcpStatus = z.infer<typeof dbMcpStatusSchema>;

// "installed" | "notFound" | "installFailed" — mcpinstall.Install's own outcome vocabulary.
export const dbMcpInstallResultSchema = /*#__PURE__*/ z.object({
  outcome: /*#__PURE__*/ z.enum(['installed', 'notFound', 'installFailed']),
  detail: z.string(),
  probed: z.array(z.string()),
});
export type DbMcpInstallResult = z.infer<typeof dbMcpInstallResultSchema>;

// P520: the payloads of the dbmcp approval kinds (internal/dbmcp/approval_item.go). The common item
// fields (target, operation, origin) are approvals.ts's.
export const dbMcpApprovalPlanIssueSchema = /*#__PURE__*/ z.object({
  severity: /*#__PURE__*/ z.enum(['warn', 'info']),
  code: z.string(),
  message: z.string(),
});
export const dbMcpApprovalPlanSchema = /*#__PURE__*/ z.object({
  estimatedRowsRead: z.number().nullable(),
  thresholdRows: z.number(),
  overThreshold: z.boolean(),
  issues: z.array(dbMcpApprovalPlanIssueSchema),
  issuesOmitted: z.number(),
});
export type DbMcpApprovalPlan = z.infer<typeof dbMcpApprovalPlanSchema>;

export const dbMcpModeSchema = /*#__PURE__*/ z.enum(['read', 'write', 'ddl']);
export type DbMcpMode = z.infer<typeof dbMcpModeSchema>;
// The mode an approval payload names: a run_query mode, or masked for a browse_table read.
export const dbMcpPayloadModeSchema = /*#__PURE__*/ z.enum(['read', 'write', 'ddl', 'masked']);
export type DbMcpEnforcement = 'session' | 'app' | 'none';

// Mirror of internal/dbmcp/permissions.go: writeConsoleKinds, enforcementOf and accessOf. The flow
// test's list_connections contract pins both sides.
const WRITE_CONSOLE_KINDS: ReadonlySet<string> = /*#__PURE__*/ new Set([
  'postgres',
  'mysql',
  'mariadb',
  'sqlite',
  'clickhouse',
  'mongodb',
  'redis',
]);
export function hasWriteConsole(kind: string): boolean {
  return WRITE_CONSOLE_KINDS.has(kind);
}

export function enforcementOf(kind: string): DbMcpEnforcement {
  if (['postgres', 'mysql', 'mariadb', 'sqlite', 'clickhouse'].includes(kind)) return 'session';
  if (kind === 'mongodb' || kind === 'redis') return 'app';
  return 'none';
}

// Mirror of internal/dbmcp/timeout.go timeoutEnforcementOf: what stops a statement past the
// connection's statement timeout, per mode. The flow test's list_connections contract pins both sides.
export function statementTimeoutEnforcementOf(kind: string): Record<DbMcpMode, DbMcpEnforcement> {
  const all = (e: DbMcpEnforcement): Record<DbMcpMode, DbMcpEnforcement> => ({
    read: e,
    write: e,
    ddl: e,
  });
  switch (kind) {
    case 'postgres':
      return all('session');
    case 'mysql':
    case 'clickhouse':
      return { read: 'session', write: 'app', ddl: 'app' };
    case 'mariadb':
      return { read: 'session', write: 'session', ddl: 'app' };
    case 'sqlite':
    case 'mongodb':
    case 'redis':
      return all('app');
    case 'kibana':
      return { read: 'app', write: 'none', ddl: 'none' };
    default:
      return all('none');
  }
}

export type ExpensiveQueryCheck = 'plan' | 'off' | 'none';

// Mirror of internal/dbmcp/expensive.go expensiveCheckOf: none (no row estimate), off (the connection's
// MCP auto_explain switch is off) or plan. The flow test's list_connections contract pins both sides.
export function expensiveQueryCheckOf(kind: string, autoExplain: boolean): ExpensiveQueryCheck {
  if (!ROW_ESTIMATE_KINDS.has(kind as ConnectionKind)) return 'none';
  return autoExplain ? 'plan' : 'off';
}

export interface DbMcpAccessInput {
  kind: string;
  readOnly: boolean;
  read: string;
  write: string;
  ddl: string;
}

// The summary, e.g. "read, write (approval)", or "none".
export function accessSummaryOf(c: DbMcpAccessInput): string {
  const parts: string[] = [];
  for (const mode of dbMcpModeSchema.options) {
    const configured = c[mode];
    const blocked = mode !== 'read' && (c.readOnly || !hasWriteConsole(c.kind));
    if (blocked || (configured !== 'allow' && configured !== 'prompt')) continue;
    parts.push(configured === 'prompt' ? `${mode} (approval)` : mode);
  }
  return parts.length > 0 ? parts.join(', ') : 'none';
}

// The result script of a run_query item (P680): the source the user approves, its hash and the kept names it
// may write. Also the transform of an api.request item.
const transformViewSchema = /*#__PURE__*/ z.object({
  source: z.string(),
  hash: z.string(),
  // Kept variables the script may write (P678), with their calls in the source.
  mayKeep: z
    .array(
      z.object({
        name: z.string(),
        secret: z.boolean().default(false),
        sites: z.array(keptSiteSchema),
      }),
    )
    .nullish()
    .transform((v) => v ?? []),
});
export type TransformView = z.infer<typeof transformViewSchema>;

// Kept variables bound to the statement: the names as the agent listed them, how each placeholder shows,
// and the variables as they stood when the statement was checked; the result script, if any (P680).
// Older items carry none.
const dbKeptShape = {
  params: z.array(z.string()).default([]),
  placeholders: z.array(z.string()).default([]),
  kept: z.array(keptUseSchema).default([]),
  script: transformViewSchema.nullish().transform((v) => v ?? null),
};

export const dbQueryPayloadSchema = /*#__PURE__*/ z.object({
  connectionId: z.string(),
  statement: z.string(),
  // Declared mode, what the statement looks like, and (read items) what backs mode read.
  mode: dbMcpPayloadModeSchema,
  detected: z.string(),
  enforcement: z.string(),
  // A plan request: the statement is the agent SQL, shown as an explain.
  explain: z.boolean().default(false),
  plan: dbMcpApprovalPlanSchema.nullable(),
  // The item offers "review the result"; reviewResult says the connection's settings already do.
  reviewOffered: z.boolean().default(false),
  reviewResult: z.boolean().default(false),
  ...dbKeptShape,
});
export type DbQueryPayload = z.infer<typeof dbQueryPayloadSchema>;

export const dbMcpLimitHitSchema = /*#__PURE__*/ z.object({
  limit: z.string(),
  measured: z.number(),
  max: z.number(),
  unit: z.enum(['ms', 'rows']),
});
export type DbMcpLimitHit = z.infer<typeof dbMcpLimitHitSchema>;

export const dbExemptionPayloadSchema = /*#__PURE__*/ z.object({
  connectionId: z.string(),
  statement: z.string(),
  mode: dbMcpPayloadModeSchema,
  detected: z.string(),
  enforcement: z.string(),
  reason: z.string(),
  limits: z.array(dbMcpLimitHitSchema).min(1),
  plan: dbMcpApprovalPlanSchema.nullable(),
  reviewOffered: z.boolean().default(false),
  reviewResult: z.boolean().default(false),
  ...dbKeptShape,
});
export type DbExemptionPayload = z.infer<typeof dbExemptionPayloadSchema>;

export const dbResponsePayloadSchema = /*#__PURE__*/ z.object({
  connectionId: z.string(),
  statement: z.string(),
  isError: z.boolean(),
  bytes: z.number(),
  alreadyRan: z.boolean(),
  // The declared mode; requested says the approver asked for this review rather than a setting.
  mode: dbMcpPayloadModeSchema.optional(),
  requested: z.boolean().default(false),
  // A script produced the held text from the raw result (P680); rawBytes sizes that raw result.
  scripted: z.boolean().default(false),
  rawBytes: z.number().default(0),
});
export type DbResponsePayload = z.infer<typeof dbResponsePayloadSchema>;

export const kubeReadPayloadSchema = /*#__PURE__*/ z.object({
  context: z.string(),
  server: z.string(),
  namespace: z.string(),
  operation: z.enum(['discover', 'list', 'describe', 'logs']),
  statement: z.string(),
});
export type KubeReadPayload = z.infer<typeof kubeReadPayloadSchema>;

// One param or env entry of a script request. A secret's value is always empty; saved says its value
// comes from the script's saved secret values.
export const scriptValueSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  value: z.string(),
  secret: z.boolean(),
  saved: z.boolean(),
  fromVar: z.string(),
});
export type ScriptValue = z.infer<typeof scriptValueSchema>;

// script.run item payload (dbmcp scriptPayload).
export const scriptRunPayloadSchema = /*#__PURE__*/ z.object({
  scriptId: z.string(),
  description: z.string(),
  command: z.string(),
  cwd: z.string(),
  params: z.array(scriptValueSchema),
  env: z.array(scriptValueSchema),
});
export type ScriptRunPayload = z.infer<typeof scriptRunPayloadSchema>;

// api.request item payload (dbmcp apiPayload). Values are as the agent sent them: a credential stays
// {{name}}, and a secret variable's value is "••••".
const apiPairSchema = /*#__PURE__*/ z.object({ name: z.string(), value: z.string() });
// Scripts an agent attached to an api.request (P676): the source the user approves, with its hash.
const apiScriptPayloadSchema = /*#__PURE__*/ z.object({
  prepare: z
    .object({
      source: z.string(),
      hash: z.string(),
      changed: z.array(z.string()),
      input: z.string(),
      inputTruncated: z.boolean(),
    })
    .nullable(),
  transform: transformViewSchema.nullable(),
});
// A declared input of an api.request item (dbmcp apiInputPayload). Never a value.
export const apiInputSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  label: z.string(),
  state: z.enum(['needed', 'stored']),
  origin: z.string(),
  keptId: z.string().optional(),
  // The choice the card starts on (the agent's reuse hint) and what "keep" holds the value for.
  mode: z.enum(['once', 'keep']).default('once'),
  keepMinutes: z.number().default(60),
});
export type ApiInput = z.infer<typeof apiInputSchema>;

export const apiRequestPayloadSchema = /*#__PURE__*/ z.object({
  method: z.string(),
  url: z.string(),
  host: z.string(),
  environment: z.object({ id: z.string(), name: z.string(), color: z.string() }).nullable(),
  query: z.array(apiPairSchema),
  headers: z.array(apiPairSchema.extend({ masked: z.boolean() })),
  auth: z.object({ scheme: z.string(), display: z.string() }).nullable(),
  body: z
    .object({
      contentType: z.string(),
      text: z.string(),
      truncated: z.boolean(),
      form: z.array(apiPairSchema).nullable(),
    })
    .nullable(),
  variables: z.array(z.object({ name: z.string(), value: z.string(), secret: z.boolean() })),
  credentials: z.array(z.object({ name: z.string(), hosts: z.array(z.string()) })),
  followRedirects: z.boolean(),
  timeoutSeconds: z.number(),
  // Kept variables the request uses (P678), with where each is read.
  kept: z
    .array(keptUseSchema)
    .nullish()
    .transform((v) => v ?? []),
  // Values the agent asked the user for (P679): typed on the card (needed) or already held (stored).
  inputs: z.array(apiInputSchema).default([]),
  script: apiScriptPayloadSchema.nullish().transform((v) => v ?? null),
  // "Review the response" on the card (P677): offered, and already on by settings.
  reviewOffered: z.boolean().default(false),
  reviewResult: z.boolean().default(false),
});
export type ApiRequestPayload = z.infer<typeof apiRequestPayloadSchema>;

// The review fields every pre-run kind carries (db.query, db.exemption, api.request); the db kinds add a mode.
export const reviewOptionPayloadSchema = /*#__PURE__*/ z.object({
  reviewOffered: z.boolean().default(false),
  reviewResult: z.boolean().default(false),
  mode: z.string().default(''),
});

// api.response item payload (dbmcp apiResponsePayload): no body, text, raw or script; those come from
// the pending-response query.
export const apiResponsePayloadSchema = /*#__PURE__*/ z.object({
  method: z.string(),
  url: z.string(),
  host: z.string(),
  environment: z.object({ id: z.string(), name: z.string(), color: z.string() }).nullable(),
  status: z.number(),
  statusText: z.string(),
  isError: z.boolean(),
  scripted: z.boolean(),
  bytes: z.number(),
  rawBytes: z.number(),
  class: z.enum(['read', 'write']),
  reason: z.string(),
  // Secret variables the request masker hid in the held text, by name.
  masked: z.array(z.string()).default([]),
});
export type ApiResponsePayload = z.infer<typeof apiResponsePayloadSchema>;

// agentapi.Result as the review shows it: the masked response a transform read.
export const agentApiResultSchema = /*#__PURE__*/ z.object({
  status: z.number(),
  statusText: z.string().default(''),
  headers: z
    .array(apiPairSchema)
    .nullish()
    .transform((v) => v ?? []),
  contentType: z.string().default(''),
  body: z.string().default(''),
  binary: z.boolean().default(false),
  bodyBytes: z.number().default(0),
  truncated: z.boolean().default(false),
  finalUrl: z.string().default(''),
  redirects: z
    .array(z.object({ status: z.number(), url: z.string() }))
    .nullish()
    .transform((v) => v ?? []),
});
export type AgentApiResult = z.infer<typeof agentApiResultSchema>;

// bridge/dbmcp.go's DbMcpPendingResponse: the exact text the AI client receives if the user sends.
export const dbMcpPendingResponseSchema = /*#__PURE__*/ z.object({
  requestId: z.string(),
  text: z.string(),
  isError: z.boolean(),
  // The response a script read, and the script that turned it into text (P677); empty without one.
  raw: z.string().default(''),
  script: z
    .object({ source: z.string(), hash: z.string() })
    .nullish()
    .transform((v) => v ?? null),
  // Values the script keeps once the result is sent (P678).
  keep: z
    .array(keptStagedSchema)
    .nullish()
    .transform((v) => v ?? []),
});
export type DbMcpPendingResponse = z.infer<typeof dbMcpPendingResponseSchema>;

// run_query's JSON result as the AI client receives it: internal/dbmcp/render.go's tabularResult,
// documentResult, keyValueResult, streamResult and the "empty" object. A parser for the response
// preview, not a second renderer; unknown fields are dropped, never re-serialised.
const nullableText = /*#__PURE__*/ z.string().nullable();
const runQueryEnvelope = {
  rowCount: z.number(),
  returned: z.number(),
  truncated: z.boolean().optional(),
  plan: z.unknown().optional(),
  additionalStatementResults: z.number().optional(),
  note: z.string().optional(),
};
export const dbMcpRunQueryResultSchema = /*#__PURE__*/ z.discriminatedUnion('kind', [
  z.object({
    ...runQueryEnvelope,
    kind: z.literal('tabular'),
    columns: z.array(z.object({ name: z.string(), dataType: z.string(), typeClass: z.string() })),
    rows: z.array(z.array(nullableText)),
    truncatedCells: z.number(),
  }),
  z.object({
    ...runQueryEnvelope,
    kind: z.literal('document'),
    documents: z.array(z.object({ id: nullableText, body: nullableText })),
  }),
  z.object({
    ...runQueryEnvelope,
    kind: z.literal('keyvalue'),
    redisType: z.string(),
    ttlMs: z.number().optional(),
    memoryBytes: z.number().optional(),
    entries: z.array(z.object({ field: nullableText, value: nullableText })),
  }),
  z.object({
    ...runQueryEnvelope,
    kind: z.literal('stream'),
    messages: z.array(
      z.object({
        key: nullableText,
        timestamp: nullableText,
        headers: nullableText,
        attrs: nullableText,
        body: nullableText,
      }),
    ),
  }),
  z.object({ ...runQueryEnvelope, kind: z.literal('empty') }),
]);
export type DbMcpRunQueryResult = z.infer<typeof dbMcpRunQueryResultSchema>;

// P535: kinds with a structured browse (internal/dbmcp browseKinds).
export const BROWSE_KINDS: ReadonlySet<string> = /*#__PURE__*/ new Set([
  'postgres',
  'mysql',
  'mariadb',
  'sqlite',
  'clickhouse',
]);

// P532: kinds the per-connection table allow list applies to (internal/tableref Levels > 0).
export const TABLE_LIST_KINDS: ReadonlySet<string> = /*#__PURE__*/ new Set([
  'postgres',
  'mysql',
  'mariadb',
  'clickhouse',
  'mongodb',
  'sqlite',
  'kibana',
]);

// bridge/dbmcp_tables.go's DbMcpService.Tables: the live relations the MCP tab can list.
export const dbMcpTablesListingSchema = /*#__PURE__*/ z.object({
  tables: /*#__PURE__*/ z.array(/*#__PURE__*/ z.object({ ref: z.string(), kind: z.string() })),
  truncated: z.boolean(),
});
export type DbMcpTablesListing = z.infer<typeof dbMcpTablesListingSchema>;
