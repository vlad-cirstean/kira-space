import {
  agentSettingsSchema,
  appearanceSettingsSchema,
  DEFAULT_AGENT_PROVIDER,
  FONT_SIZE_RANGE,
  HTTP_VERSIONS,
  httpVersionSchema,
  logLevelSchema,
} from '@shared/domain/settings';
import { z } from 'zod';
import { TRACKED_KINDS, type TrackedKind, trackingDefaults } from './trackingConcepts';

// Space's settings, the database module's sections (data, cache, api, dbMcp) included.
export { FONT_SIZE_RANGE, HTTP_VERSIONS };

// G7 D16: minutes between automatic background fetches; 0 disables it.
export const FETCH_AUTO_INTERVAL_MINUTES_RANGE = { min: 0, max: 1440 } as const;

// Server-owned — two windows disagreeing about either is a correctness/safety issue (a
// force-push confirmation only one window enforces, an auto-fetch cadence that differs per
// viewer), edited only in this dialog, never as a per-window setting. Not exported —
// nothing outside this file references the raw schema object; `Settings['git']` covers real
// consumers.
const gitSettingsSchema = /*#__PURE__*/ z.object({
  protectedBranches: z.array(z.string()).default(['main', 'master', 'release/*']),
  fetchAutoIntervalMinutes: z
    .number()
    .int()
    .min(FETCH_AUTO_INTERVAL_MINUTES_RANGE.min)
    .max(FETCH_AUTO_INTERVAL_MINUTES_RANGE.max)
    .default(0),
  // git.path was already classified server-owned (it answers "where is the git binary
  // on this machine", not a per-repo or per-window preference) but its wiring was dead
  // — a third leaf of this same trio, fixed the same way, not a redesign. Empty means "auto-
  // discover" (PATH lookup) — gitclient.Discovery's own existing contract.
  gitPath: z.string().default(''),
  // P92 item 9: 0 = follow appearance.fontSize. Reaches every embedded git-ui surface (graph,
  // diff, review) through --kira-graph-font-size (git-ui's theme/git.css graph scale).
  graphFontSize: z.number().int().min(0).max(FONT_SIZE_RANGE.max).default(0),
});

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
  // Let agents send API requests with the variables an environment shares with them.
  apiRequests: z.boolean().default(false),
  apiApproval: z.enum(['always', 'never']).default('always'),
});

// advanced carries just gitLogLevel here — this app's own diagnostic log verbosity, its entire
// `advanced` section (apps/kira-space/internal/storage/model/settings.go's own AdvancedSettings
// embeds appsettings.AdvancedCore for the same one leaf). Not exported — nothing
// outside this file references the raw schema object or its own inferred type; AdvancedPane.vue
// reads the leaf's type through the shared `LogLevel` above instead.
const advancedSettingsSchema = /*#__PURE__*/ z.object({
  gitLogLevel: logLevelSchema.default('info'),
  // P246: system notification when a popup waits in a window that is not in front.
  notifyPrompts: z.boolean().default(true),
  opLogRetentionDays: z
    .number()
    .int()
    .min(OP_LOG_RETENTION_DAYS_RANGE.min)
    .max(OP_LOG_RETENTION_DAYS_RANGE.max)
    .default(30),
  // Estimated rows read, never a cost unit: drives the Explain over-threshold flag and auto-explain.
  expensiveQueryRows: z
    .number()
    .int()
    .min(EXPENSIVE_QUERY_ROWS_RANGE.min)
    .max(EXPENSIVE_QUERY_ROWS_RANGE.max)
    .default(100_000),
});

// P120: inlineBlame/dateFormat are this app's own — the only app with a git module to show either
// in. `.extend(...)` on the shared appearanceSettingsSchema keeps the five shared leaves' own
// validation/defaults in one place.
const appSpaceAppearanceSettingsSchema = /*#__PURE__*/ appearanceSettingsSchema.extend({
  // P62: inline git-blame annotation at the end of the cursor's line in the repo file viewer.
  // `.default(true)` follows the same discipline as wordWrap/rowColoring above — a stored row
  // saved before this field existed hydrates with the annotation on.
  inlineBlame: z.boolean().default(true),
  // P72 §9.1: relative-vs-absolute commit timestamps in the git graph — moved here from the
  // per-repo RepoSettingsDialog.vue/PersistedViewState (a reading preference about the person, not
  // the repository, the same class as fontSize/fontFamily above). `.default('relative')` matches
  // PersistedViewState's own pre-existing default, so an existing stored settings row hydrates to
  // today's behavior.
  dateFormat: z.enum(['relative', 'absolute']).default('relative'),
});

