import { z } from 'zod';

// Trimmed to the sections genuinely shared between both apps' own
// settingsSchema — appearance and the logLevel enum (both apps' own `advanced` section embeds it
// under its own name/key, alongside whatever else is app-only). Everything app-only
// (data/cache/api/dbMcp, Kira Studio only; git, Kira Space only — Kira Space is
// the only app with a git module) moved to each app's own frontend/src/state/settingsDomain.ts,
// which composes its own settingsSchema/settingsPatchSchema/defaultSettings out of this file's
// exports plus its own sections — the same split apps/*/internal/storage/model/settings.go already
// follows against internal/appsettings.

export const rowDensitySchema = /*#__PURE__*/ z.enum(['compact', 'comfortable']);
export type RowDensity = z.infer<typeof rowDensitySchema>;

// P17 D6: the one numeric bound both apps' appearance panes read (input min/max attributes, the
// settings dialog's own validity check), as an exported constant so it's one number, not two
// hard-coded copies.
export const FONT_SIZE_RANGE = { min: 9, max: 24 } as const;

// Genuinely shared, not merely parallel — domain/http.ts's own HttpRequestSettingsWire
// (Kira Studio's HTTP request builder, the only consumer of either) needs this enum from a shared
// package file, and a shared package may not import apps/*/frontend/src/state/settingsDomain.ts
// (CLAUDE.md's "a shared package never imports apps/*/internal/..." applies the same way to
// apps/*/frontend/src/state), so it stays here rather than moving into Kira Studio's own
// settingsDomain.ts with the rest of apiSettingsSchema.
export const HTTP_VERSIONS = ['1.1', '2'] as const;
export const httpVersionSchema = /*#__PURE__*/ z.enum(HTTP_VERSIONS);
export type HttpVersion = z.infer<typeof httpVersionSchema>;

export const appearanceSettingsSchema = /*#__PURE__*/ z.object({
  fontFamily: z.string(),
  // UI-only bound (FONT_SIZE_RANGE) — deliberately not enforced here, same discipline as
  // wordWrap/rowColoring's `.default(...)` below: an already-stored row outside 9-24 must still
  // hydrate.
  fontSize: z.number(),
  rowDensity: rowDensitySchema,
  // P42 D14: word wrap in every Monaco surface (query console, Mongo console, cell editor,
  // definition view, ...). `.default(true)` keeps a settings row saved before this field existed
  // wrapping as it always did.
  wordWrap: z.boolean().default(true),
  // P9: colour grid cell text by the column's data type. `.default(true)` keeps a pre-P9 stored
  // shape parsing to today's behavior (colouring on).
  rowColoring: z.boolean().default(true),
});
export type AppearanceSettings = z.infer<typeof appearanceSettingsSchema>;

// The diagnostic-log-verbosity enum — genuinely shared, not merely
// parallel, between Kira Studio's own `advanced.logLevel` (alongside opLogRetentionDays/
// expensiveQueryRows, app-only) and Kira Space's own `advanced.gitLogLevel` (its own `advanced`
// section's entire content) — each app's own name/key, same enum. Go's own
// appsettings.ValidLogLevel mirrors this same enum.
export const logLevelSchema = /*#__PURE__*/ z.enum(['off', 'error', 'warn', 'info', 'debug']);
export type LogLevel = z.infer<typeof logLevelSchema>;

// P316: the agent harness every new agent run, launch and structured call uses. A string id, not an
// enum: the registered providers live in Go, and an unknown id falls back to the default there.
export const DEFAULT_AGENT_PROVIDER = 'claude';
export const agentSettingsSchema = /*#__PURE__*/ z.object({
  provider: z.string().default(DEFAULT_AGENT_PROVIDER),
  // P455: agents the app starts skip the user's own agent config.
  hermetic: z.boolean().default(true),
});
export type AgentSettings = z.infer<typeof agentSettingsSchema>;
