import { t } from '@theme/i18n';
import { z } from 'zod';
import { capsSchema } from '../caps';
import { PALETTE_COLOR_CHOICES, type PaletteColor, paletteColorSchema } from './color';

export const connectionKindSchema = /*#__PURE__*/ z.enum([
  'postgres',
  'mariadb',
  'mysql',
  'sqlite',
  'clickhouse',
  'mongodb',
  'redis',
  'kafka',
  'sqs',
  's3',
  'kibana',
]); // all v1 kinds; every one has an adapter as of P36 (rabbitmq dropped, see P58 findings)
export type ConnectionKind = z.infer<typeof connectionKindSchema>;

// The connection dialog's default port per kind (D27's "kind-driven default port", not a
// second hardcoded number per adapter). Kinds with no conventional default port are absent.
export const DEFAULT_PORT: Partial<Record<ConnectionKind, number>> = {
  postgres: 5432,
  mariadb: 3306,
  mysql: 3306,
  // P36 D10: the HTTP interface, not the native protocol's 9000 — the only port the app's driver
  // (@clickhouse/client, HTTP-only) can ever speak to.
  clickhouse: 8123,
  mongodb: 27017,
  redis: 6379,
  kafka: 9092,
  kibana: 5601,
};

// The minimum server version each adapter's client library (and this app's own catalog SQL)
// supports — the verified floor of the on-demand compatibility suite, not a marketing claim.
// SQS/S3 have no server version at all (AWS's APIs are versionless), so they have no entry.
// Getters, not values: the text resolves through the catalog when read, never at module load.
export const MIN_SERVER_VERSION: Partial<Record<ConnectionKind, string>> = {
  get postgres() {
    return t('shared.connection.minVersion.postgres.hint');
  },
  get mariadb() {
    return t('shared.connection.minVersion.mariadb.hint');
  },
  get mysql() {
    return t('shared.connection.minVersion.mysql.hint');
  },
  get clickhouse() {
    return t('shared.connection.minVersion.clickhouse.hint');
  },
  get mongodb() {
    return t('shared.connection.minVersion.mongodb.hint');
  },
  get redis() {
    return t('shared.connection.minVersion.redis.hint');
  },
  get kafka() {
    return t('shared.connection.minVersion.kafka.hint');
  },
  get kibana() {
    return t('studio.kibana.minVersion.hint');
  },
  get sqlite() {
    return t('shared.connection.minVersion.sqlite.hint');
  },
};

// P28 §5.3: 0 (unlimited) or 0.01-1000 commands/sec — a token-bucket rate, not a minimum
// interval, so a user reacting to "keep it under N/s" types the same number the server told them.
export const CONNECTION_THROTTLE_RANGE = { min: 0.01, max: 1000 } as const;

// P307 Part 2: whole seconds; 0 = off. 300 covers a browser-login tunnel; longer means run it by hand.
export const PRECONNECT_DELAY_RANGE = { min: 0, max: 300 } as const;

// Promoted to domain/color.ts as paletteColorSchema/PaletteColor/PALETTE_COLOR_CHOICES —
// one palette *by construction* now that an environment's colour draws from the same set. These
// three names stay as aliases so nothing in Studio (or its call sites' comments) has to change.
export const connectionColorSchema = paletteColorSchema; // D18; matches --kira-conn-* in
// tokens.css. 'none' is a real, stored value (the design system's own default — "no colour is the
// default, the rail slot stays reserved either way") rather than the field being nullable, so no
// DB/schema change is needed to add it.
export type ConnectionColor = PaletteColor;

/** The *offered* subset, not the storable one — `connectionColorSchema` above stays
 *  whole on purpose (a connection saved with a retired colour must keep parsing, listing and
 *  painting its own rail, or "trim the palette" silently deletes connections on next launch).
 *  Six hues chosen for a 42° minimum adjacent OKLCH hue gap at the app's one fixed
 *  lightness/chroma (`oklch(0.72 0.09 h)`) — roughly double the full eleven-hue ring's own worst
 *  gap (25.6°, blue↔indigo), which is what makes a 2px tab rail or a 5px status dot legible at
 *  all. Retired from the picker: `orange`, `olive`, `teal`, `indigo`, `violet`. */
export const CONNECTION_COLOR_CHOICES = PALETTE_COLOR_CHOICES;

export const connectionModeSchema = /*#__PURE__*/ z.enum(['fields', 'uri']);
export type ConnectionMode = z.infer<typeof connectionModeSchema>;

// M2: a connection's per-operation MCP permission — deny/allow/prompt, mirroring the Go side's
// model.ValidMcpPermissionMode.
export const mcpPermissionModeSchema = /*#__PURE__*/ z.enum(['deny', 'allow', 'prompt']);
export type McpPermissionMode = z.infer<typeof mcpPermissionModeSchema>;

// mirrors model.ValidMcpReviewResults
export const mcpReviewResultsSchema = /*#__PURE__*/ z.enum(['inherit', 'review', 'send']);
export type McpReviewResults = z.infer<typeof mcpReviewResultsSchema>;

