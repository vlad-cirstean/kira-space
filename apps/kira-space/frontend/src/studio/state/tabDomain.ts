import { grpcRequestTabStateSchema } from '@shared/domain/grpc';
import { httpRequestTabStateSchema } from '@shared/domain/http';
import { kibanaConsoleStateSchema } from '@shared/domain/kibana';
import {
  SQS_DEFAULT_VISIBILITY_TIMEOUT_SECONDS,
  SQS_MAX_VISIBILITY_TIMEOUT_SECONDS,
  SQS_MIN_VISIBILITY_TIMEOUT_SECONDS,
} from '@shared/domain/streamFilter';
import {
  type PageSize,
  pageSizeSchema,
  scriptRunTabStateSchema,
  sortSpecSchema,
  type TabScope,
  tabRecordBase,
  terminalTabStateSchema,
} from '@shared/domain/tabs';
import { z } from 'zod';

// Kira Studio's own tab-kind vocabulary, split out of the old shared
// `packages/shared/domain/tabs.ts` — 12 kinds, `terminal` the one genuinely shared with Kira
// Space (its schema stays in the shared file). See that file's own header for why this split
// happened.
// Not consumed outside this file — every other module wants the derived `StudioTabKind` type
// (below) or the RENDERABLE_TAB_KINDS/TAB_KIND_MODE constants, never the schema itself.
const studioTabKindSchema = /*#__PURE__*/ z.enum([
  'data',
  'definition',
  'document',
  'keyvalue',
  'stream',
  'console',
  'browse',
  // P2: the first Api-mode kind (§2 F1) — four vocabularies join it, this one plus
  // STUDIO_RENDERABLE_TAB_KINDS/STUDIO_TAB_KIND_MODE below and Go's own model.RenderableTabKinds.
  'http-request',
  // P11 D2: the second kind inside the 'api' mode — a protocol AND a surface, following
  // 'http-request''s own naming reasoning verbatim.
  'grpc-request',
  // P17 D16: the third kind inside the 'api' mode — one collection's or one environment's
  // variable set, opened as a tab rather than a dialog.
  'variable-set',
  // P28 D16(c): the fourth kind inside the 'api' mode — the environment *list*.
  'environments',
  // P83 §7.1: an embedded shell at a working directory, rendered with @xterm/xterm.
  'terminal',
  // P242: a smart script run, opened from the Automations module.
  'script-run',
]);
export type StudioTabKind = z.infer<typeof studioTabKindSchema>;

// 'data' and 'definition' are renderable; 'console' joins them, 'document', 'keyvalue', 'stream',
// 'browse', 'http-request', 'grpc-request'. The restore path drops rows of any other kind with
// a `warn` — a closed vocabulary decided once, same discipline as the Caps/connectionKind.
export const STUDIO_RENDERABLE_TAB_KINDS: readonly StudioTabKind[] = [
  'data',
  'definition',
  'console',
  'document',
  'keyvalue',
  'stream',
  'browse',
  'http-request',
  'grpc-request',
  'variable-set',
  'environments',
  'terminal',
  'script-run',
];

// P1 D5: a tab's mode is a total function of its kind — no mode column, no migration. All seven
// Studio kinds map to 'studio'; 'http-request' (P2) and 'grpc-request' (P11 D2) both map to 'api'.
export const STUDIO_TAB_KIND_MODE: Record<StudioTabKind, TabScope> = {
  data: 'studio',
  definition: 'studio',
  console: 'studio',
  document: 'studio',
  keyvalue: 'studio',
  stream: 'studio',
  browse: 'studio',
  'http-request': 'api',
  'grpc-request': 'api',
  'variable-set': 'api',
  environments: 'api',
  terminal: 'repo',
  'script-run': 'repo',
};

// Every field defaulted, matching defaultDataTabState's own values below and
// the convention documentTabStateSchema/streamTabStateSchema/browseTabStateSchema already use — a
// row saved before a field existed used to fail safeParse outright on that one missing field,
// falling back to raw, unvalidated state (createTabsStore.ts's own hydrateTabs).
export const dataTabStateSchema = /*#__PURE__*/ z.object({
  pageSize: pageSizeSchema.default(100),
  pageIndex: z.number().int().min(0).default(0), // what the pager shows; offset = pageIndex * pageSize
  filter: z.string().nullable().default(null),
  sort: sortSpecSchema.nullable().default(null),
  projection: /*#__PURE__*/ z.array(z.string()).nullable().default(null),
  columnWidths: /*#__PURE__*/ z.record(z.string(), z.number()).default({}),
  columnOrder: /*#__PURE__*/ z.array(z.string()).nullable().default(null),
  scrollTop: z.number().default(0),
  scrollLeft: z.number().default(0),
});
export type DataTabState = z.infer<typeof dataTabStateSchema>;

