/**
 * Review repaint fan-out: after a review write succeeds, every open review surface repaints,
 * whichever half of the UI (or which caller) made the write. `repo.changed` needs no entry here; it
 * arrives as an ordinary Go event. Module-level, not per-transport: a listener filters by its own
 * `(repoId, branch)` or `path`, so one registry serves every open repo workspace.
 */
import type { RequestKey } from '@kira/git-ipc';

export type ReviewRepaintEvent =
  | { readonly kind: 'comments'; readonly repoId: string; readonly branch: string }
  | {
      readonly kind: 'mark';
      /** `commit`: the committed review; `working`: the uncommitted one (same path, other marks). */
      readonly scope: 'commit' | 'working';
      readonly repoId: string;
      readonly branch: string;
      readonly paths: readonly string[];
    };

const reviewRepaintListeners = new Set<(event: ReviewRepaintEvent) => void>();

/** Registered by `views/repo/reviewDecorations.ts`, one per mounted review diff editor. */
export function onReviewRepaint(listener: (event: ReviewRepaintEvent) => void): () => void {
  reviewRepaintListeners.add(listener);
  return () => {
    reviewRepaintListeners.delete(listener);
  };
}

export function notifyReviewRepaint(event: ReviewRepaintEvent): void {
  for (const listener of reviewRepaintListeners) listener(event);
}

const COMMENT_MUTATION_METHODS: ReadonlySet<RequestKey> = new Set([
  'review.comment.add',
  'review.comment.remove',
  'review.comment.setState',
  'review.comment.edit',
  'review.comment.clear',
]);

interface CommentsRefresh {
  readonly emit: () => void;
  readonly holds: (repoId: string) => boolean;
}

const commentsRefreshes = new Set<CommentsRefresh>();

/** A transport's `ui.action refreshReviewComments` emitter, called for writes on repos it holds. */
export function registerCommentsRefresh(
  emit: () => void,
  holds: (repoId: string) => boolean,
): () => void {
  const entry = { emit, holds };
  commentsRefreshes.add(entry);
  return () => {
    commentsRefreshes.delete(entry);
  };
}

/** The repaint rule for a review write that just succeeded; a no-op for any other method. */
export function afterReviewWrite(method: RequestKey, params: unknown): void {
  if (method === 'review.mark' || method === 'review.markMany') {
    const p = params as {
      repoId: string;
      branch: string;
      path?: string;
      paths?: readonly string[];
    };
    const paths = p.paths ?? (p.path === undefined ? [] : [p.path]);
    notifyReviewRepaint({
      kind: 'mark',
      scope: 'commit',
      repoId: p.repoId,
      branch: p.branch,
      paths,
    });
    return;
  }
  if (COMMENT_MUTATION_METHODS.has(method)) {
    const { repoId, branch } = params as { repoId: string; branch: string };
    notifyReviewRepaint({ kind: 'comments', repoId, branch });
    for (const entry of [...commentsRefreshes]) if (entry.holds(repoId)) entry.emit();
  }
}
