import type { HttpSavedRequest } from '@shared/domain/collections';
import { type Ref, shallowRef, watch } from 'vue';

// Decision 3: the saved request a tab last synced with, sent with Save as its guard. It follows
// the saved row while the tab holds no unsaved edit (or once a save made them equal); an edit open
// when another window saves keeps the older value, so Save asks before it overwrites that save.
export function useSeenSaved(
  saved: Ref<HttpSavedRequest | null>,
  edited: (against: HttpSavedRequest | null) => boolean,
): Ref<HttpSavedRequest | null> {
  const seen = shallowRef<HttpSavedRequest | null>(null);
  watch(
    saved,
    (next, prev) => {
      if (next === null) return;
      if (seen.value === null || !edited(prev ?? null) || !edited(next)) seen.value = next;
    },
    { immediate: true },
  );
  return seen;
}
