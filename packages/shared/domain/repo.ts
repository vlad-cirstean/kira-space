import { z } from 'zod';
import { paletteColorSchema } from './color';

// C5 §3.2: a repo entry's own wire shape — a parallel list to ConnectionSummary, never an
// extension of it (§3.1's Grounding: a repository needs none of that schema's sixteen fields but
// `name`). Mirrors bridge/coderepos.go's RepoSummary field-for-field.
export const repoSummarySchema = /*#__PURE__*/ z.object({
  id: z.string(),
  name: z.string().min(1).max(120),
  root: z.string(),
  repoId: z.string(),
  sortOrder: z.number(),
  color: paletteColorSchema,
  createdAt: z.string(),
  hidden: z.boolean(),
});
export type RepoSummary = z.infer<typeof repoSummarySchema>;

// C5 §7.1: one `git status --porcelain=v2` snapshot taken alongside the listing, collapsed to the
// four-value glyph the tree renders — 'M' modified, 'A' added, 'D' deleted, '?' untracked. A path
// absent from `status` is unchanged.
export const fileStatusCodeSchema = /*#__PURE__*/ z.enum(['M', 'A', 'D', '?']);
export type FileStatusCode = z.infer<typeof fileStatusCodeSchema>;

export const fileListingSchema = /*#__PURE__*/ z.object({
  paths: z.array(z.string()),
  status: /*#__PURE__*/ z.record(z.string(), fileStatusCodeSchema),
  // §7.1: an honest cap on a single IPC payload (200,000 paths) — true means the tree is showing
  // a prefix of the repository, not the whole thing.
  truncated: z.boolean(),
});
export type FileListing = z.infer<typeof fileListingSchema>;

// Mirrors the git contract's own `file.read` result shape (found/tooLarge, packages/
// git-ipc/src/contract.ts) rather than inventing one, plus `missing` (a path git listed that no
// longer exists on disk) and `language` (this app's own id — resolved from the extension
// only, never sniffed; the binary check is the one place this phase *does* sniff bytes).
export const fileStatSchema = /*#__PURE__*/ z.object({ size: z.number(), hash: z.string() });
export type FileStat = z.infer<typeof fileStatSchema>;

export const fileContentSchema = /*#__PURE__*/ z.object({
  kind: /*#__PURE__*/ z.enum(['found', 'binary', 'tooLarge', 'missing']),
  text: z.string(),
  bytes: z.number(),
  limitBytes: z.number(),
  language: z.string(),
  stat: fileStatSchema.optional(),
});
export type FileContent = z.infer<typeof fileContentSchema>;

/** A read made with `ifStat` that still matched: no text, the caller keeps what it has. */
export type FileRead = FileContent | { kind: 'unchanged'; stat?: FileStat };

// ReadFileForEdit: FileContent's classification plus the on-disk content hash a save must present,
// and `notUtf8` for a found file whose bytes JSON cannot carry back unchanged.
export const editContentSchema = /*#__PURE__*/ z.object({
  kind: /*#__PURE__*/ z.enum(['found', 'binary', 'tooLarge', 'missing', 'notUtf8']),
  text: z.string(),
  hash: z.string(),
  language: z.string(),
});
export type EditContent = z.infer<typeof editContentSchema>;

export const writeFileResultSchema = /*#__PURE__*/ z.object({ hash: z.string() });
export type WriteFileResult = z.infer<typeof writeFileResultSchema>;

// One file an external open (Finder, a default-app double-click) brought to a window: `repoId` and
// `relPath` when it lies in an imported repository, else `path` (absolute) alone.
export interface OpenedFileTarget {
  path: string;
  repoId: string;
  relPath: string;
  // 1-based cursor position; absent when the open names none.
  line?: number;
  col?: number;
}

export interface OpenedFilesTerminalResolveArgs {
  terminalId: string;
  fallbackCwd: string;
  paths: string[];
}

export interface OpenedFilesTerminalOpenArgs {
  terminalId: string;
  fallbackCwd: string;
  path: string;
  line: number;
  col: number;
}

// Who opens one associated extension: Kira Space (`self`), another app (`other`), nobody (`none`),
// or a type the system does not know yet (`unavailable`, an app never registered with the OS).
export type FileHandlerState = 'self' | 'other' | 'none' | 'unavailable';

export interface FileHandlerApp {
  path: string;
  bundleId: string;
  name: string;
}

export interface FileHandlerExt {
  ext: string;
  uti: string;
  state: FileHandlerState;
  handler: FileHandlerApp | null;
}

export interface FileHandlerGroup {
  id: string;
  extensions: FileHandlerExt[];
}

export interface FileHandlersStatus {
  supported: boolean;
  groups: FileHandlerGroup[];
}

