import type { ResultOf, ReviewComment } from '@kira/git-ipc';
import { t } from '@theme/i18n';
import { createLatestRequest, runLatest } from '@workbench/behaviour/latest';
import { errorText } from '@workbench/bridge/codedError';
import { type ShallowRef, shallowRef } from 'vue';
import type { BridgeClient } from '../bridge/client.ts';
import { copyToClipboard } from './clipboardActions.ts';
import { isCancelledTransport } from './isCancelled.ts';

/** The `(repoId, branch)` pair `review.comment.*` is keyed on (G13 D14) — no `base`, since the
 *  session key has none: a comment is a fact about a branch's own review, not about a comparison. */
export interface ReviewCommentsTarget {
  readonly repoId: string;
  readonly branch: string;
}

/**
 * G13 D10 — the review sidebar's Comments pane: a centralized, ordered list, the copy-for-AI
 * action, and clear-all. `ReviewFilesState`'s own supersede-and-verify discipline (`DetailState`'s,
 * really): every request checks its own target is still current before committing its result.
 */
export class ReviewCommentsState {
  readonly comments: ShallowRef<readonly ReviewComment[]> = shallowRef([]);
  readonly loading: ShallowRef<boolean> = shallowRef(false);
  readonly loadError: ShallowRef<string | undefined> = shallowRef(undefined);
  /** A remove/clear request in flight — the pane's delete/clear-all buttons disable themselves
   *  while true rather than let a double-click race two writes against the same session. */
  readonly pending: ShallowRef<boolean> = shallowRef(false);
  /** D10's inline two-step clear-all confirmation (`RefsPopover.vue`'s own `confirmForceDelete`
   *  pattern) — never a modal, never a single unconfirmed destructive click. */
  readonly confirmingClear: ShallowRef<boolean> = shallowRef(false);
  /** Fed into the shared live region for "Cleared 3 comments" / a copy or remove failure. */
  readonly announcement: ShallowRef<string> = shallowRef('');

  readonly #bridge: BridgeClient;
  readonly #unsubscribeChanged: () => void;
  #target: ReviewCommentsTarget | undefined;
  readonly #loadRequest =
    createLatestRequest<ResultOf<'review.comment.list'>>(isCancelledTransport);

  constructor(bridge: BridgeClient) {
    this.#bridge = bridge;
    // D10's own refresh list: "on repo.changed" — a landed commit can move every anchor (D7), so
    // the pane's projected ranges must not go stale silently.
    this.#unsubscribeChanged = bridge.on('repo.changed', (event) => {
      if (event.kind !== 'refsChanged') return;
      if (!this.#target || this.#target.repoId !== event.repoId) return;
      void this.#load();
    });
  }

  /** Called whenever the review session's own target changes — resets everything and, when a
   *  target is given, loads the list. */
  setTarget(target: ReviewCommentsTarget | undefined): void {
    this.#loadRequest.abort();
    this.#target = target;
    this.comments.value = [];
    this.loadError.value = undefined;
    this.confirmingClear.value = false;
    this.pending.value = false;
    if (target) void this.#load();
  }

  /** D10's own "on becoming the active pane" and "after every [editor-side] mutation"
   *  (`refreshReviewComments`, D19) — a plain re-fetch against the current target. */
  async reload(): Promise<void> {
    await this.#load();
  }

  async #load(): Promise<void> {
    const target = this.#target;
    if (!target) return;
    await runLatest(this.#loadRequest, {
      request: (signal) =>
        this.#bridge.request(
          'review.comment.list',
          { repoId: target.repoId, branch: target.branch },
          signal,
        ),
      stillCurrent: () => this.#target === target,
      onResult: (result) => {
        this.comments.value = result.comments;
      },
      onError: (message) => {
        this.loadError.value = message;
      },
      setLoading: (loading) => {
        this.loading.value = loading;
      },
    });
  }

  /** Removes one comment — applies the server's own scoped, idempotent delete (G13 D11) and drops
   *  it from the in-memory list directly, so the pane updates from the response rather than a
   *  second `review.comment.list` round trip. */
  async remove(id: string): Promise<void> {
    const target = this.#target;
    if (!target || this.pending.value) return;
    this.pending.value = true;
    try {
      await this.#bridge.request('review.comment.remove', {
        repoId: target.repoId,
        branch: target.branch,
        id,
      });
      if (this.#target !== target) return;
      this.comments.value = this.comments.value.filter((c) => c.id !== id);
    } catch (error) {
      if (this.#target === target) {
        this.announcement.value = errorText(error);
      }
    } finally {
      if (this.#target === target) this.pending.value = false;
    }
  }

  /** The clear-all button's first click. */
  confirmClear(): void {
    this.confirmingClear.value = true;
  }

  /** The clear-all confirmation's own cancel. */
  cancelClear(): void {
    this.confirmingClear.value = false;
  }

  /** The clear-all confirmation's own confirm — scoped to this session's comments only (G13 D14):
   *  never `review_file`/`review_range`, never the session row itself. */
  async clear(): Promise<void> {
    const target = this.#target;
    if (!target || this.pending.value) return;
    this.pending.value = true;
    try {
      const result = await this.#bridge.request('review.comment.clear', {
        repoId: target.repoId,
        branch: target.branch,
      });
      if (this.#target !== target) return;
      this.comments.value = [];
      this.announcement.value = t('git.review.comments.cleared.announce', {
        count: result.removed,
      });
    } catch (error) {
      if (this.#target === target) {
        this.announcement.value = errorText(error);
      }
    } finally {
      this.confirmingClear.value = false;
      if (this.#target === target) this.pending.value = false;
    }
  }

  /** `review.comment.export` -> the existing clipboard port (G13 D12) — the export text is
   *  produced entirely server-side; this is a fetch-then-copy, never a client-side render. */
  async copyForAi(): Promise<void> {
    const target = this.#target;
    if (!target) return;
    try {
      const result = await this.#bridge.request('review.comment.export', {
        repoId: target.repoId,
        branch: target.branch,
      });
      const outcome = await copyToClipboard(
        this.#bridge,
        result.text,
        t('git.copy.reviewComments.announce'),
      );
      this.announcement.value = outcome.message;
    } catch (error) {
      this.announcement.value = errorText(error);
    }
  }

  dispose(): void {
    this.#loadRequest.abort();
    this.#unsubscribeChanged();
  }
}
