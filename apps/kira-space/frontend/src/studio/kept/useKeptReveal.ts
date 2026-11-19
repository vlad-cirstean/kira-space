import { t } from '@theme/i18n';
import { useTimeoutFn } from '@vueuse/core';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed, onScopeDispose, ref } from 'vue';
import { runReveal } from '../api/reveal';
import { control } from '../bridge/control';

const REVEAL_MS = 60_000;

/**
 * Shows a secret kept value on demand behind the OS gate. The plaintext lives only in this ref:
 * never in TanStack or Pinia. It hides after a minute and when the view goes away.
 */
export function useKeptReveal(id: () => string) {
  const value = ref<string | null>(null);
  const refusal = ref('');
  const timer = useTimeoutFn(hide, REVEAL_MS, { immediate: false });
  // Resolves to nothing: the mutation's `data` would keep the plaintext past hide().
  const action = useAsyncAction(async () => {
    await runReveal(
      (confirmed) => control.keptReveal(id(), confirmed),
      (v) => {
        value.value = v;
        timer.start();
      },
      (message) => {
        refusal.value = message;
      },
      t('studio.kept.reveal.body'),
    );
  });

  function hide(): void {
    value.value = null;
    timer.stop();
    action.reset();
  }

  async function reveal(): Promise<void> {
    refusal.value = '';
    await action.run();
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
