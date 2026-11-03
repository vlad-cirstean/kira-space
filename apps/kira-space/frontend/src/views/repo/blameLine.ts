/**
 * The cursor-line blame request lifecycle, extracted out of `blameAnnotation.ts`
 * so a second renderer (the status-bar widget, `RepoFileView.vue`) can share one request instead of
 * issuing its own. Cursor-line trigger, dedupe on the line (not the column), 150 ms debounce, an
 * `AbortController` per request, a per-mount cache and `repo.changed` invalidation.
 * `blameAnnotation.ts` keeps just the Monaco decoration/hover/reveal-action rendering of `state`
 * below.
 */
import type { ResultOf, Transport } from '@kira/git-ipc';
import { t } from '@theme/i18n';
import { useDebounceFn } from '@vueuse/core';
import { formatAbsoluteUtc, formatAgeCompact } from '@workbench/behaviour/format';
import { TIMING } from '@workbench/behaviour/timing';
import { type ShallowRef, shallowRef } from 'vue';
import { gitCall, onGitRepoEvent } from '../../repo/git/gitCall';
import { holdGitRepo } from '../../repo/git/repoHolds';

type CodeEditor = import('monaco-editor').editor.IStandaloneCodeEditor;

/** `blame.line`'s own sentinel for a line whose content isn't in any commit yet (an unstaged,
 *  on-disk edit) — the same literal `porcelain.UncommittedBlameSHA` and
 *  `blameState.ts`'s `UNCOMMITTED_BLAME_SHA` already carry. A third copy rather than a shared
 *  import: neither of those is reachable from this module (Go isn't importable; the extension's
 *  own `blameState.ts` is a different bundle) — `blame.go`'s own doc comment names this as the
 *  right call over inventing a boolean the wire does not have. */
const UNCOMMITTED_BLAME_SHA = '0000000000000000000000000000000000000000';

type BlameResult = ResultOf<'blame.line'>;

export type BlameLineState =
  | { readonly kind: 'none' }
  | { readonly kind: 'uncommitted'; readonly line: number }
  | {
      readonly kind: 'resolved';
      readonly line: number;
      readonly sha: string;
      readonly author: string;
      readonly authorTimeSeconds: number;
      readonly summary: string;
    };

/** Everything the controller needs off a Monaco editor — narrowed so a test can satisfy it
 *  without an editor (and without importing monaco-editor at all). */
type BlameCursorSource = Pick<CodeEditor, 'getPosition' | 'onDidChangeCursorPosition'>;

export interface BlameLineControllerDeps {
  readonly transport: Transport;
  readonly gitRepoId: string;
  /** Already repo-relative — `props.tab.path`, the same string `codeWorkspaceReadFile` reads. */
  readonly path: string;
  readonly cursor: BlameCursorSource;
}

export interface BlameLineController {
  readonly state: ShallowRef<BlameLineState>;
  /** Exposed so `blameAnnotation.ts`'s own reveal-in-graph editor action can call
   *  `graph.revealCommit` (host-local, so it needs the transport) without a second lease. */
  readonly gitRepoId: string;
  readonly transport: Transport;
  dispose(): void;
}

/** Injected text must be a single line (Monaco's own `InjectedTextOptions.content` doc comment) —
 *  a commit subject can't itself contain a newline after `--line-porcelain`, but this normalises
 *  whitespace and clamps length anyway rather than trusting that, so a pathological subject can
 *  never push the horizontal scrollbar out. */
function clampSubject(summary: string): string {
  const normalized = summary.replace(/\s+/g, ' ').trim();
  return normalized.length > 120 ? `${normalized.slice(0, 119)}…` : normalized;
}

function toState(line: number, result: BlameResult | null): BlameLineState {
  if (!result) return { kind: 'none' };
  if (result.sha === UNCOMMITTED_BLAME_SHA) return { kind: 'uncommitted', line };
  return {
    kind: 'resolved',
    line,
    sha: result.sha,
    author: result.author,
    authorTimeSeconds: result.authorTimeSeconds,
    summary: result.summary,
  };
}

/** `<author>, <age> · <subject>` — today's inline annotation text, verbatim. */
export function blameLineText(state: Extract<BlameLineState, { kind: 'resolved' }>): string {
  const subject = clampSubject(state.summary);
  const age = formatAgeCompact(state.authorTimeSeconds);
  return subject.length > 0
    ? t('space.repo.blame.line.label', { author: state.author, when: age, subject })
    : t('common.commaPair.label', { first: state.author, second: age });
}

/** The tooltip parts: the clamped subject (or a no-message placeholder), author and absolute date. */
export function blameLineTooltip(state: Extract<BlameLineState, { kind: 'resolved' }>): {
  subject: string;
  author: string;
  date: string;
} {
  const subject = clampSubject(state.summary);
  return {
    subject: subject.length > 0 ? subject : t('space.repo.blame.noMessage.label'),
    author: state.author,
    date: formatAbsoluteUtc(state.authorTimeSeconds),
  };
}

/** Resolves `blame.line` for the cursor's current line, debounced/cached/cancelled, and publishes
 *  the result to `state`. Call once per consuming mount; `dispose()` on unmount. Transport
 *  ownership: the caller leases `deps.transport` and is the only one who may release it — this
 *  controller disposes only what it created (timers, subscriptions, the in-flight abort), never a
 *  lease it did not take (`blameAnnotation.ts` is no longer the sole consumer of the transport it's
 *  handed). */