// The queue-board UI prefs (panel width, history window, calendar overrides), persisted as
// settings leaves rather than component state so they survive a relaunch.
// Not exported — nothing outside this file references the raw schema object; `Settings['ade']`
// covers real consumers.
const ADE_HISTORY_DAYS_RANGE = { min: 1, max: 365 } as const;
const ADE_WORKDAY_HOURS_RANGE = { min: 1, max: 24 } as const;
const ADE_SPAN_DAY_SHARE_RANGE = { min: 0.05, max: 1 } as const;
// §0.13 keeps at most 1000 override dates in any one list — well past what a calendar UI would ever
// need to page through, so it is a sanity ceiling, not a real limit.
const ADE_DATE_LIST_MAX = 1000;

const adeIsoDateSchema = /*#__PURE__*/ z
  .string()
  .regex(/^\d{4}-\d{2}-\d{2}$/, 'expected YYYY-MM-DD');
const adeDateListSchema = /*#__PURE__*/ z.array(adeIsoDateSchema).max(ADE_DATE_LIST_MAX);

const adeSettingsSchema = /*#__PURE__*/ z.object({
  historyDays: z
    .number()
    .int()
    .min(ADE_HISTORY_DAYS_RANGE.min)
    .max(ADE_HISTORY_DAYS_RANGE.max)
    .default(14),
  offDays: adeDateListSchema.default([]),
  workWeekendDays: adeDateListSchema.default([]),
  workdayHours: z
    .number()
    .min(ADE_WORKDAY_HOURS_RANGE.min)
    .max(ADE_WORKDAY_HOURS_RANGE.max)
    .default(6),
  spanDayShare: z
    .number()
    .min(ADE_SPAN_DAY_SHARE_RANGE.min)
    .max(ADE_SPAN_DAY_SHARE_RANGE.max)
    .default(0.5),
  headlessSettingSources: z.enum(['user', 'all']).default('all'),
});

// The shared agent.provider leaf plus this app's agent leaves. P188: keep this Mac awake while any
// agent session runs, independent of the title bar's own keep-awake toggle. Off by default — an OS
// power assertion is opt-in. Not exported, like the other raw section schemas.
const appAgentSettingsSchema = /*#__PURE__*/ agentSettingsSchema.extend({
  keepAwakeWithAgents: z.boolean().default(false),
  // P238: desktop notifications. All on by default; notifyEnabled is the master switch.
  notifyEnabled: z.boolean().default(true),
  notifyOnFinished: z.boolean().default(true),
  notifyOnNeedsInput: z.boolean().default(true),
  notifyOnRunEnded: z.boolean().default(true),
  notifyIncludeMessage: z.boolean().default(true),
  // P239: agent usage limits in the ADE status bar.
  usageEnabled: z.boolean().default(true),
});

// P212: the mobile agents web server. Off until enabled; the port is unprivileged. Changed
// through MobileAccessService, which restarts the server when it is running.
export const MOBILE_PORT_RANGE = { min: 1024, max: 65535 } as const;
const mobileSettingsSchema = /*#__PURE__*/ z.object({
  enabled: z.boolean().default(false),
  port: z.number().int().min(MOBILE_PORT_RANGE.min).max(MOBILE_PORT_RANGE.max).default(7790),
  agentInput: z.boolean().default(false),
});

// P430: the clipboard capture hotkey. The Go side validates the chord (model.ValidCaptureHotkey).
const DEFAULT_CAPTURE_HOTKEY = 'CmdOrCtrl+Shift+K';
const captureSettingsSchema = /*#__PURE__*/ z.object({
  enabled: z.boolean().default(true),
  hotkey: z.string().default(DEFAULT_CAPTURE_HOTKEY),
});

// P314: the model alias per provider id for every memory call (gate, reconcile, import, distill).
// A provider without an entry uses its default.
const memorySettingsSchema = /*#__PURE__*/ z.object({
  models: z.record(z.string(), z.string()).default({}),
});

// P556: one boolean per feature still behind a toggle; a new flag is one field here and one row in
// workbench/settings/ExperimentalPane.vue.
const experimentalSettingsSchema = /*#__PURE__*/ z.object({
  memory: z.boolean().default(false),
});

// P512: per item kind, whether each concept counts as a meaningful update. One boolean per concept
// in state/trackingConcepts.ts; an unknown stored key drops, a missing one reads its default.
function trackingKindSchema(kind: TrackedKind) {
  const entry = TRACKED_KINDS.find((k) => k.kind === kind);
  const shape: Record<string, z.ZodDefault<z.ZodBoolean>> = {};
  for (const c of entry?.concepts ?? []) shape[c.id] = z.boolean().default(c.default);
  return z.object(shape).default(trackingDefaults(kind));
}
const trackingSettingsSchema = /*#__PURE__*/ z.object({
  changeRequest: trackingKindSchema('changeRequest'),
  issue: trackingKindSchema('issue'),
  branch: trackingKindSchema('branch'),
});