// P529: MCP statement timeout bounds; mirrors model.McpStatementTimeoutDefault/Max.
export const CONNECTION_MCP_STATEMENT_TIMEOUT = { default: 10, max: 3600 } as const;

// The plain object shape, with no refinement — kept separate so both connectionInputSchema
// (which adds the fields/uri superRefine below) and connectionSummarySchema (which cannot
// .omit() from a refined schema) can each build off it independently.
const connectionFieldsSchema = /*#__PURE__*/ z.object({
  name: z.string().trim().min(1).max(120),
  kind: connectionKindSchema,
  color: connectionColorSchema,
  mode: connectionModeSchema,
  readOnly: z.boolean(),
  host: z.string().trim().nullable(),
  port: z.number().int().min(1).max(65535).nullable(),
  database: z.string().nullable(),
  username: z.string().nullable(),
  password: z.string().nullable(), // present on the way IN only; never on the way OUT (D9)
  // Input: null = unchanged (URI mode, masked and never revealed), blank is refused, else replaces.
  // Never carries a value on the way out (P181) — see connectionSummarySchema.
  uri: z.string().nullable(),
  options: /*#__PURE__*/ z.record(z.string(), z.unknown()),
  // P11: optional shell command run before connect (e.g. a port-forward). A first-class column
  // rather than an options_json key — options round-trips through the connection URI and the
  // Copy URI menu item, and a shell command must never be settable by pasting a URI.
  preconnect: z.string().trim().min(1).max(2000).nullable().default(null),
  // Overrides the settle-window auto-detection with an explicit per-connection choice. false
  // (default) = "run each time it tries to connect" — a fresh instance is spawned on every connect
  // attempt and its exit is never monitored, whether or not it happens to still be alive at the
  // settle window. true = "run once, and disconnect the db when it dies" — always arm() once the
  // adapter connects, regardless of what the settle-window race resolved to (a no-op if the script
  // already exited, since there's nothing left to monitor).
  preconnectSidecar: z.boolean().default(false),
  // P307 Part 2: seconds between starting the script and the first connect; 0 = off. `.default(0)`
  // is load-bearing for an older stored row, like throttlePerSec's.
  preconnectDelaySec: z.number().int().min(0).max(PRECONNECT_DELAY_RANGE.max).default(0),
  // Runs the connection's own EXPLAIN before every SELECT a console run issues on
  // it, warning (never blocking) when the threshold or a structural issue fires. A first-class
  // column rather than an options_json key — the same reason preconnect above is one: `options`
  // round-trips through the connection URI and the Copy URI menu item, and a behaviour that issues
  // an extra statement per run must not be switchable on by pasting a URI.
  autoExplain: z.boolean().default(false),
  // P28 §5.3: commands/sec this connection is paced to; 0 = unlimited. `.default(0)` is
  // load-bearing the same way preconnect's and autoExplain's are — an older stored row has no
  // such key. The real {0} ∪ [0.01, 1000] bound (a value in (0, 0.01) makes no practical sense
  // and is rejected) lives in the Go input validator (connections/input.go's Validate) and the
  // dialog's own field error, matching every other numeric field's split between "this schema
  // parses a stored row" and "Save enforces the real bound".
  throttlePerSec: z.number().min(0).max(CONNECTION_THROTTLE_RANGE.max).default(0),
  // Whether this connection is exposed to the local DB MCP server. Deny by default.
  // `.default(false)` is load-bearing the same way autoExplain's/throttlePerSec's are — an older
  // stored row has no such key. A first-class column, not an options_json key — the migration's
  // own comment carries the full argument (an access grant must not be settable by pasting a URI).
  mcpEnabled: z.boolean().default(false),
  // Runs this connection's own EXPLAIN before every explainable run_query on the DB MCP path,
  // and pauses the query for a human decision when the plan crosses the expensive-query row
  // threshold. `.default(true)` is load-bearing the same way mcpEnabled's is — an older stored row
  // has no such key. Separate from autoExplain (above), which governs this app's own console — see
  // the migration's own comment for why one switch cannot mean both.
  mcpAutoExplain: z.boolean().default(true),
  // P536: result review override per mode; inherit follows dbMcp.previewResponses. Default is load-bearing
  // for older payloads.
  mcpReview: z
    .object({
      read: mcpReviewResultsSchema,
      write: mcpReviewResultsSchema,
      ddl: mcpReviewResultsSchema,
    })
    .default({ read: 'inherit', write: 'inherit', ddl: 'inherit' }),
  // P529: seconds each agent statement may run, 0 off. Default is load-bearing for older rows.
  mcpStatementTimeoutSec: z
    .number()
    .int()
    .min(0)
    .max(CONNECTION_MCP_STATEMENT_TIMEOUT.max)
    .default(CONNECTION_MCP_STATEMENT_TIMEOUT.default),
  // P532: tables MCP statements may touch (tableref entries). null = all, including later ones; [] = none.
  mcpTables: z.array(z.string()).nullable().default(null),
  // P534: column review for masking is on; the reviewed list lives in the connection file.
  maskReviewOn: z.boolean().default(false),
  // M2: free-text "what this DB is for", passed to an AI client as connection metadata
  // (list_connections). Never interpreted by this app.
  mcpDescription: z.string().max(1000).default(''),
  // M2: per-operation MCP permission, one of deny/allow/prompt. `.default` is load-bearing the
  // same way mcpEnabled's is — an older stored row has no such key. Defaults tighten what M1
  // shipped (an exposed connection could previously be written to): read allow, write prompt, DDL
  // deny.
  mcpReadMode: mcpPermissionModeSchema.default('allow'),
  mcpWriteMode: mcpPermissionModeSchema.default('prompt'),
  mcpDdlMode: mcpPermissionModeSchema.default('deny'),
});

