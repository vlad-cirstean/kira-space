// The completion shapes `MonacoHost.vue`'s `completionSources` prop carries — pure data
// in, pure data out, the same discipline `lintSource`/`rangeHighlights` already keep. Replaces
// `@codemirror/autocomplete`'s `CompletionSource`/`Completion` at this one seam; every source this
// app registers already produces exactly this shape (a synchronous lookup over a static array or
// an already-loaded list), so porting a source is a type-only change, never a behavioural one.

/** `Completion.type` (CodeMirror) -> the six kinds this app's own sources ever produce, mapped to
 *  `CompletionItemKind.Variable/Method/Function/Class/Keyword/Property` at the Monaco boundary
 *  (`MonacoHost.vue`'s own provider glue) rather than at each source. */
export type EditorCompletionKind =
  | 'variable'
  | 'method'
  | 'function'
  | 'class'
  | 'keyword'
  | 'property';

export interface EditorCompletion {
  label: string;
  /** Text actually inserted, when it differs from `label` (a quoted identifier, `ddl.ts`'s own
   *  `Completion.apply`). Defaults to `label`. */
  insert?: string;
  detail?: string;
  type?: EditorCompletionKind;
  /** Higher sorts first among otherwise-equal matches (PK columns first, `ddl.ts:369`) — mapped to
   *  a `sortText` prefix at the Monaco boundary rather than compared here. */
  boost?: number;
  /** A snippet body using `$0` for the final cursor stop (the six BSON constructors,
   *  `completion.ts:24`) — `insertTextRules: InsertAsSnippet` at the Monaco boundary when set. */
  snippet?: string;
}

export interface EditorCompletionResult {
  /** Offset the replacement range starts at — the option's own `insert`/`label` replaces
   *  `[from, ctx.offset)`. */
  from: number;
  options: readonly EditorCompletion[];
  /** The list is partial: Monaco asks the source again as the word grows. */
  incomplete?: boolean;
}

interface EditorCompletionContext {
  doc: string;
  offset: number;
  explicit: boolean;
}

/** A completion source: pure text + cursor offset in, a replacement range and options out (or
 *  `null` for "no completions here"). `explicit` is true for an explicit trigger (Ctrl+Space) —
 *  every current source ignores it, kept only so a future source can tell the difference without a
 *  signature change. */
export type EditorCompletionSource = (
  ctx: EditorCompletionContext,
) => EditorCompletionResult | null;

/** Like a source, but resolves later (server-backed values); `signal` aborts when Monaco cancels. */
export type EditorAsyncCompletionSource = (
  ctx: EditorCompletionContext,
  signal: AbortSignal,
) => Promise<EditorCompletionResult | null>;