export interface FileHandlersSetResult {
  unrestored: string[];
}

// One side of a HEAD-vs-worktree diff — the same four-value classification
// fileContentSchema already uses for the worktree side, so the diff view reuses one vocabulary
// rather than inventing a second. No status field on the diff as a whole: the two sides already
// say what happened (HEAD missing means added, worktree missing means deleted, both present means
// modified).
export const diffSideSchema = /*#__PURE__*/ z.object({
  kind: /*#__PURE__*/ z.enum(['found', 'binary', 'tooLarge', 'missing']),
  text: z.string(),
  bytes: z.number(),
  limitBytes: z.number(),
  /** Worktree side only. */
  stat: fileStatSchema.optional(),
  /** HEAD side only: the blob id. */
  oid: z.string().optional(),
});
export type DiffSide = z.infer<typeof diffSideSchema>;

export const diffContentSchema = /*#__PURE__*/ z.object({
  path: z.string(),
  language: z.string(),
  head: diffSideSchema,
  worktree: diffSideSchema,
  /** Uncommitted review read only: identifies `worktree`, echoed on a mark or comment. */
  contentHash: z.string().optional(),
});
export type DiffContent = z.infer<typeof diffContentSchema>;

/** A side the caller's `ifStat` or `ifHeadOid` still matched: no text. */
export type DiffSideRead = DiffSide | { kind: 'unchanged'; stat?: FileStat; oid?: string };
export type DiffRead = Omit<DiffContent, 'head' | 'worktree'> & {
  head: DiffSideRead;
  worktree: DiffSideRead;
};

// C7 §3.1: a repository-wide search request — Monaco's own find-widget vocabulary (case/whole-word/
// regex), so the panel's own options read the same as the in-file find widget (D13).
export const searchRequestSchema = /*#__PURE__*/ z.object({
  query: z.string(),
  regex: z.boolean(),
  caseSensitive: z.boolean(),
  wholeWord: z.boolean(),
});
export type SearchRequest = z.infer<typeof searchRequestSchema>;

// One match within one file. line/column/endColumn are Monaco's own 1-based line and
// 1-based UTF-16 column (the identical rules internal/codeworkspace/textpos.go's LineIndex uses).
// preview is the line's own text (EOL stripped, possibly windowed) with
// previewMatchStart/End as UTF-16 offsets *into preview*, so the renderer highlights without
// re-deriving anything; truncatedStart/End say which side (if any) was windowed away — the string
// itself never carries an ellipsis, so a copied preview is always real file text.
export const searchMatchSchema = /*#__PURE__*/ z.object({
  line: z.number(),
  column: z.number(),
  endColumn: z.number(),
  preview: z.string(),
  previewMatchStart: z.number(),
  previewMatchEnd: z.number(),
  truncatedStart: z.boolean(),
  truncatedEnd: z.boolean(),
});
export type SearchMatch = z.infer<typeof searchMatchSchema>;

// C7 §3.1: one file's own group of matches, streamed as codeworkspace.Search finds them.
// truncated means this file alone hit MaxMatchesPerFile.
export const fileMatchesSchema = /*#__PURE__*/ z.object({
  path: z.string(),
  matches: z.array(searchMatchSchema),
  truncated: z.boolean(),
});
export type FileMatches = z.infer<typeof fileMatchesSchema>;

// C7 §3.1: one search run's own summary, reported in the panel's status line (D4: the skip count
// is surfaced, never silent) — truncated here means the *whole run* hit MaxSearchMatches and
// stopped early, distinct from a single FileMatches' own per-file truncated flag.
export const searchStatsSchema = /*#__PURE__*/ z.object({
  filesScanned: z.number(),
  filesMatched: z.number(),
  filesSkipped: z.number(),
  matches: z.number(),
  truncated: z.boolean(),
});
export type SearchStats = z.infer<typeof searchStatsSchema>;

// ChannelCodeSearch's own payload — one coalesced batch of file groups. seq is the index of
// the first file group in this batch (the renderer merges by path, not by seq, but the field stays
// for the same future-reviewer-detects-a-gap reason GrpcCallEvent's own seq exists);
// stats/error are set only on the terminal event (done: true), which always fires exactly once,
// even with nothing pending and even on cancel.
export const codeSearchEventSchema = /*#__PURE__*/ z.object({
  searchId: z.string(),
  seq: z.number(),
  files: z.array(fileMatchesSchema),
  done: z.boolean(),
  stats: searchStatsSchema.nullish(),
  error: /*#__PURE__*/ z.object({ code: z.string(), message: z.string() }).nullish(),
});
export type CodeSearchEvent = z.infer<typeof codeSearchEventSchema>;
