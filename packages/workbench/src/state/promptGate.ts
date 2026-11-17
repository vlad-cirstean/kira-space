import { refAutoReset, useActiveElement, useEventListener } from '@vueuse/core';
import { defineStore } from 'pinia';
import { computed, type MaybeRefOrGetter, ref, toValue, watch } from 'vue';
import { isEditableTarget } from '../behaviour/dom';

/** Idle time after the last typed key before a prompt may take the screen: long enough to finish a
 *  sentence, short against the 10 min answer wait. */
export const PROMPT_QUIET_MS = 4000;

// Monaco 0.57 types through EditContext on Chromium, not an input; xterm's helper textarea is
// already an editable target, the class check is belt and braces.
function isTypingTarget(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  return isEditableTarget(el) || el.closest('.monaco-editor, .xterm') !== null;
}

// The typing gate: true while the user typed into a text surface a moment ago and still sits in it.
// Prompt dialogs wait for it to clear (usePromptGateOpen) so they never steal focus mid-sentence.
export const usePromptGateStore = defineStore('promptGate', () => {
  const activeElement = useActiveElement();
  const recent = refAutoReset(false, PROMPT_QUIET_MS);
  useEventListener(
    window,
    'keydown',
    (e: KeyboardEvent) => {
      if (isTypingTarget(e.target)) recent.value = true;
    },
    { capture: true, passive: true },
  );
  const typing = computed(() => recent.value && isTypingTarget(activeElement.value ?? null));
  return { typing };
});

/** `ready` turns true once `want` holds and the user is not typing, then stays true until `want`
 *  drops or changes, so typing inside the open prompt never closes it. `force` opens it now. */
export function useGatedOpen(want: MaybeRefOrGetter<unknown>) {
  const gate = usePromptGateStore();
  const ready = ref(false);
  watch(
    () => [toValue(want), gate.typing] as const,
    ([w, typing], old) => {
      if (!w || (old && old[0] !== w)) ready.value = false;
      if (w && !typing) ready.value = true;
    },
    { immediate: true },
  );
  function force(): void {
    if (toValue(want)) ready.value = true;
  }
  return { ready, force };
}
