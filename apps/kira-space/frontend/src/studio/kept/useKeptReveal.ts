import type { RevealResult } from '@shared/domain/variables';
import { t } from '@theme/i18n';
import { useTimeoutFn } from '@vueuse/core';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed, onScopeDispose, ref } from 'vue';
import { runReveal } from '../api/reveal';

const REVEAL_MS = 60_000;

/**
 * Shows a secret kept value on demand behind the OS gate. The plaintext lives only in this ref:
 * never in TanStack or Pinia. It hides after a minute and when the view goes away.
 */
export function useKeptReveal(call: (confirmed: boolean) => Promise<RevealResult>) {
  const value = ref<string | null>(null);
  const refusal = ref('');
  const timer = useTimeoutFn(hide, REVEAL_MS, { immediate: false });
  // Resolves to nothing: the mutation's `data` would keep the plaintext past hide().
  // hide() bumps it: a reveal still waiting on the gate or the dialog then settles into nothing.
  let generation = 0;
  const action = useAsyncAction(async (started: number) => {
    await runReveal(
      call,
      (v) => {
        if (started !== generation) return;
        value.value = v;
        timer.start();
      },
      (message) => {
        if (started !== generation) return;
        refusal.value = message;
      },
      t('studio.kept.reveal.body'),
    );
  });

  function hide(): void {
    generation++;
    value.value = null;
    timer.stop();
    action.reset();
  }

  async function reveal(): Promise<void> {
    refusal.value = '';
    await action.run(generation);
  }

  onScopeDispose(hide);
  return {
    value,
    error: computed(() => refusal.value || action.error.value || ''),
    busy: action.pending,
    reveal,
    hide,
  };
}
