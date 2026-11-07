import { t } from '@theme/i18n';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, type Ref, ref, watch } from 'vue';
import type { ReviewWorkingSection } from '../state/review.ts';
import type { ReviewFilesState } from '../state/reviewFiles.ts';
import { orderedFilePaths } from './fileTreeModel.ts';
import { type ReviewSlot, reviewAttention, stepUnreviewed } from './reviewAttention.ts';

interface SteppingInput {
  reviewFiles: Readonly<Ref<ReviewFilesState | undefined>>;
  /** Committed paths in the order shown. */
  committedOrder: () => readonly string[];
  working: () => ReviewWorkingSection | null | undefined;
  filter: () => string;
  listMode: () => 'tree' | 'flat';
  /** Bind Alt+F8 / Shift+Alt+F8. */
  shortcuts: () => boolean;
  announce: (text: string) => void;
}

/** The review file list's cursor and its next/previous-unreviewed step: committed files in shown
 *  order, then uncommitted ones, wrapping. */
export function useReviewStepping(input: SteppingInput) {
  const cursor = ref<{ section: ReviewSlot['section']; path: string } | null>(null);
  watch(
    () => input.reviewFiles.value?.selectedPath.value,
    (path) => {
      if (path !== null && path !== undefined) cursor.value = { section: 'committed', path };
    },
  );

  const slots = computed<ReviewSlot[]>(() => {
    const committed = new Map(
      (input.reviewFiles.value?.files.value ?? []).map((e) => [e.change.path, e.review]),
    );
    const working = input.working();
    const out: ReviewSlot[] = input.committedOrder().map((path) => ({
      section: 'committed',
      path,
      attention: reviewAttention(committed.get(path)),
    }));
    for (const path of orderedFilePaths(working?.files ?? [], input.filter(), input.listMode())) {
      out.push({
        section: 'working',
        path,
        attention: reviewAttention(working?.reviewStates.get(path)),
      });
    }
    return out;
  });
  const canStep = computed(() => slots.value.some((s) => s.attention !== 'reviewed'));

  function step(dir: 'next' | 'previous'): void {
    const selected = input.reviewFiles.value?.selectedPath.value;
    const from =
      cursor.value ?? (selected ? { section: 'committed' as const, path: selected } : null);
    const slot = stepUnreviewed(slots.value, from, dir);
    if (!slot) {
      input.announce(t('git.review.allReviewed.announce'));
      return;
    }
    cursor.value = { section: slot.section, path: slot.path };
    if (slot.section === 'working') input.working()?.onOpen(slot.path, false);
    else input.reviewFiles.value?.selectFile(slot.path);
  }

  /** Moves the cursor to a file the host already shows; opens nothing. Unlisted paths are ignored. */
  function reveal(section: ReviewSlot['section'], path: string): void {
    if (!slots.value.some((s) => s.section === section && s.path === path)) return;
    if (section === 'committed') input.reviewFiles.value?.followFile(path);
    cursor.value = { section, path };
  }

  useShortcut(
    window,
    ['review.nextUnreviewed', 'review.previousUnreviewed'],
    (id) => step(id === 'review.nextUnreviewed' ? 'next' : 'previous'),
    { when: input.shortcuts, capture: true },
  );

  return { cursor, canStep, step, reveal };
}