// `.default(...)` on every section is load-bearing: an older kira-space.sqlite has a settings row
// with no `advanced`/`git`/`ade`/`agent` keys, and that row must still parse on next launch.
const settingsSchema = /*#__PURE__*/ z.object({
  appearance: appSpaceAppearanceSettingsSchema,
  advanced: advancedSettingsSchema.default({
    gitLogLevel: 'info',
    notifyPrompts: true,
    opLogRetentionDays: 30,
    expensiveQueryRows: 100_000,
  }),
  git: gitSettingsSchema.default({
    protectedBranches: ['main', 'master', 'release/*'],
    fetchAutoIntervalMinutes: 0,
    gitPath: '',
    graphFontSize: 0,
  }),
  ade: adeSettingsSchema.default({
    historyDays: 14,
    offDays: [],
    workWeekendDays: [],
    workdayHours: 6,
    spanDayShare: 0.5,
    headlessSettingSources: 'all',
  }),
  agent: appAgentSettingsSchema.default({
    provider: DEFAULT_AGENT_PROVIDER,
    hermetic: true,
    keepAwakeWithAgents: false,
    notifyEnabled: true,
    notifyOnFinished: true,
    notifyOnNeedsInput: true,
    notifyOnRunEnded: true,
    notifyIncludeMessage: true,
    usageEnabled: true,
  }),
  mobile: mobileSettingsSchema.default({
    enabled: false,
    port: 7790,
    agentInput: false,
  }),
  capture: captureSettingsSchema.default({ enabled: true, hotkey: DEFAULT_CAPTURE_HOTKEY }),
  memory: memorySettingsSchema.default({ models: {} }),
  experimental: experimentalSettingsSchema.default({ memory: false }),
  tracking: trackingSettingsSchema.default({
    changeRequest: trackingDefaults('changeRequest'),
    issue: trackingDefaults('issue'),
    branch: trackingDefaults('branch'),
  }),
  data: dataSettingsSchema.default({ defaultPageSize: 100 }),
  cache: cacheSettingsSchema.default({ l2BudgetMb: 64 }),
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
    apiRequests: false,
    apiApproval: 'always',
  }),
});
export type Settings = z.infer<typeof settingsSchema>;

const settingsPatchSchema = /*#__PURE__*/ z.object({
  appearance: appSpaceAppearanceSettingsSchema.partial().optional(),
  advanced: advancedSettingsSchema.partial().optional(),
  git: gitSettingsSchema.partial().optional(),
  ade: adeSettingsSchema.partial().optional(),
  mobile: mobileSettingsSchema.partial().optional(),
  capture: captureSettingsSchema.partial().optional(),
  memory: memorySettingsSchema.partial().optional(),
  experimental: experimentalSettingsSchema.partial().optional(),
  tracking: trackingSettingsSchema.partial().optional(),
  agent: appAgentSettingsSchema.partial().optional(),
  data: dataSettingsSchema.partial().optional(),
  cache: cacheSettingsSchema.partial().optional(),
  api: apiSettingsSchema.partial().optional(),
  dbMcp: dbMcpSettingsSchema.partial().optional(),
});
export type SettingsPatch = z.infer<typeof settingsPatchSchema>;

export const defaultSettings: Settings = {
  appearance: {
    fontFamily: 'Menlo, monospace',
    fontSize: 12,
    rowDensity: 'comfortable',
    wordWrap: true,
    rowColoring: true,
    inlineBlame: true,
    dateFormat: 'relative',
  },
  advanced: {
    gitLogLevel: 'info',
    notifyPrompts: true,
    opLogRetentionDays: 30,
    expensiveQueryRows: 100_000,
  },
  git: {
    protectedBranches: ['main', 'master', 'release/*'],
    fetchAutoIntervalMinutes: 0,
    gitPath: '',
    graphFontSize: 0,
  },
  ade: {
    historyDays: 14,
    offDays: [],
    workWeekendDays: [],
    workdayHours: 6,
    spanDayShare: 0.5,
    headlessSettingSources: 'all',
  },
  agent: {
    provider: DEFAULT_AGENT_PROVIDER,
    hermetic: true,
    keepAwakeWithAgents: false,
    notifyEnabled: true,
    notifyOnFinished: true,
    notifyOnNeedsInput: true,
    notifyOnRunEnded: true,
    notifyIncludeMessage: true,
    usageEnabled: true,
  },
  mobile: {
    enabled: false,
    port: 7790,
    agentInput: false,
  },
  capture: {
    enabled: true,
    hotkey: DEFAULT_CAPTURE_HOTKEY,
  },
  memory: {
    models: {},
  },
  experimental: {
    memory: false,
  },
  tracking: {
    changeRequest: trackingDefaults('changeRequest'),
    issue: trackingDefaults('issue'),
    branch: trackingDefaults('branch'),
  },
  data: {
    defaultPageSize: 100,
  },
  cache: {
    l2BudgetMb: 64,
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
    apiRequests: false,
    apiApproval: 'always',
  },
};
