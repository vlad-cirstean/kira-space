/**
 * The review diff's own Monaco layer: gutter glyphs for hunk/comment state,
 * inline comment threads, and the "mark reviewed"/"add a comment" gestures. No precedent
 * elsewhere in this repo — everything here was designed against the
 * pinned `monaco-editor@0.56.0` source, not from memory.
 *
 * The whole reason this module computes no line mapping of its own: a review diff's right-hand
 * document is always `<branchTip>:<path>`, and `review.comment.list`/`review.fileDiff` already
 * return `range` in exactly those coordinates (anchored server-side, `gitsession/comments.go`) —
 * this module places a decoration at `comment.range.start` and renders `anchor` as a label, the
 * same two lines of logic `reviewComments.ts:63-74` already uses.
 *
 * The gutter has no glyph margin. The hovered line's number turns into the add mark and a commented
 * line's number is tinted (`lineNumberClassName`); a click on either number opens the composer or
 * the thread. Each hunk gets a one-line view zone above its first changed line whose margin node
 * holds the reviewed mark. View zones are not model lines, so every line coordinate stays put.
 */

import {
  clampRanges,
  coverage,
  hunkChangeBlock,
  normalizeRanges,
  selectionToRange,
} from '@kira/git-core';
import type {
  DiffHunk,
  FileDiffBody,
  LineRange,
  ReviewComment,
  ReviewCommentKind,
  ReviewCommentState,
} from '@kira/git-ipc';
import { i18n, t } from '@theme/i18n';
import { logError } from '@workbench/bridge/log';
import { type App, createApp, shallowRef } from 'vue';
import type { MonacoModule } from './monaco';
import ReviewBanner from './ReviewBanner.vue';
import ReviewHunkMark from './ReviewHunkMark.vue';
import ReviewThread from './ReviewThread.vue';
import type { ReviewSource } from './reviewSource';

type DiffEditor = import('monaco-editor').editor.IStandaloneDiffEditor;
type ModifiedEditor = import('monaco-editor').editor.IStandaloneCodeEditor;
type DeltaDecoration = import('monaco-editor').editor.IModelDeltaDecoration;
type ViewZone = import('monaco-editor').editor.IViewZone;
type OverlayWidget = import('monaco-editor').editor.IOverlayWidget;

// Pinned monaco-editor@0.56.0's own `view.js` appends `.view-zones` BEFORE `.view-lines` in
// `.lines-content`, and `viewLines.js` sizes `.view-lines` to the full scroll width/height with no
// z-index of its own (`position: absolute; z-index: auto`) — a later positioned sibling with
// `z-index: auto` loses every hit test to it. `10` is VS Code's own value for the view-zone
// widgets it mounts the same way; without it, a zone paints but nothing in it can ever be clicked.
const VIEW_ZONE_Z_INDEX = '10';

// A thread's height cap: the larger of 12 lines and 80% of the editor, as VS Code's ZoneWidget does.
const THREAD_MIN_LINES = 12;
const THREAD_MAX_FRACTION = 0.8;

export interface ReviewDecorationsHandle {
  /** Settles once the first diff and comment load finished (or failed). */
  readonly ready: Promise<void>;
  /** Re-reads the diff and the comments, e.g. after the file's models were rewritten. */
  reload(): void;
  dispose(): void;
}

function commentHoverMessage(comment: ReviewComment): { value: string } {
  const firstLine = comment.body.split('\n', 1)[0] ?? '';
  return { value: firstLine.length > 0 ? firstLine : '(empty comment)' };
}

/** Attaches the review layer to an already-mounted diff editor's modified pane. Call once per
 *  mount (`RepoDiffView.vue`, S11); `dispose()` on unmount, mirroring `registerDiffEditor`/
 *  `unmountEditor`'s own widget lifecycle (`editors.ts`) — never on a mere repaint. */
