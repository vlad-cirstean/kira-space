import {
  agentSettingsSchema,
  appearanceSettingsSchema,
  DEFAULT_AGENT_PROVIDER,
  HTTP_VERSIONS,
  httpVersionSchema,
  logLevelSchema,
} from '@shared/domain/settings';
import { z } from 'zod';

// This app's own six-section settingsSchema/settingsPatchSchema/
// defaultSettings, split out of the former one shared `@shared/domain/settings` (which Kira Space's
// own three-section store carried five dead sections of — data/cache/api/dbMcp — the
// same "tab-kind vocabulary" defect). appearance/logLevel
// stay genuinely shared; this file imports `appearanceSettingsSchema`/`logLevelSchema`/
// `HTTP_VERSIONS`/`httpVersionSchema` unexported, purely to build the composed schema below —
// nothing outside this file references those specific names directly (`HTTP_VERSIONS` is the one
// exception, re-exported for ApiPane/RequestSettingsPane; `httpVersionSchema` itself stays
// import-only, domain/http.ts's own dependency on it goes straight to `@shared/domain/settings`,
// never through here). `FONT_SIZE_RANGE`/`AppearanceSettings`/`LogLevel`/`RowDensity` used to
// re-export here for AppearancePane/AdvancedPane; both now read them straight from
// `@shared/domain/settings` via the shared field components (`FontSizeField.vue` etc.).
export { HTTP_VERSIONS };

// P17 D6: the two numeric bounds a control can actually violate, as exported constants so the
// schema (where one applies), the input min/max attributes and the settings dialog's own
// validity check read one number each, not three hard-coded copies.
export const CACHE_L2_BUDGET_MB_RANGE = { min: 8, max: 1024 } as const;
export const OP_LOG_RETENTION_DAYS_RANGE = { min: 1, max: 365 } as const;
// The "expensive query" threshold is an estimated-*rows-read* number, never a
// cost unit — the two dialects that report a same-named `cost` field disagree by three orders of
// magnitude for a comparable scan, so no cost-based number could be shared
// across engines. 1,000 floor keeps the field meaningful; 1e9 ceiling is generous headroom above
// any real table this app's own fixture corpus uses.
export const EXPENSIVE_QUERY_ROWS_RANGE = { min: 1_000, max: 1_000_000_000 } as const;

const pageSizeSchema = /*#__PURE__*/ z.union([
  z.literal(10),
  z.literal(100),
  z.literal(1000),
  z.literal(10000),
]);

// Neither dataSettingsSchema/cacheSettingsSchema/advancedSettingsSchema/apiSettingsSchema/
// dbMcpSettingsSchema/settingsSchema/settingsPatchSchema below, nor
// their own `z.infer` type aliases, are exported: nothing outside this file references the raw
// zod schema objects or their per-section inferred types directly (confirmed by grep — every real
// consumer reads through the composed `Settings`/`SettingsPatch`/`defaultSettings` below, or
// through a specific pane-facing type like `ApiSettings`/`AppearanceSettings`, both still
// exported where a pane's own type-assertion pattern needs them).
const dataSettingsSchema = /*#__PURE__*/ z.object({
  defaultPageSize: pageSizeSchema,
});

const cacheSettingsSchema = /*#__PURE__*/ z.object({
  l2BudgetMb: z.number().int().min(CACHE_L2_BUDGET_MB_RANGE.min).max(CACHE_L2_BUDGET_MB_RANGE.max),
});

const advancedSettingsSchema = /*#__PURE__*/ z.object({
  opLogRetentionDays: z
    .number()
    .int()
    .min(OP_LOG_RETENTION_DAYS_RANGE.min)
    .max(OP_LOG_RETENTION_DAYS_RANGE.max),
  // P18 D14/D20: drives both the manual Explain panel's over-threshold flag and auto-explain's
  // warning strip. `.default(...)` is load-bearing the same way every other P17-era leaf's is —
  // an older stored settings row has no such key.
  expensiveQueryRows: z
    .number()
    .int()
    .min(EXPENSIVE_QUERY_ROWS_RANGE.min)
    .max(EXPENSIVE_QUERY_ROWS_RANGE.max)
    .default(100_000),
  // Studio's own diagnostic log verbosity (internal/logging, applied immediately by
  // bridge/settings.go's Set on every patch). `.default('info')` matches that leaf's own
  // pre-existing default. logLevelSchema is the shared enum — Kira Space validates the same leaf
  // against it under its own settings key.
  logLevel: logLevelSchema.default('info'),
  // P246: system notification when a popup waits in a window that is not in front.
  notifyPrompts: z.boolean().default(true),
});

// The seven request-settings leaves that used to be internal/httpclient package
// constants (options.go states the coupling back at this file: its normalize() defaults must
// equal this section's own defaults, field for field, since neither package may import the
// other). Three deliberate default changes from httpclient behaviour: timeout 30s -> none,
// max response 10 MiB -> 50 MB (-> 5 MB), max redirects unchanged at 10.
// HTTP_VERSIONS/httpVersionSchema come from @shared/domain/settings (re-exported above) —
// domain/http.ts's own dependency, not this section's.

