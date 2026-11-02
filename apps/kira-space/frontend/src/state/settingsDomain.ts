import {
  agentSettingsSchema,
  appearanceSettingsSchema,
  DEFAULT_AGENT_PROVIDER,
  FONT_SIZE_RANGE,
  logLevelSchema,
} from '@shared/domain/settings';
import { z } from 'zod';

// This app's own three-section settingsSchema/settingsPatchSchema/defaultSettings — this store used
// to carry Kira Studio's own data/cache/api/dbMcp sections dead (never populated by this
// app's own Go backend, apps/kira-space/internal/storage/model/settings.go), the same "tab-kind
// vocabulary" defect already fixed for tabs. appearance/logLevel enum stay genuinely
// shared (this app's own leaf name stays `advanced.gitLogLevel`); FONT_SIZE_RANGE is the one
// this app's own GitPane reads directly, so it re-exports; git is this app's own (Kira Space
// is the only app with a git module) — its schema and `FETCH_AUTO_INTERVAL_MINUTES_RANGE` are
// defined below, not imported. The raw schema objects
// (appearanceSettingsSchema/logLevelSchema/rowDensitySchema) stay import-only: nothing outside
// this file references them directly, every real consumer reads through the composed
// `Settings`/`SettingsPatch`/`defaultSettings` below. `AppearanceSettings`/`LogLevel`/
// `RowDensity` used to re-export here too; every pane now reads them straight from
// `@shared/domain/settings` via the shared field components (`FontSizeField.vue` etc.).
export { FONT_SIZE_RANGE };

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

// advanced carries just gitLogLevel here — this app's own diagnostic log verbosity, its entire
// `advanced` section (apps/kira-space/internal/storage/model/settings.go's own AdvancedSettings
// embeds appsettings.AdvancedCore for the same one leaf). Not exported — nothing
// outside this file references the raw schema object or its own inferred type; AdvancedPane.vue
// reads the leaf's type through the shared `LogLevel` above instead.
const advancedSettingsSchema = /*#__PURE__*/ z.object({
  gitLogLevel: logLevelSchema.default('info'),
  // P246: system notification when a popup waits in a window that is not in front.
  notifyPrompts: z.boolean().default(true),
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

// `.default(...)` on every section is load-bearing: an older kira-space.sqlite has a settings row
// with no `advanced`/`git`/`ade`/`agent` keys, and that row must still parse on next launch.
const settingsSchema = /*#__PURE__*/ z.object({
  appearance: appSpaceAppearanceSettingsSchema,
  advanced: advancedSettingsSchema.default({ gitLogLevel: 'info', notifyPrompts: true }),
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
  agent: appAgentSettingsSchema.partial().optional(),
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
};
