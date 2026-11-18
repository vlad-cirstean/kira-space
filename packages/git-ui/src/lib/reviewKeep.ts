import { useKeptScroll, useKeptState } from '@workbench/behaviour/viewState';
import { KEPT_VIEW_STATE_KEY } from '@workbench/state/viewState';
import {
  computed,
  type InjectionKey,
  inject,
  type MaybeRefOrGetter,
  type Ref,
  ref,
  type ShallowRef,
  watch,
} from 'vue';
import type { ReviewSessionState } from '../state/review.ts';
import type { ReviewFilesState } from '../state/reviewFiles.ts';

/** View-state key prefix of a review mount whose host keeps its state (`MountOptions.keep`). */
export const REVIEW_KEEP_KEY: InjectionKey<string> = Symbol('reviewKeepKey');

/** Saves and restores an element's scroll under `key`; a no-op when the host keeps no state. */
export function keptScrollIf(
  key: MaybeRefOrGetter<string | null>,
  el: Readonly<Ref<HTMLElement | null | undefined>>,
  opts?: { ready?: () => boolean },
): void {
  if (inject(KEPT_VIEW_STATE_KEY, null)) useKeptScroll(key, el, opts);
}

/**
 * Review view state that survives an unmount (P660 Part 2): the file filter and its bar, per branch
 * the expanded rows, the selected file and the pane scroll keys. Plain refs and null keys without a
 * host `keep`.
 */
export function useReviewKeep(args: {
  review: ShallowRef<ReviewSessionState | undefined>;
  reviewFiles: ShallowRef<ReviewFilesState | undefined>;
}) {
  const prefix = inject(REVIEW_KEEP_KEY, null);
  const kept = inject(KEPT_VIEW_STATE_KEY, null);
  const filter: Ref<string> = prefix ? useKeptState(`${prefix}:filter`, '') : ref('');
  const filterVisible: Ref<boolean> = prefix
    ? useKeptState(`${prefix}:filterVisible`, false)
    : ref(false);
  const branchKey = computed(() => {
    const branch = args.review.value?.branch.value;
    return prefix && branch ? `${prefix}:b:${branch}` : null;
  });
  const scrollKeys = computed(() => {
    const key = branchKey.value;
    return key
      ? { commits: `${key}:commits`, files: `${key}:files`, comments: `${key}:comments` }
      : null;
  });

  if (prefix && kept) {
    // Rows arrive in chunks. Per key, `pending` holds kept shas not seen yet: each chunk expands the
    // ones that loaded, the stream's end drops the rest. Writes wait for the restore to start so
    // setTarget's clear never lands over the kept value, and carry `pending` so unseen rows survive.
    const pending = new Map<string, Set<string>>();
    // Rows already scanned per key; each chunk scans only the new rows.
    const scanned = new Map<string, number>();
    const selectedRestored = new Set<string>();
    const listing = () => args.review.value?.phase.value === 'listing';

    watch(
      () => args.review.value?.expandedShas.value,
      (shas) => {
        const key = branchKey.value;
        const left = key ? pending.get(key) : undefined;
        if (!key || !shas || !left || !listing()) return;
        kept.write(`${key}:expanded`, [...shas, ...left]);
      },
    );
    watch(
      [
        branchKey,
        () => args.review.value?.phase.value,
        () => args.review.value?.loadedRows.value,
        () => args.review.value?.streaming.value,
      ],
      ([key, phase, rows, streaming]) => {
        const review = args.review.value;
        if (!key || !review || phase !== 'listing' || !rows) return;
        let left = pending.get(key);
        if (!left) {
          left = new Set(kept.read<string[]>(`${key}:expanded`, []));
          pending.set(key, left);
          scanned.set(key, 0);
        }
        if (left.size > 0) {
          let i = scanned.get(key) ?? 0;
          if (rows < i) i = 0; // stream restarted with fewer rows
          for (; i < rows && left.size > 0; i++) {
            const sha = review.store.shaAt(i);
            if (left.delete(sha)) review.expand(sha);
          }
          scanned.set(key, i);
          if (!streaming) left.clear();
        }
      },
    );

    watch(
      () => args.reviewFiles.value?.selectedPath.value,
      (path) => {
        const key = branchKey.value;
        if (!key || !path || !selectedRestored.has(key)) return;
        kept.write(`${key}:selected`, path);
      },
    );
    watch(
      [branchKey, () => args.reviewFiles.value?.files.value.length, listing],
      ([key, count, isListing]) => {
        const files = args.reviewFiles.value;
        if (!key || !files || !count || !isListing || selectedRestored.has(key)) return;
        selectedRestored.add(key);
        const path = kept.read<string | null>(`${key}:selected`, null);
        if (files.selectedPath.value === null && path !== null) {
          if (files.files.value.some((e) => e.change.path === path)) files.followFile(path);
        }
      },
    );
  }

  /** Explicit leave: re-picking the branch starts clean. */
  function forgetBranch(): void {
    const key = branchKey.value;
    if (key) kept?.drop(`${key}:`);
  }

  return { filter, filterVisible, stateKey: prefix, scrollKeys, forgetBranch };
}
