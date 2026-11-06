import { defineStore } from 'pinia';
import { ref } from 'vue';

export type ProofSide = 'current' | 'pending';

/** The proof window's own selection: which session, which revision, which step tab. */
export const useProofWindowStore = defineStore('proofWindow', () => {
  const sessionId = ref<string | null>(null);
  /** `null` follows the default: the pending revision when one exists, else the current one. */
  const side = ref<ProofSide | null>(null);
  const compare = ref(false);
  /** `${rev}:${ordinal}`; `null` follows the first tab. */
  const activeId = ref<string | null>(null);
  /** Step keys whose grid and query box have restored their saved view and painted. */
  const ready = ref<Record<string, true>>({});

  function selectSession(id: string | null): void {
    if (sessionId.value === id) return;
    sessionId.value = id;
    side.value = null;
    compare.value = false;
    activeId.value = null;
  }

  function markReady(key: string): void {
    ready.value = { ...ready.value, [key]: true };
  }

  function unmarkReady(key: string): void {
    const { [key]: _gone, ...rest } = ready.value;
    ready.value = rest;
  }

  return { sessionId, side, compare, activeId, ready, selectSession, markReady, unmarkReady };
});