// 0 = no timeout. Ceiling is one hour — past that a request is a subscription, not a request.
export const REQUEST_TIMEOUT_MS_RANGE = { min: 0, max: 3_600_000 } as const;
// 0 = unlimited. Whole MB, cache.l2BudgetMb's own unit. 2048 is well past any response a
// request builder should be holding in memory and rendering.
export const MAX_RESPONSE_MB_RANGE = { min: 0, max: 2048 } as const;
export const MAX_REDIRECTS_RANGE = { min: 0, max: 100 } as const;

const apiSettingsSchema = /*#__PURE__*/ z.object({
  httpVersion: httpVersionSchema.default('2'),
  requestTimeoutMs: z
    .number()
    .int()
    .min(REQUEST_TIMEOUT_MS_RANGE.min)
    .max(REQUEST_TIMEOUT_MS_RANGE.max)
    .default(0),
  maxResponseMb: z
    .number()
    .int()
    .min(MAX_RESPONSE_MB_RANGE.min)
    .max(MAX_RESPONSE_MB_RANGE.max)
    .default(5),
  sslVerify: z.boolean().default(true),
  followRedirects: z.boolean().default(true),
  maxRedirects: z
    .number()
    .int()
    .min(MAX_REDIRECTS_RANGE.min)
    .max(MAX_REDIRECTS_RANGE.max)
    .default(10),
  disableCookieJar: z.boolean().default(true),
});
export type ApiSettings = z.infer<typeof apiSettingsSchema>;

// M1 §6.2: one leaf, default false. The embedded DB MCP server instance's own on/off switch
// (internal/bridge/dbmcp.go owns the actual start/stop side effect; this leaf is only the
// persisted, cross-restart record of "should it be on"). A section on its own, not folded into
// `advanced`.
const dbMcpSettingsSchema = /*#__PURE__*/ z.object({
  serverEnabled: z.boolean().default(false),
  // Hold each run_query result for the user's review before the AI client gets it.
  previewResponses: z.boolean().default(false),
  focusOnApproval: z.boolean().default(true),
});

// `.default(...)` on every new section is load-bearing: an older kira.db has a settings
// row with no `data`/`cache`/`advanced`/`dbMcp` keys, and that row must still parse on
// next launch.
const settingsSchema = /*#__PURE__*/ z.object({
  appearance: appearanceSettingsSchema,
  data: dataSettingsSchema.default({ defaultPageSize: 100 }),
  cache: cacheSettingsSchema.default({ l2BudgetMb: 64 }),
  advanced: advancedSettingsSchema.default({
    opLogRetentionDays: 30,
    expensiveQueryRows: 100_000,
    logLevel: 'info',
    notifyPrompts: true,
  }),
  api: apiSettingsSchema.default({
    httpVersion: '2',
    requestTimeoutMs: 0,
    maxResponseMb: 5,
    sslVerify: true,
    followRedirects: true,
    maxRedirects: 10,
    disableCookieJar: true,
  }),
  dbMcp: dbMcpSettingsSchema.default({
    serverEnabled: false,
    previewResponses: false,
    focusOnApproval: true,
  }),
  agent: agentSettingsSchema.default({ provider: DEFAULT_AGENT_PROVIDER, hermetic: true }),
});
export type Settings = z.infer<typeof settingsSchema>;

const settingsPatchSchema = /*#__PURE__*/ z.object({
  appearance: appearanceSettingsSchema.partial().optional(),
  data: dataSettingsSchema.partial().optional(),
  cache: cacheSettingsSchema.partial().optional(),
  advanced: advancedSettingsSchema.partial().optional(),
  api: apiSettingsSchema.partial().optional(),
  dbMcp: dbMcpSettingsSchema.partial().optional(),
  agent: agentSettingsSchema.partial().optional(),
});
export type SettingsPatch = z.infer<typeof settingsPatchSchema>;

export const defaultSettings: Settings = {
  appearance: {
    fontFamily: 'Menlo, monospace',
    fontSize: 12,
    rowDensity: 'comfortable',
    wordWrap: true,
    rowColoring: true,
  },
  data: {
    defaultPageSize: 100,
  },
  cache: {
    l2BudgetMb: 64,
  },
  advanced: {
    opLogRetentionDays: 30,
    expensiveQueryRows: 100_000,
    logLevel: 'info',
    notifyPrompts: true,
  },
  api: {
    httpVersion: '2',
    requestTimeoutMs: 0,
    maxResponseMb: 5,
    sslVerify: true,
    followRedirects: true,
    maxRedirects: 10,
    disableCookieJar: true,
  },
  dbMcp: {
    serverEnabled: false,
    previewResponses: false,
    focusOnApproval: true,
  },
  agent: {
    provider: DEFAULT_AGENT_PROVIDER,
    hermetic: true,
  },
};