export function createBlameLineController(deps: BlameLineControllerDeps): BlameLineController {
  let disposed = false;
  // `blame.line` needs the window to hold the repo; the hold lives as long as this controller.
  const hold = holdGitRepo(deps.gitRepoId);
  const state = shallowRef<BlameLineState>({ kind: 'none' });

  // Dedupe on the line, not the column (a horizontal cursor move must not re-fire) — mirrors
  // `blameWidget.ts`'s own `lastLineKey`, minus its `isDirty` half: this view is
  // readOnly/domReadOnly and nothing in this app writes its model, so the buffer is always the
  // saved file — the dirty-buffer state is structurally absent here, not skipped.
  let lastLine: number | undefined;
  let inFlight: AbortController | undefined;
  // Per-mount cache: the model is immutable for the life of the mount, so (path, line) is a
  // stable key until `repo.changed` says otherwise. A cached `null` is a resolved miss
  // (untracked path, line past EOF, a real RPC error) — not retried on every revisit.
  //
  // Capped, not unbounded — holding the down-arrow through a
  // very large file could otherwise grow this to one entry per line in the file. A plain Map's
  // insertion order doubles as recency (touching a key deletes then re-inserts it), so the
  // least-recently-touched key is always whatever `cache.keys().next()` yields. A few thousand is a
  // generous cap for a per-mount line-blame cache — a session rarely visits more than a few hundred
  // distinct lines.
  const CACHE_LIMIT = 2000;
  const cache = new Map<number, BlameResult | null>();

  function touchCache(line: number, value: BlameResult | null): void {
    cache.delete(line);
    cache.set(line, value);
    if (cache.size <= CACHE_LIMIT) return;
    const oldest = cache.keys().next().value;
    if (oldest !== undefined) cache.delete(oldest);
  }

  function paint(line: number, result: BlameResult | null): void {
    if (disposed) return;
    state.value = toState(line, result);
  }

  // P99 §9.3: useDebounceFn replaces the hand-rolled clearTimeout/setTimeout pair this used to be —
  // resolveDebounced.cancel() below is cancelPending()'s own equivalent of the old clearTimeout.
  const resolveDebounced = useDebounceFn((line: number) => resolveLine(line), TIMING.search);

  function cancelPending(): void {
    resolveDebounced.cancel();
    inFlight?.abort();
    inFlight = undefined;
  }

  async function resolveLine(line: number): Promise<void> {
    try {
      await hold.ready;
    } catch {
      // §4.3: failure is silence — from the reader's own vantage this is an ordinary file.
      if (line === lastLine) paint(line, null);
      return;
    }
    if (disposed || line !== lastLine) return;

    const controller = new AbortController();
    inFlight = controller;
    try {
      const result = await gitCall(
        'blame.line',
        { repoId: deps.gitRepoId, path: deps.path, line },
        controller.signal,
      );
      touchCache(line, result);
      if (line === lastLine) paint(line, result);
    } catch {
      // 5b: an ABORTED request (cancelPending's own inFlight.abort(), or a fresh resolveLine for a
      // later line) is not a resolved miss — the cache's own doc comment says a cached `null` means
      // exactly that (untracked path, line past EOF, a genuine RPC error), never "we didn't wait to
      // find out". Caching it anyway combined with 5a's bug meant arrowing through lines faster than
      // responses land accumulated permanently blame-less lines that never retried, even on revisit.
      if (!controller.signal.aborted) touchCache(line, null);
      if (line === lastLine) paint(line, null);
    } finally {
      // Cleared on every settle (success or failure), not left
      // pointing at an already-settled controller — otherwise a later cancelPending() couldn't tell
      // "a request is genuinely still live" from "the last one already finished" (harmless in
      // practice, since aborting a settled controller is a no-op, but not a signal any future
      // reader of this file could trust). Guarded by identity in case a future change ever lets a
      // second resolveLine start before this one's own finally runs.
      if (inFlight === controller) inFlight = undefined;
    }
  }

  function refresh(): void {
    const line = deps.cursor.getPosition()?.lineNumber;
    // The `line === lastLine` early return must come BEFORE cancelPending — a same-line cursor
    // move (a horizontal arrow key, a column click, a reveal-selection) used to cancel the request
    // already running for THIS line before this check ever ran, and since lastLine already equalled
    // line, nothing restarted it: no annotation ever appeared for that line. This file's own nearby
    // comment already says the opposite is intended ("a horizontal cursor move must not re-fire").
    if (line === undefined || line === lastLine) return;
    cancelPending();
    lastLine = line;

    const cached = cache.get(line);
    if (cached !== undefined) {
      touchCache(line, cached); // rule 2's own re-resolving-refreshes-recency, mirrored on read too.
      paint(line, cached);
      return;
    }
    paint(line, null); // clear any stale state from the previous line while this one resolves.
    void resolveDebounced(line);
  }

  const cursorSub = deps.cursor.onDidChangeCursorPosition(() => refresh());

  // §4.3: re-resolve on any change but host data, which cannot change blame. Clearing `lastLine`
  // before `refresh()` re-triggers it even though the cursor hasn't moved.
  const unsubscribeRepoChanged = onGitRepoEvent('repo.changed', deps.gitRepoId, (event) => {
    if (event.kind === 'changeRequestsChanged') return;
    cache.clear();
    lastLine = undefined;
    refresh();
  });

  refresh();

  return {
    state,
    gitRepoId: deps.gitRepoId,
    transport: deps.transport,
    dispose(): void {
      disposed = true;
      cancelPending();
      cursorSub.dispose();
      unsubscribeRepoChanged();
      hold.release();
    },
  };
}
