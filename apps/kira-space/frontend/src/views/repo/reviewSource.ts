/**
 * Where the review layer (`reviewDecorations.ts`) reads and writes: one source per diff tab.
 * Committed sources talk to the git transport; the working source talks to the uncommitted-review
 * control methods. Both emit through `notifyReviewRepaint`, so every open surface reloads.
 */

import type {
  DiffHunk,
  FileDiffBody,
  LineRange,
  ResultOf,
  ReviewComment,
  ReviewCommentKind,
  ReviewCommentState,
  Transport,
} from '@kira/git-ipc';
import { t } from '@theme/i18n';
import { logError } from '@workbench/bridge/log';
import type { HostCommentTarget } from '../../ade/v2/review/useHostLineComment';
import { invalidateReviewWorking } from '../../ade/v2/review/useReviewWorking';
import type { ReviewWindowTarget } from '../../ade/v2/wire';
import { control } from '../../bridge/control';
import { notifyReviewRepaint, onReviewRepaint } from '../../repo/git/reviewRepaint';
import { gitTransportFor } from '../../repo/git/transport';

interface ReviewDiffLoad {
  readonly hunks: readonly DiffHunk[];
  readonly bodyKind: FileDiffBody['kind'];
  readonly reviewedRanges: readonly LineRange[];
  readonly lineCount: number;
}

export interface ReviewSource {
  readonly branch: string;
  /** Rejects on a failed load; resolves `null` when no base branch is configured. */
  loadDiff(): Promise<ReviewDiffLoad | null>;
  loadComments(): Promise<readonly ReviewComment[]>;
  mark(range: LineRange, reviewed: boolean): Promise<void>;
  addComment(range: LineRange, body: string, kind: ReviewCommentKind): Promise<void>;
  editComment(id: number, body: string, kind: ReviewCommentKind): Promise<void>;
  setCommentState(id: number, state: ReviewCommentState): Promise<void>;
  removeComment(id: number): Promise<void>;
  /** The change request a compose may post a line comment to; absent on a source that cannot. */
  hostComment?(): HostCommentTarget | null;
  /** Posts a line comment to the change request at the tab's commit. Kira keeps no copy. */
  postToChangeRequest?(range: LineRange, body: string): Promise<{ number: number; short: string }>;
  /** Fires `onDiff` when a mark of this source's file lands, `onComments` after a comment change. */
  subscribe(onDiff: () => void, onComments: () => void): () => void;
  dispose(): void;
}

export interface CommittedReviewDeps {
  readonly transport: Transport;
  readonly gitRepoId: string;
  readonly path: string;
  /** The tab's own left revision: merge base in `range` mode, the file's `reviewedAtSha` in
   *  `sinceReview` mode. A fresh diff whose `reviewedAtSha` differs reloads as `range`, so a tab
   *  never silently repaints as the other mode. */
  readonly leftRev: string;
  readonly review: {
    readonly branch: string;
    readonly branchTip: string;
    readonly leftLabel: string;
  };
  /** Set for a tab of the review window's own branch: the change request line comments go to. */
  readonly hostComment?: () => HostCommentTarget | null;
}

// `review.fileDiff` needs `base`, which a diff tab carries only as `leftRev`; re-resolving here,
// memoised by (repoId, branch), beats threading `base` through the whole tab-open path. Module
// level: two tabs for one branch share one resolution. Only a success stays cached — a transient
// failure (index.lock) must not disable the layer until restart. A rejection propagates, so it
// stays distinct from the permanent `base: null` answer.
const baseMemo = new Map<string, Promise<string | null>>();

function resolveBase(transport: Transport, repoId: string, branch: string): Promise<string | null> {
  const key = `${repoId}\0${branch}`;
  let cached = baseMemo.get(key);
  if (!cached) {
    cached = transport
      .request('review.resolveBase', { repoId, branch }, undefined)
      .then((r) => r.base)
      .catch((err) => {
        baseMemo.delete(key);
        logError('reviewSource', 'review.resolveBase failed', err);
        throw err;
      });
    baseMemo.set(key, cached);
  }
  return cached;
}

function toLoad(
  body: FileDiffBody,
  reviewedRanges: readonly LineRange[],
  lineCount: number,
): ReviewDiffLoad {
  return {
    hunks: body.kind === 'text' ? body.hunks : [],
    bodyKind: body.kind,
    reviewedRanges,
    lineCount,
  };
}

function idMutations(
  transport: () => Transport,
  gitRepoId: string,
  branch: string,
): Pick<ReviewSource, 'editComment' | 'setCommentState' | 'removeComment'> {
  return {
    async editComment(id, body, kind) {
      await transport().request('review.comment.edit', {
        repoId: gitRepoId,
        branch,
        id,
        body,
        kind,
      });
    },
    async setCommentState(id, state) {
      await transport().request('review.comment.setState', {
        repoId: gitRepoId,
        branch,
        id,
        state,
      });
    },
    async removeComment(id) {
      await transport().request('review.comment.remove', { repoId: gitRepoId, branch, id });
    },
  };
}

function subscribeRepaint(
  gitRepoId: string,
  branch: string,
  path: string,
  scope: 'commit' | 'working',
  onDiff: () => void,
  onComments: () => void,
): () => void {
  return onReviewRepaint((event) => {
    if (event.repoId !== gitRepoId || event.branch !== branch) return;
    if (event.kind === 'mark') {
      if (event.scope === scope && event.paths.includes(path)) onDiff();
    } else {
      onComments();
    }
  });
}

