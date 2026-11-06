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

// The repository module's tab kinds, plus the two shared ones (`terminal`, `script-run`).
const spaceTabKindSchema = /*#__PURE__*/ z.enum([
  'repo-graph',
  'repo-file',
  'local-file',
  'repo-diff',
  'repo-multi-diff',
  'terminal',
  'script-run',
]);
export type SpaceTabKind = z.infer<typeof spaceTabKindSchema>;

export const SPACE_RENDERABLE_TAB_KINDS: readonly SpaceTabKind[] = [
  'repo-graph',
  'repo-file',
  'local-file',
  'repo-diff',
  'repo-multi-diff',
  'terminal',
  'script-run',
];

export const SPACE_TAB_KIND_MODE: Record<SpaceTabKind, TabScope> = {
  'repo-graph': 'repo',
  'repo-file': 'repo',
  'local-file': 'repo',
  'repo-diff': 'repo',
  'repo-multi-diff': 'repo',
  terminal: 'repo',
  'script-run': 'repo',
};

// The permanently pinned, unclosable first tab every repo workspace reserves for the
// git graph. `viewState` holds git-ui's own `PersistedViewState` (version-6 shape) — a permissive
// passthrough rather than re-deriving that shape here; git-ui's own `parsePersistedViewState` is
// the sole validator. `reviewSession` is a second opaque blob of the same shape, the review
// sidebar's own "back to branch selection" resume point.
export const repoGraphTabStateSchema = /*#__PURE__*/ z.object({
  viewState: z.unknown().nullable().default(null),
  reviewSession: z.unknown().nullable().default(null),
});
export type RepoGraphTabState = z.infer<typeof repoGraphTabStateSchema>;

// C5 §9.3/§12: revealLine is the one thing worth remembering across a restore. P67c D12: source
// is the default for every file type this app opens, markdown included. P74 §7.3: non-null `rev`
// means this tab shows `path` at that revision, read-only, via `file.read` — not the worktree file.
export const repoFileTabStateSchema = /*#__PURE__*/ z.object({
  revealLine: z.number().int().min(1).nullable().default(null),
  markdownReading: z.boolean().default(false),
  rev: z.string().nullable().default(null),
});
export type RepoFileTabState = z.infer<typeof repoFileTabStateSchema>;

// An externally opened file outside any imported repository: `path` is the absolute path and the
// tab lives in GENERAL_WORKSPACE. No `rev`, no blame: nothing but the file on disk.
export const localFileTabStateSchema = /*#__PURE__*/ z.object({
  revealLine: z.number().int().min(1).nullable().default(null),
  markdownReading: z.boolean().default(false),
});
export type LocalFileTabState = z.infer<typeof localFileTabStateSchema>;

// C11 §7.5: a review diff's own target — which branch this review is against, that branch's tip
// at review-open time, and the left side's display label.
// Not consumed outside this file — every other module wants the derived `ReviewRef` type below.
const reviewRefSchema = /*#__PURE__*/ z.object({
  branch: z.string(),
  branchTip: z.string(),
  leftLabel: z.string(),
});
export type ReviewRef = z.infer<typeof reviewRefSchema>;

// C6 §8.1/D10: a diff tab's session state — both null (the default) is C6's own HEAD-vs-worktree
// comparison; a commit diff's view reads `left`/`right` as the two revisions. C11 §7.5: review
// turns on the gutter/comment-thread layer over the same left/right pair.
export const repoDiffTabStateSchema = /*#__PURE__*/ z.object({
  left: z.string().nullable().default(null),
  right: z.string().nullable().default(null),
  leftLabel: z.string().nullable().default(null),
  rightLabel: z.string().nullable().default(null),
  review: reviewRefSchema.nullable().default(null),
  // Uncommitted file of a task branch's worktree (review window): `left`/`right` then only key the
  // Monaco models, content comes from AdeTaskService.ReviewWorkingDiff.
  working: /*#__PURE__*/ z.object({ branchId: z.string() }).nullable().default(null),
});
export type RepoDiffTabState = z.infer<typeof repoDiffTabStateSchema>;

// P92 item 5: one commit's whole changed-file set, in one tab (VS Code's multi-file diff).
// `files` is the commit's own file order, captured at open time.
export const repoMultiDiffTabStateSchema = /*#__PURE__*/ z.object({
  left: z.string(),
  right: z.string(),
  leftLabel: z.string(),
  rightLabel: z.string(),
  files: /*#__PURE__*/ z.array(z.string()).default([]),
  review: reviewRefSchema.nullable().default(null),
});
export type RepoMultiDiffTabState = z.infer<typeof repoMultiDiffTabStateSchema>;