// SQS and S3 have no host/port at all (P10's D8, P17's own D8/D9 mirror) — fields mode repurposes
// `database` for the AWS region and `username` for the named profile instead, per §5.1's "named
// AWS profile" wording.
export const AWS_STYLE_KINDS: ReadonlySet<ConnectionKind> = new Set(['sqs', 's3']);

// Kinds whose "connection" is a local file path, not a network endpoint (P35 D10/D11). Fields
// mode repurposes `database` for the absolute path; host/port/username/password are unused.
export const FILE_KINDS: ReadonlySet<ConnectionKind> = new Set(['sqlite']);

// The five kinds that have an EXPLAIN this app can parse — frontend/src/views/console/
// explain.ts's explainStatementsFor switch is the real authority (its own comment cross-references
// this constant); this copy exists so project/ConnectionDialog.vue's own auto-explain checkbox can
// read it without project/ importing views/ directly.
export const EXPLAIN_SUPPORTED_KINDS: ReadonlySet<ConnectionKind> = new Set([
  'postgres',
  'mysql',
  'mariadb',
  'sqlite',
  'clickhouse',
]);

// Kinds whose plan carries an estimated rows read: EXPLAIN_SUPPORTED_KINDS minus sqlite, whose
// EXPLAIN QUERY PLAN has none. Mirror of internal/queryplan/statements.go HasRowEstimate.
export const ROW_ESTIMATE_KINDS: ReadonlySet<ConnectionKind> = new Set([
  'postgres',
  'mysql',
  'mariadb',
  'clickhouse',
]);

export const connectionInputSchema = connectionFieldsSchema.superRefine((input, ctx) => {
  if (input.mode === 'fields') {
    if (FILE_KINDS.has(input.kind)) {
      const path = input.database?.trim() ?? '';
      if (!path) {
        ctx.addIssue({
          code: 'custom',
          path: ['database'],
          message: t('shared.connection.validation.databaseRequired.error'),
        });
      } else if (!path.startsWith('/')) {
        ctx.addIssue({
          code: 'custom',
          path: ['database'],
          message: t('shared.connection.validation.databaseAbsolute.error'),
        });
      }
    } else {
      if (!AWS_STYLE_KINDS.has(input.kind) && !input.host) {
        ctx.addIssue({
          code: 'custom',
          path: ['host'],
          message: t('shared.connection.validation.hostRequired.error'),
        });
      }
      if (!AWS_STYLE_KINDS.has(input.kind) && !input.port) {
        ctx.addIssue({
          code: 'custom',
          path: ['port'],
          message: t('shared.connection.validation.portRequired.error'),
        });
      }
    }
  } else {
    if (input.uri !== null && input.uri.trim() === '') {
      ctx.addIssue({
        code: 'custom',
        path: ['uri'],
        message: t('shared.connection.validation.uriRequired.error'),
      });
    }
  }
});

// Query keys whose value is a secret fields mode cannot store; mirrors the Go side's
// secretOptionKeys (connections/uri.go). Lowercase — compare case-insensitively.
export const SECRET_OPTION_KEYS: ReadonlySet<string> = /*#__PURE__*/ new Set([
  'password',
  'sslpassword',
  'tlscertificatekeyfilepassword',
  'proxypassword',
]);

export type ConnectionInput = z.infer<typeof connectionInputSchema>;

// What the renderer gets. Note the absence of `password` — this is D9 enforced by the type.
export const connectionSummarySchema = connectionFieldsSchema.omit({ password: true }).extend({
  uri: z.null(), // P181: the stored URI is ciphertext, only the gated reveal returns it
  id: z.string(),
  /** The connection's config file name in the connections folder. */
  file: z.string(),
  sortOrder: z.number(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type ConnectionSummary = z.infer<typeof connectionSummarySchema>;

export const connectionStatusSchema = /*#__PURE__*/ z.enum([
  'disconnected',
  'connecting',
  'connected',
  'error',
]);
export type ConnectionStatus = z.infer<typeof connectionStatusSchema>;

export const connectionStateSchema = /*#__PURE__*/ z.object({
  connectionId: z.string(),
  status: connectionStatusSchema,
  serverVersion: z.string().nullable(),
  error: z.string().nullable(),
  since: z.number(), // epoch ms
  // Non-null only while connected — the toolbar's projection menu (Step 9) reads this to
  // decide whether server-side projection actually applies.
  caps: capsSchema.nullable(),
});
export type ConnectionState = z.infer<typeof connectionStateSchema>;