// P19 D7: which pane (Structure/Source) the tab was last showing — the one thing this tab now
// has worth remembering. `.default('structure')` keeps a tab saved under the old empty `{}` shape
// restorable.
export const definitionTabStateSchema = /*#__PURE__*/ z.object({
  pane: /*#__PURE__*/ z.enum(['structure', 'source']).default('structure'),
});
export type DefinitionTabState = z.infer<typeof definitionTabStateSchema>;

// Only the editor's own text is session state (§8.4) — the last run's results are runtime-only.
// `newResultSet` (P40 D6, re-flipped back on P46-2): the toolbar toggle deciding whether a run
// appends a new result set (stacking) or replaces the current one with a fresh single result.
export const consoleTabStateSchema = /*#__PURE__*/ z.object({
  text: z.string(),
  newResultSet: z.boolean().default(true),
  // P424: Discover state of a kibana console tab; absent on every other kind, so no migration.
  kibana: kibanaConsoleStateSchema.optional(),
});
export type ConsoleTabState = z.infer<typeof consoleTabStateSchema>;

// Per-_id expand/collapse memory, the search text, and (mirroring DataTabState) the sort,
// projection and pageSize the toolbar's own controls set are session state (§8.7).
export const documentTabStateSchema = /*#__PURE__*/ z.object({
  expanded: /*#__PURE__*/ z.record(z.string(), z.boolean()),
  search: z.string(),
  sort: sortSpecSchema.nullable().default(null),
  projection: /*#__PURE__*/ z.array(z.string()).nullable().default(null),
  pageSize: pageSizeSchema.default(100),
  pageIndex: z.number().int().min(0).default(0),
});
export type DocumentTabState = z.infer<typeof documentTabStateSchema>;

// `pageIndex` is the offset-strategy fallback's own position — needed because a redis list key's
// pagination has no id/cursor token to advance by (P9's read.ts uses plain LRANGE offsets).
export const keyValueTabStateSchema = /*#__PURE__*/ z.object({
  pageIndex: z.number().int().min(0),
  pageSize: pageSizeSchema.default(100),
});
export type KeyValueTabState = z.infer<typeof keyValueTabStateSchema>;

// Read-only view — no edit memory to persist, no offset-fallback position to remember either.
export const streamTabStateSchema = /*#__PURE__*/ z.object({
  pageSize: pageSizeSchema.default(100),
  offsetFilter: z.string().nullable().default(null),
  partitions: /*#__PURE__*/ z.array(z.number().int()).default([]),
  timestampFilter: z.string().nullable().default(null),
  columnWidths: /*#__PURE__*/ z.record(z.string(), z.number()).default({}),
  // Read-only SQS poll hide time; unused by Kafka tabs (shared schema).
  sqsVisibilityTimeoutSeconds: z
    .number()
    .int()
    .min(SQS_MIN_VISIBILITY_TIMEOUT_SECONDS)
    .max(SQS_MAX_VISIBILITY_TIMEOUT_SECONDS)
    .catch(SQS_DEFAULT_VISIBILITY_TIMEOUT_SECONDS),
});
export type StreamTabState = z.infer<typeof streamTabStateSchema>;

// P41: a Browse tab's identity is its container; the level currently shown is per-tab session
// state. `''` means "the tab's own `path`".
export const browseTabStateSchema = /*#__PURE__*/ z.object({
  levelPath: z.string().default(''),
  /** P63: the split's list-pane width in px. `0` means "the default". */
  listWidth: z.number().default(0),
});
export type BrowseTabState = z.infer<typeof browseTabStateSchema>;

// P17 D16: a variable-set tab's identity is `path` — `scope`/`ownerId` here are what the view
// itself needs to query VariablesRepo, duplicated from `path` rather than parsed back out of it.
export const variableSetTabStateSchema = /*#__PURE__*/ z.object({
  scope: /*#__PURE__*/ z.enum(['collection', 'environment']),
  ownerId: z.string(),
  name: z.string().default(''),
});
export type VariableSetTabState = z.infer<typeof variableSetTabStateSchema>;

// P28 D16(c): the environments tab carries no state of its own — an empty object, like every
// other kind's state, so parseState has something to validate against.
export const environmentsTabStateSchema = /*#__PURE__*/ z.object({});
export type EnvironmentsTabState = z.infer<typeof environmentsTabStateSchema>;