export function attachReviewDecorations(
  mod: MonacoModule,
  diffEditor: DiffEditor,
  source: ReviewSource,
): ReviewDecorationsHandle {
  const modifiedEditor: ModifiedEditor = diffEditor.getModifiedEditor();
  // §7.3 gotcha 1: the registered option default and the .d.ts prose disagree — set explicitly
  // rather than depend on either being right in a future Monaco bump. A click on a line number
  // adds or opens a comment, so it must not also select the line.
  modifiedEditor.updateOptions({ glyphMargin: false, selectOnLineNumbers: false });
  // Monaco forces a glyph margin onto the original pane side by side; switch it off again.
  const originalEditor = diffEditor.getOriginalEditor();
  const keepGlyphMarginOff = (): void => {
    if (originalEditor.getOption(mod.editor.EditorOption.glyphMargin)) {
      originalEditor.updateOptions({ glyphMargin: false });
    }
  };
  keepGlyphMarginOff();
  const glyphMarginDisposable = originalEditor.onDidChangeConfiguration((e) => {
    if (e.hasChanged(mod.editor.EditorOption.glyphMargin)) keepGlyphMarginOff();
  });

  let disposed = false;
  // Separate counters per resource -- loadDiff and loadComments used to share one loadSeq,
  // so a fast loadComments (single DB read) firing while a slower loadDiff (server-side git diff)
  // was still in flight bumped the shared counter and made the diff load discard its own result on
  // resolution, with nothing else left to re-fetch it. Each counter still guarantees its own "a
  // stale result never clobbers a fresher one" property; the two can no longer invalidate each
  // other.
  let diffSeq = 0;
  let commentsSeq = 0;

  let hunks: readonly DiffHunk[] = [];
  let bodyKind: FileDiffBody['kind'] = 'text';
  let reviewedRanges: readonly LineRange[] = [];
  let lineCount = 0;
  let comments: readonly ReviewComment[] = [];

  let commentByLine = new Map<number, ReviewComment>();

  const reviewedLineCollection = modifiedEditor.createDecorationsCollection([]);
  const commentCollection = modifiedEditor.createDecorationsCollection([]);
  const addGlyphCollection = modifiedEditor.createDecorationsCollection([]);

  let hoverLine: number | null = null;
  let openThreadCommentId: number | null = null;
  let closeThread: (() => void) | null = null;
  // The open thread's comment; a repaint swaps in the stored copy so a saved edit shows at once.
  const openComment = shallowRef<ReviewComment | undefined>();

  function closeZone(): void {
    const close = closeThread;
    closeThread = null;
    openThreadCommentId = null;
    openComment.value = undefined;
    close?.();
  }

  // VS Code's ZoneWidget mechanics over public API (`ZoneWidget` itself is not exported): an empty
  // view zone reserves the vertical space, the content lives in an overlay widget outside Monaco's
  // scrollable element. That keeps the content at the visible editor width, lets the wheel scroll
  // it, and lets its height be capped to the viewport.
  let threadSeq = 0;
  function openZone(
    afterLine: number,
    mount: (container: HTMLElement, onResize: (height: number) => void) => App,
  ): void {
    closeZone();
    const domNode = document.createElement('div');
    domNode.style.top = '-1000px';
    const widgetId = `kira.review.thread.${++threadSeq}`;
    const widget: OverlayWidget = {
      getId: () => widgetId,
      getDomNode: () => domNode,
      getPosition: () => null,
    };
    let contentHeight = 0;
    let zoneId = '';
    const zone: ViewZone = {
      afterLineNumber: afterLine,
      ordinal: 0,
      // A thread on the last visible line before a collapsed region would otherwise count as hidden.
      showInHiddenAreas: true,
      heightInPx: 120,
      domNode: document.createElement('div'),
      onDomNodeTop: (top) => {
        domNode.style.top = `${top}px`;
      },
      onComputedHeight: (height) => {
        domNode.style.height = `${height}px`;
      },
    };
    const layout = (): void => {
      const info = modifiedEditor.getLayoutInfo();
      domNode.style.left = '0px';
      domNode.style.width = `${info.width - info.minimap.minimapWidth - info.verticalScrollbarWidth}px`;
      const lineHeight = modifiedEditor.getOption(mod.editor.EditorOption.lineHeight);
      const cap = Math.max(THREAD_MIN_LINES * lineHeight, THREAD_MAX_FRACTION * info.height);
      const wanted = Math.ceil(Math.min(contentHeight || zone.heightInPx || 0, cap));
      if (wanted === zone.heightInPx) return;
      zone.heightInPx = wanted;
      modifiedEditor.changeViewZones((accessor) => accessor.layoutZone(zoneId));
    };
    modifiedEditor.addOverlayWidget(widget);
    modifiedEditor.changeViewZones((accessor) => {
      zoneId = accessor.addZone(zone);
    });
    const layoutDisposable = modifiedEditor.onDidLayoutChange(layout);
    layout();
    const app = mount(domNode, (height) => {
      contentHeight = height;
      layout();
    });
    closeThread = () => {
      layoutDisposable.dispose();
      app.unmount();
      modifiedEditor.removeOverlayWidget(widget);
      modifiedEditor.changeViewZones((accessor) => accessor.removeZone(zoneId));
    };
  }

  // resolveBase's own failure (a transient index.lock conflict, most likely) used to be
  // invisible — the diff still rendered, just with zero gutter glyphs/highlighting/comments and no
  // indication anything had gone wrong. A plain DOM view zone (no Vue mount needed, unlike
  // openZone/closeZone's own compose/thread box above) banners the real failure at the top of the
  // editor with a manual retry, independent of that zone's own lifecycle.
  let errorZoneId: string | null = null;
  let bannerApp: App | null = null;

  function closeErrorZone(): void {
    if (errorZoneId === null) return;
    const id = errorZoneId;
    modifiedEditor.changeViewZones((accessor) => accessor.removeZone(id));
    errorZoneId = null;
    bannerApp?.unmount();
    bannerApp = null;
  }

  // C13-9: one banner zone, two distinct uses — a genuine failure (network/server error, worth a
  // Retry that can plausibly succeed) and a legitimate, permanent condition (no base branch
  // configured for this repository, where Retry re-resolving the same answer would only mislead).
  function showBannerZone(message: string, retryable: boolean): void {
    closeErrorZone();
    const domNode = document.createElement('div');
    // §3.1: same paint-order defect as the compose zone above — Retry is otherwise unreachable.
    domNode.style.zIndex = VIEW_ZONE_Z_INDEX;
    bannerApp = createApp(ReviewBanner, { message, retryable, onRetry: () => void load() });
    bannerApp.use(i18n);
    bannerApp.mount(domNode);
    modifiedEditor.changeViewZones((accessor) => {
      errorZoneId = accessor.addZone({ afterLineNumber: 0, heightInPx: 40, domNode });
    });
  }

  function showLoadError(): void {
    showBannerZone(t('space.repo.reviewEditor.loadError.error'), true);
  }

  function showNoBaseConfigured(): void {
    showBannerZone(t('space.repo.reviewEditor.noBase.error'), false);
  }

  function paintAddGlyph(): void {
    if (hoverLine === null || commentByLine.has(hoverLine)) {
      addGlyphCollection.set([]);
      return;
    }
    addGlyphCollection.set([
      {
        range: new mod.Range(hoverLine, 1, hoverLine, 1),
        options: {
          lineNumberClassName: 'kira-review-ln kira-review-ln-add',
          lineNumberHoverMessage: { value: t('space.repo.reviewEditor.addComment.tooltip') },
        },
      },
    ]);
  }

  // One-line view zone per hunk, above its first changed line; the margin node holds the mark.
  let hunkRowKey = '';
  let hunkZoneIds: string[] = [];
  let hunkApps: App[] = [];

  function clearHunkRows(): void {
    const ids = hunkZoneIds;
    hunkZoneIds = [];
    for (const app of hunkApps) app.unmount();
    hunkApps = [];
    hunkRowKey = '';
    if (ids.length === 0) return;
    modifiedEditor.changeViewZones((accessor) => {
      for (const id of ids) accessor.removeZone(id);
    });
  }

  function paintHunkRows(rows: readonly { block: LineRange; reviewed: boolean }[]): void {
    const key = rows.map((r) => `${r.block.start}:${r.reviewed ? 1 : 0}`).join(',');
    if (key === hunkRowKey) return;
    clearHunkRows();
    hunkRowKey = key;
    if (rows.length === 0) return;
    modifiedEditor.changeViewZones((accessor) => {
      for (const { block, reviewed } of rows) {
        const marginDomNode = document.createElement('div');
        const app = createApp(ReviewHunkMark, {
          reviewed,
          onToggle: () => void mark(block, !reviewed),
        });
        app.use(i18n);
        app.mount(marginDomNode);
        hunkApps.push(app);
        hunkZoneIds.push(
          accessor.addZone({
            afterLineNumber: block.start - 1,
            heightInLines: 1,
            ordinal: 1,
            domNode: document.createElement('div'),
            marginDomNode,
            suppressMouseDown: true,
          }),
        );
      }
    });
  }

  function paint(): void {
    if (disposed) return;

    // P75 §5: a fully-reviewed file needs no per-line tint — the per-hunk glyph below and the file
    // tree's own checked box already say it, on every line at once. The tint earns its keep only
    // for a partial review, where it is the sole thing showing which lines are covered.
    const fullyReviewed =
      lineCount > 0 && coverage({ start: 1, end: lineCount }, reviewedRanges) === 'full';
    reviewedLineCollection.set(
      fullyReviewed
        ? []
        : normalizeRanges(reviewedRanges).map(
            (r): DeltaDecoration => ({
              range: new mod.Range(r.start, 1, r.end, 1),
              options: {
                isWholeLine: true,
                className: 'kira-review-line-reviewed',
                overviewRuler: {
                  color: 'var(--kira-ok)',
                  position: mod.editor.OverviewRulerLane.Left,
                },
              },
            }),
          ),
    );

    const hunkRows: { block: LineRange; reviewed: boolean }[] = [];
    if (bodyKind === 'text') {
      for (const hunk of hunks) {
        const block = hunkChangeBlock(hunk);
        if (block) hunkRows.push({ block, reviewed: coverage(block, reviewedRanges) === 'full' });
      }
    }
    paintHunkRows(hunkRows);

    const nextCommentByLine = new Map<number, ReviewComment>();
    for (const comment of comments) nextCommentByLine.set(comment.range.start, comment);
    commentByLine = nextCommentByLine;
    commentCollection.set(
      comments.map(
        (c): DeltaDecoration => ({
          range: new mod.Range(c.range.start, 1, c.range.start, 1),
          options: {
            lineNumberClassName: 'kira-review-ln kira-review-ln-comment',
            lineNumberHoverMessage: commentHoverMessage(c),
          },
        }),
      ),
    );
    paintAddGlyph();
    // A repaint can move or remove the line the open thread was anchored to (a mark/comment
    // mutation elsewhere, §7.6) — close it rather than leave it pointing at a stale line.
    if (openThreadCommentId !== null) {
      const fresh = comments.find((c) => c.id === openThreadCommentId);
      if (fresh) openComment.value = fresh;
      else closeZone();
    }
  }

  // onReviewRepaint's own fan-out (below) now dispatches by the repaint event's own `kind`
  // instead of always reloading both. A `mark` mutation can't change the comment list;
  // a `comments` mutation can't change the file diff — with N open review tabs on one branch,
  // reloading both on every mutation cost N redundant full diffs (a real server-side git diff,
  // review.fileDiff) plus N full branch-wide comment re-fetches (re-anchoring every comment on
  // the branch server-side) for every single comment click or mark toggle. Each now guards against
  // staleness with its own counter (diffSeq/commentsSeq) rather than one shared one — see those
  // declarations' own comment.

  // reviewMarking.ts:245-288's own resolution chain, ported: sinceReview first, retried as `range`
  // only when the file's own reviewedAtSha has moved past what this tab was opened against — the
  // same "never silently switch modes under an open tab" rule.
  // Returns whether the diff loaded — the load() uses this so a failed diff load
  // (any of the three early-return cases below) skips the now-separate loadComments() call too,
  // preserving the original combined function's own short-circuit: no point fetching a branch's
  // full comment list for an editor that has nothing to anchor it against and is showing an error
  // banner instead.
  async function loadDiff(): Promise<boolean> {
    const seq = ++diffSeq;
    let loaded: Awaited<ReturnType<ReviewSource['loadDiff']>>;
    try {
      loaded = await source.loadDiff();
    } catch (err) {
      if (disposed || seq !== diffSeq) return false;
      // A transient failure (index.lock, say) is not cached upstream, so Retry can succeed.
      logError('reviewDecorations', 'review diff load failed', err);
      showLoadError();
      return false;
    }
    if (disposed || seq !== diffSeq) return false;
    if (loaded === null) {
      // Permanent: no base branch configured. Retry would only re-resolve the same answer.
      showNoBaseConfigured();
      return false;
    }
    closeErrorZone();
    hunks = loaded.hunks;
    bodyKind = loaded.bodyKind;
    reviewedRanges = loaded.reviewedRanges;
    lineCount = loaded.lineCount;
    paint();
    return true;
  }

  // True while the shown banner is a failed comment load, so a later success clears only that.
  let commentsLoadFailed = false;

  // Monaco's hideUnchangedRegions feature unhides every line a selection touches
  // (hideUnchangedRegionsFeature.js, onDidChangeCursorPosition); no public API reveals one line.
  // It unhides contiguously from the nearer region edge to the line, so context around a comment
  // in the middle of a long unchanged stretch is wider than three lines.
  // The event fires only when the primary cursor moves, so alternate the column per call.
  let revealColumn = 1;
  function revealCommentLines(): void {
    const max = modifiedEditor.getModel()?.getLineCount() ?? 0;
    const lines = [...new Set(comments.map((c) => c.range.start))]
      .filter((l) => l >= 1 && l <= max)
      .sort((a, b) => a - b);
    const [first] = lines;
    if (first === undefined) return;
    revealColumn = revealColumn === 1 ? 2 : 1;
    const col = revealColumn;
    // Restore after: a repaint must not collapse the user's selection or move their cursor.
    const saved = modifiedEditor.getSelections();
    modifiedEditor.setSelections(lines.map((l) => new mod.Selection(l, col, l, col)));
    modifiedEditor.setSelection(new mod.Selection(first, col, first, col));
    if (saved?.length) modifiedEditor.setSelections(saved);
  }

  async function loadComments(): Promise<void> {
    const seq = ++commentsSeq;
    let loaded: readonly ReviewComment[];
    try {
      loaded = await source.loadComments();
    } catch (err) {
      if (disposed || seq !== commentsSeq) return;
      // Keep the comments already painted: a transient failure must not look like "all deleted".
      logError('reviewDecorations', 'review comment load failed', err);
      commentsLoadFailed = true;
      showLoadError();
      return;
    }
    if (disposed || seq !== commentsSeq) return;
    comments = loaded;
    if (commentsLoadFailed) {
      commentsLoadFailed = false;
      closeErrorZone();
    }
    paint();
    revealCommentLines();
  }

  /** The mount-time (and Retry-button) load only — reloads reachable from onReviewRepaint's own
   *  fan-out below call loadDiff/loadComments directly, whichever the mutation could have
   *  changed. Skips loadComments() when the diff itself never loaded (any of loadDiff's own three
   *  early-return cases) — no point fetching a branch's full comment list for an editor that is
   *  showing an error banner instead. */
  async function load(): Promise<void> {
    if (await loadDiff()) await loadComments();
  }

  async function mark(range: LineRange, reviewed: boolean): Promise<void> {
    const clamped = clampRanges([range], lineCount);
    if (clamped.length === 0) return;
    try {
      await source.mark(clamped[0] as LineRange, reviewed);
    } catch (err) {
      logError('reviewDecorations', 'review mark failed', err);
    }
    // No local prediction and no explicit reload: the source's repaint event reloads every open
    // tab on this file, this editor included.
  }

  function currentSelectionRange(): LineRange {
    const selection = modifiedEditor.getSelection();
    if (!selection) return { start: 1, end: 1 };
    return selectionToRange({
      start: { line: selection.startLineNumber - 1, character: selection.startColumn - 1 },
      end: { line: selection.endLineNumber - 1, character: selection.endColumn - 1 },
    });
  }

  function toggleThread(comment: ReviewComment): void {
    if (openThreadCommentId === comment.id) {
      closeZone();
      return;
    }
    openZone(comment.range.start, (container, onResize) => {
      const app = createApp(ReviewThread, {
        mode: 'view' as const,
        comment: openComment,
        branch: source.branch,
        onClose: () => closeZone(),
        onResize,
        onEdit: (body: string, kind: ReviewCommentKind) => handleEdit(comment, body, kind),
        onDelete: () => handleDelete(comment),
        onSetState: (state: ReviewCommentState) => handleSetState(comment, state),
      });
      app.use(i18n);
      app.mount(container);
      return app;
    });
    openThreadCommentId = comment.id;
    openComment.value = comment;
  }

  function openCompose(line: number): void {
    openZone(line, (container, onResize) => {
      const app = createApp(ReviewThread, {
        mode: 'compose' as const,
        branch: source.branch,
        host: source.hostComment?.() ?? null,
        onCancel: () => closeZone(),
        onClose: () => closeZone(),
        onResize,
        onSubmit: (body: string, kind: ReviewCommentKind) => handleSubmit(line, body, kind),
        onPostToHost: source.postToChangeRequest
          ? (body: string) => handlePostToHost(line, body)
          : undefined,
      });
      app.use(i18n);
      app.mount(container);
      return app;
    });
  }

  /** Runs one thread action. A failure is logged and rethrown for the thread to show inline. */
  async function runThreadAction(
    name: string,
    call: () => Promise<unknown>,
    closeAfter = true,
  ): Promise<void> {
    try {
      await call();
    } catch (err) {
      logError('reviewDecorations', `${name} failed`, err);
      throw err;
    }
    if (closeAfter) closeZone();
    // No explicit reload — see mark()'s own comment: transport.ts's repaint fan-out covers this
    // editor already (C12-4).
  }

  function handleSubmit(line: number, body: string, kind: ReviewCommentKind): Promise<void> {
    return runThreadAction('add comment', () =>
      source.addComment({ start: line, end: line }, body, kind),
    );
  }

  async function handlePostToHost(
    line: number,
    body: string,
  ): Promise<{ number: number; short: string }> {
    let posted = { number: 0, short: '' };
    await runThreadAction(
      'post to host',
      async () => {
        posted = await (
          source.postToChangeRequest as NonNullable<ReviewSource['postToChangeRequest']>
        )({ start: line, end: line }, body);
      },
      false,
    );
    return posted;
  }

  function handleEdit(
    comment: ReviewComment,
    body: string,
    kind: ReviewCommentKind,
  ): Promise<void> {
    return runThreadAction('edit comment', () => source.editComment(comment.id, body, kind), false);
  }

  function handleSetState(comment: ReviewComment, state: ReviewCommentState): Promise<void> {
    return runThreadAction('set comment state', () => source.setCommentState(comment.id, state));
  }

  function handleDelete(comment: ReviewComment): Promise<void> {
    return runThreadAction('remove comment', () => source.removeComment(comment.id));
  }

  const mouseDownDisposable = modifiedEditor.onMouseDown((e) => {
    if (e.target.type !== mod.editor.MouseTargetType.GUTTER_LINE_NUMBERS) return;
    const line = e.target.position?.lineNumber;
    if (line === undefined) return;
    const comment = commentByLine.get(line);
    if (comment) toggleThread(comment);
    else if (hoverLine === line) openCompose(line);
  });

  const mouseMoveDisposable = modifiedEditor.onMouseMove((e) => {
    const line = e.target.position?.lineNumber ?? null;
    if (line === hoverLine) return;
    hoverLine = line;
    paintAddGlyph();
  });

  const mouseLeaveDisposable = modifiedEditor.onMouseLeave(() => {
    hoverLine = null;
    paintAddGlyph();
  });

  // §7.4: the context-menu/keybinding route to the same two gestures a glyph click already gives —
  // OQ3's "keep both".
  const addCommentAction = modifiedEditor.addAction({
    id: 'kira.review.addComment',
    label: t('space.repo.reviewEditor.addComment.label'),
    contextMenuGroupId: 'kiraReview',
    run: () => {
      const line =
        modifiedEditor.getSelection()?.positionLineNumber ??
        modifiedEditor.getPosition()?.lineNumber;
      if (line !== undefined) openCompose(line);
    },
  });
  const markReviewedAction = modifiedEditor.addAction({
    id: 'kira.review.markReviewed',
    label: t('space.repo.reviewEditor.markReviewed.label'),
    contextMenuGroupId: 'kiraReview',
    run: () => void mark(currentSelectionRange(), true),
  });
  const markUnreviewedAction = modifiedEditor.addAction({
    id: 'kira.review.markUnreviewed',
    label: t('space.repo.reviewEditor.markUnreviewed.label'),
    contextMenuGroupId: 'kiraReview',
    run: () => void mark(currentSelectionRange(), false),
  });

  // A mutation from either half of the UI (this editor or the panel's Comments pane) reloads
  // only what it could have changed: a mark the diff, a comment change the comments.
  const unsubscribeRepaint = source.subscribe(
    () => void loadDiff(),
    () => void loadComments(),
  );

  // Regions rebuild on every diff compute, which can re-collapse lines revealed earlier.
  const diffUpdateDisposable = diffEditor.onDidUpdateDiff(revealCommentLines);

  const ready = load().catch((err: unknown) =>
    logError('reviewDecorations', 'initial load failed', err),
  );

  return {
    ready,
    reload(): void {
      void load().catch((err: unknown) => logError('reviewDecorations', 'reload failed', err));
    },
    dispose(): void {
      disposed = true;
      diffUpdateDisposable.dispose();
      glyphMarginDisposable.dispose();
      unsubscribeRepaint();
      closeZone();
      closeErrorZone();
      mouseDownDisposable.dispose();
      mouseMoveDisposable.dispose();
      mouseLeaveDisposable.dispose();
      addCommentAction.dispose();
      markReviewedAction.dispose();
      markUnreviewedAction.dispose();
      reviewedLineCollection.clear();
      clearHunkRows();
      commentCollection.clear();
      addGlyphCollection.clear();
      source.dispose();
    },
  };
}