export function committedReviewSource(deps: CommittedReviewDeps): ReviewSource {
  const { transport, gitRepoId, path, review, hostComment } = deps;
  const { branch } = review;
  const host: Pick<ReviewSource, 'hostComment' | 'postToChangeRequest'> = hostComment
    ? {
        hostComment,
        async postToChangeRequest(range, body) {
          const target = hostComment();
          if (!target) throw new Error(t('space.ade.review.actions.errorDisabled.body'));
          const res = await transport.request('changeRequest.lineComment', {
            repoId: gitRepoId,
            number: target.number,
            path,
            range,
            headSha: review.branchTip,
            body,
          });
          switch (res.kind) {
            case 'ok':
              return { number: target.number, short: target.short };
            case 'disabled':
              throw new Error(t('space.ade.review.actions.errorDisabled.body'));
            case 'unavailable':
              throw new Error(res.status.reason);
            case 'headMoved':
              throw new Error(
                t('git.review.comments.host.headMoved.error', { short: target.short }),
              );
          }
        },
      }
    : {};
  return {
    branch,
    async loadDiff() {
      const base = await resolveBase(transport, gitRepoId, branch);
      if (base === null) return null;
      const fetchDiff = (mode: 'sinceReview' | 'range'): Promise<ResultOf<'review.fileDiff'>> =>
        transport.request('review.fileDiff', { repoId: gitRepoId, branch, base, path, mode });
      let result = await fetchDiff('sinceReview');
      if (result.reviewedAtSha !== null && result.reviewedAtSha !== deps.leftRev) {
        result = await fetchDiff('range');
      }
      return toLoad(result.body, result.reviewedRanges, result.lineCount);
    },
    async loadComments() {
      const { comments } = await transport.request('review.comment.list', {
        repoId: gitRepoId,
        branch,
        at: review.branchTip,
      });
      // The list spans every file of the branch and both origins; a stray one would paint as a
      // phantom glyph at the same line number here.
      return comments.filter((c) => c.path === path && c.origin !== 'working');
    },
    async mark(range, reviewed) {
      await transport.request('review.mark', {
        repoId: gitRepoId,
        branch,
        path,
        reviewed,
        ranges: [range],
      });
    },
    async addComment(range, body, kind) {
      await transport.request('review.comment.add', {
        repoId: gitRepoId,
        branch,
        path,
        at: review.branchTip,
        range,
        body,
        kind,
      });
    },
    ...idMutations(() => transport, gitRepoId, branch),
    ...host,
    subscribe: (onDiff, onComments) =>
      subscribeRepaint(gitRepoId, branch, path, 'commit', onDiff, onComments),
    dispose() {
      // This source leases the transport; nothing else holds it to release.
      transport.dispose();
    },
  };
}

export function workingReviewSource(args: {
  readonly target: ReviewWindowTarget;
  readonly path: string;
  /** Hash of the worktree text the editor shows right now; the server refuses a write if the file moved on. */
  readonly shownHash: () => string | undefined;
  /** A mark the server refused because the file changed; the view has been reloaded. */
  readonly onMarkRefused: () => void;
}): ReviewSource {
  const { target, path, shownHash, onMarkRefused } = args;
  const { gitRepoId, branch, branchId } = target;
  let transport: Transport | null = null;
  const lease = (): Transport => {
    transport ??= gitTransportFor(target.codeRepoId);
    return transport;
  };
  // On E_CONFLICT the diff query re-reads, which rewrites the editor's text, hunks and hash together.
  const isConflict = (err: unknown): boolean => (err as { code?: string }).code === 'E_CONFLICT';
  const reloadOnConflict = async (write: () => Promise<unknown>): Promise<void> => {
    try {
      await write();
    } catch (err) {
      if (isConflict(err)) void invalidateReviewWorking();
      throw err;
    }
  };
  return {
    branch,
    async loadDiff() {
      const r = await control.adeTaskReviewWorkingFile({ branchId, path });
      return toLoad(r.body, r.reviewedRanges, r.lineCount);
    },
    async loadComments() {
      const { comments } = await control.adeTaskReviewWorkingComments({ branchId });
      // A removed comment has no line here; the Comments pane still lists it.
      return comments.filter(
        (c) =>
          c.path === path &&
          c.origin === 'working' &&
          (c.anchor === 'exact' || c.anchor === 'projected'),
      );
    },
    async mark(range, reviewed) {
      const hash = shownHash();
      try {
        await reloadOnConflict(() =>
          control.adeTaskReviewWorkingMark({
            branchId,
            paths: [path],
            reviewed,
            ranges: [range],
            ...(hash === undefined ? {} : { contentHashes: { [path]: hash } }),
          }),
        );
      } catch (err) {
        if (isConflict(err)) onMarkRefused();
        throw err;
      }
      notifyReviewRepaint({
        kind: 'mark',
        scope: 'working',
        repoId: gitRepoId,
        branch,
        paths: [path],
      });
    },
    async addComment(range, body, kind) {
      await reloadOnConflict(() =>
        control.adeTaskReviewWorkingCommentAdd({
          branchId,
          path,
          range,
          body,
          kind,
          contentHash: shownHash(),
        }),
      );
      notifyReviewRepaint({ kind: 'comments', repoId: gitRepoId, branch });
    },
    ...idMutations(lease, gitRepoId, branch),
    subscribe: (onDiff, onComments) =>
      subscribeRepaint(gitRepoId, branch, path, 'working', onDiff, onComments),
    dispose() {
      transport?.dispose();
      transport = null;
    },
  };
}