// The database and API modules' own tab kinds. `terminal` and `script-run` belong to the shared
// Space vocabulary above, so they are not repeated here.
const studioTabKindSchema = /*#__PURE__*/ z.enum([
  'data',
  'definition',
  'document',
  'keyvalue',
  'stream',
  'console',
  'browse',
  // P2: the first Api-mode kind (§2 F1) — four vocabularies join it, this one plus
  // STUDIO_TAB_KIND_MODE below and Go's own model.RenderableTabKinds.
  'http-request',
  // P11 D2: the second kind inside the 'api' mode — a protocol AND a surface, following
  // 'http-request''s own naming reasoning verbatim.
  'grpc-request',
  // P17 D16: the third kind inside the 'api' mode — one collection's or one environment's
  // variable set, opened as a tab rather than a dialog.
  'variable-set',
  // P28 D16(c): the fourth kind inside the 'api' mode — the environment *list*.
  'environments',
]);
export type StudioTabKind = z.infer<typeof studioTabKindSchema>;

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

// Every kind the merged tab store holds.
export type TabKind = SpaceTabKind | StudioTabKind;

export const TAB_KIND_MODE: Record<TabKind, TabScope> = {
  ...SPACE_TAB_KIND_MODE,
  ...STUDIO_TAB_KIND_MODE,
};

// Not consumed outside this file — every other module wants the derived `TabRecord` type below.
const tabRecordSchema = /*#__PURE__*/ z.discriminatedUnion('kind', [
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('repo-graph'),
    state: repoGraphTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('repo-file'),
    state: repoFileTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('local-file'),
    state: localFileTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('repo-diff'),
    state: repoDiffTabStateSchema,
  }),
  /*#__PURE__*/ z.object({
    ...tabRecordBase,
    kind: z.literal('repo-multi-diff'),
    state: repoMultiDiffTabStateSchema,
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
]);
export type TabRecord = z.infer<typeof tabRecordSchema>;
export type RepoGraphTabRecord = Extract<TabRecord, { kind: 'repo-graph' }>;
export type RepoFileTabRecord = Extract<TabRecord, { kind: 'repo-file' }>;
export type LocalFileTabRecord = Extract<TabRecord, { kind: 'local-file' }>;
export type RepoDiffTabRecord = Extract<TabRecord, { kind: 'repo-diff' }>;
export type RepoMultiDiffTabRecord = Extract<TabRecord, { kind: 'repo-multi-diff' }>;

export function asRepoGraphTab(tab: TabRecord | null | undefined): RepoGraphTabRecord | null {
  return tab && tab.kind === 'repo-graph' ? tab : null;
}

export function asRepoFileTab(tab: TabRecord | null | undefined): RepoFileTabRecord | null {
  return tab && tab.kind === 'repo-file' ? tab : null;
}

export function asRepoDiffTab(tab: TabRecord | null | undefined): RepoDiffTabRecord | null {
  return tab && tab.kind === 'repo-diff' ? tab : null;
}

export function defaultRepoGraphTabState(): RepoGraphTabState {
  return { viewState: null, reviewSession: null };
}

export function defaultRepoFileTabState(
  revealLine: number | null = null,
  rev: string | null = null,
): RepoFileTabState {
  return { revealLine, markdownReading: false, rev };
}

export function defaultLocalFileTabState(): LocalFileTabState {
  return { revealLine: null, markdownReading: false };
}

/**
 * With no arguments, C6's own HEAD-vs-worktree comparison (every field null). C10's
 * `openRepoCommitDiffTab` is one caller that supplies the revision pair; C11's
 * `openRepoReviewDiffTab` (S8) is the one caller that also supplies `review`.
 */
export function defaultRepoDiffTabState(
  revision?: {
    left: string;
    right: string;
    leftLabel: string;
    rightLabel: string;
  },
  review?: { branch: string; branchTip: string; leftLabel: string },
  working?: { branchId: string },
): RepoDiffTabState {
  return {
    left: revision?.left ?? null,
    right: revision?.right ?? null,
    leftLabel: revision?.leftLabel ?? null,
    rightLabel: revision?.rightLabel ?? null,
    review: review ?? null,
    working: working ?? null,
  };
}

/** P92 item 5: `files` is captured at open time (the commit's own order) — the caller's job. */
export function defaultRepoMultiDiffTabState(
  files: string[],
  revision: { left: string; right: string; leftLabel: string; rightLabel: string },
  review?: ReviewRef,
): RepoMultiDiffTabState {
  return {
    left: revision.left,
    right: revision.right,
    leftLabel: revision.leftLabel,
    rightLabel: revision.rightLabel,
    files,
    review: review ?? null,
  };
}

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