// Not consumed outside this file — every other module wants the derived `TabRecord` type below.
const tabRecordSchema = /*#__PURE__*/ z.discriminatedUnion('kind', [
  /*#__PURE__*/ z.object({ ...tabRecordBase, kind: z.literal('data'), state: dataTabStateSchema }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('definition'),
    state: definitionTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('console'),
    state: consoleTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('document'),
    state: documentTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('keyvalue'),
    state: keyValueTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('stream'),
    state: streamTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('browse'),
    state: browseTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('http-request'),
    state: httpRequestTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('grpc-request'),
    state: grpcRequestTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('variable-set'),
    state: variableSetTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('environments'),
    state: environmentsTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('terminal'),
    state: terminalTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('script-run'),
    state: scriptRunTabStateSchema,
  }),
]);
export type TabRecord = z.infer<typeof tabRecordSchema>;
export type DataTabRecord = Extract<TabRecord, { kind: 'data' }>;
export type DefinitionTabRecord = Extract<TabRecord, { kind: 'definition' }>;
export type ConsoleTabRecord = Extract<TabRecord, { kind: 'console' }>;
export type DocumentTabRecord = Extract<TabRecord, { kind: 'document' }>;
export type KeyValueTabRecord = Extract<TabRecord, { kind: 'keyvalue' }>;
export type StreamTabRecord = Extract<TabRecord, { kind: 'stream' }>;
export type BrowseTabRecord = Extract<TabRecord, { kind: 'browse' }>;
export type HttpRequestTabRecord = Extract<TabRecord, { kind: 'http-request' }>;
export type GrpcRequestTabRecord = Extract<TabRecord, { kind: 'grpc-request' }>;
export type VariableSetTabRecord = Extract<TabRecord, { kind: 'variable-set' }>;
export type EnvironmentsTabRecord = Extract<TabRecord, { kind: 'environments' }>;

export function asDataTab(tab: TabRecord | null | undefined): DataTabRecord | null {
  return tab && tab.kind === 'data' ? tab : null;
}

export function asConsoleTab(tab: TabRecord | null | undefined): ConsoleTabRecord | null {
  return tab && tab.kind === 'console' ? tab : null;
}

export function asDocumentTab(tab: TabRecord | null | undefined): DocumentTabRecord | null {
  return tab && tab.kind === 'document' ? tab : null;
}

export function asKeyValueTab(tab: TabRecord | null | undefined): KeyValueTabRecord | null {
  return tab && tab.kind === 'keyvalue' ? tab : null;
}

export function asStreamTab(tab: TabRecord | null | undefined): StreamTabRecord | null {
  return tab && tab.kind === 'stream' ? tab : null;
}

export function asBrowseTab(tab: TabRecord | null | undefined): BrowseTabRecord | null {
  return tab && tab.kind === 'browse' ? tab : null;
}

export function asHttpRequestTab(tab: TabRecord | null | undefined): HttpRequestTabRecord | null {
  return tab && tab.kind === 'http-request' ? tab : null;
}

export function asGrpcRequestTab(tab: TabRecord | null | undefined): GrpcRequestTabRecord | null {
  return tab && tab.kind === 'grpc-request' ? tab : null;
}

export function defaultDataTabState(pageSize: PageSize): DataTabState {
  return {
    pageSize,
    pageIndex: 0,
    filter: null,
    sort: null,
    projection: null,
    columnWidths: {},
    columnOrder: null,
    scrollTop: 0,
    scrollLeft: 0,
  };
}

export function defaultDefinitionTabState(): DefinitionTabState {
  return { pane: 'structure' };
}

export function defaultConsoleTabState(): ConsoleTabState {
  return { text: '', newResultSet: true };
}

export function defaultDocumentTabState(pageSize: PageSize = 100): DocumentTabState {
  return { expanded: {}, search: '', sort: null, projection: null, pageSize, pageIndex: 0 };
}

export function defaultKeyValueTabState(pageSize: PageSize = 100): KeyValueTabState {
  return { pageIndex: 0, pageSize };
}

export function defaultStreamTabState(pageSize: PageSize = 100): StreamTabState {
  return {
    pageSize,
    offsetFilter: null,
    partitions: [],
    timestampFilter: null,
    columnWidths: {},
    sqsVisibilityTimeoutSeconds: SQS_DEFAULT_VISIBILITY_TIMEOUT_SECONDS,
  };
}

export function defaultBrowseTabState(): BrowseTabState {
  return { levelPath: '', listWidth: 0 };
}
