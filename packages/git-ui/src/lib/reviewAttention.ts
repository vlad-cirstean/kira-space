import type { ReviewFileStatus } from '@kira/git-ipc';
import type { MessageKey } from '@theme/i18n';

export type ReviewAttention = 'unreviewed' | 'partial' | 'reviewed' | 'changed';

/** What a file needs: a recorded review whose content moved since is `changed`, whatever its kind. */
export function reviewAttention(s: ReviewFileStatus | undefined): ReviewAttention {
  if (!s || s.kind === 'none') return 'unreviewed';
  if (s.changedSinceReview) return 'changed';
  return s.kind === 'full' ? 'reviewed' : 'partial';
}

/** The state a checkbox click sets: a reviewed or partial file unmarks, anything else marks. */
export function markOnToggle(a: ReviewAttention): boolean {
  return a === 'unreviewed' || a === 'changed';
}

export function checkboxState(a: ReviewAttention): boolean | 'indeterminate' {
  if (a === 'reviewed') return true;
  return a === 'partial' ? 'indeterminate' : false;
}

export function toggleTitleKey(a: ReviewAttention): MessageKey {
  if (a === 'changed') return 'git.review.markAgain.tooltip';
  return a === 'unreviewed' ? 'common.markReviewed.tooltip' : 'common.markUnreviewed.tooltip';
}

/** Tone of a row's file name: changed stands out, reviewed recedes. */
export function nameTone(a: ReviewAttention): string {
  if (a === 'changed') return 'text-warn';
  return a === 'reviewed' ? 'text-muted-foreground' : '';
}

export interface ReviewSlot {
  readonly section: 'committed' | 'working';
  readonly path: string;
  readonly attention: ReviewAttention;
}

/** The next (previous) slot needing attention after (before) the cursor, wrapping; the first (last)
 *  one when the cursor is null or not listed; null when none needs attention. */
export function stepUnreviewed(
  order: readonly ReviewSlot[],
  cursor: { readonly section: ReviewSlot['section']; readonly path: string } | null,
  dir: 'next' | 'previous',
): ReviewSlot | null {
  const needs = (s: ReviewSlot): boolean => s.attention !== 'reviewed';
  if (!order.some(needs)) return null;
  const at = cursor
    ? order.findIndex((s) => s.section === cursor.section && s.path === cursor.path)
    : -1;
  const n = order.length;
  const step = dir === 'next' ? 1 : -1;
  const start = at === -1 ? (dir === 'next' ? -1 : n) : at;
  for (let i = 1; i <= n; i++) {
    const slot = order[(((start + step * i) % n) + n) % n];
    if (slot && needs(slot)) return slot;
  }
  return null;
}
