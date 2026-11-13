import {
  createSharedComposable,
  useDocumentVisibility,
  useTimeoutFn,
  useWindowFocus,
} from '@vueuse/core';
import { computed, type MaybeRefOrGetter, ref, toValue, watch } from 'vue';
import { useMarkSessionSeen } from '../queries';

/** Dwell before a session counts as viewed once its pane is shown. */
const SEEN_DWELL_MS = 500;

// One listener pair for every pane and row, not one per mounted session.
const useWindowPresence = createSharedComposable(() => ({
  visibility: useDocumentVisibility(),
  focused: useWindowFocus(),
}));

/** Marks a finished session viewed. The user views it while `active` (its pane or log is shown) in
 *  a visible, focused window. A pane shown after the finish waits out a short dwell; a pane already
 *  watched when the session finishes is marked at once. */
export function useMarkSeenOnView(
  sessionId: MaybeRefOrGetter<string>,
  unseen: MaybeRefOrGetter<boolean>,
  active: MaybeRefOrGetter<boolean> = true,
): void {
  const { visibility, focused } = useWindowPresence();
  const mark = useMarkSessionSeen();
  const present = computed(
    () => toValue(active) && visibility.value === 'visible' && focused.value,
  );

  // A failed mark rolls the cached flag back, which would re-trigger the mark at once: stop retrying
  // until the user leaves the pane and returns.
  const failed = ref(false);
  const send = (): void =>
    mark.mutate(
      { sessionId: toValue(sessionId) },
      {
        onError: () => {
          failed.value = true;
        },
      },
    );
  const dwell = useTimeoutFn(send, SEEN_DWELL_MS, { immediate: false });

  watch(
    [() => toValue(unseen), present],
    ([isUnseen, isPresent], prev) => {
      if (!isPresent) failed.value = false;
      if (!(isUnseen && isPresent) || failed.value) {
        dwell.stop();
        return;
      }
      const [wasUnseen, wasPresent] = prev ?? [false, false];
      if (!wasUnseen && wasPresent) {
        dwell.stop();
        send();
      } else dwell.start();
    },
    { immediate: true },
  );
}
