import type { QuitStatus, RoutedPrompt } from '@shared/domain/prompts';
import { useMutation, useQuery } from '@tanstack/vue-query';
import { createSharedComposable, useWindowFocus } from '@vueuse/core';
import { queryClient } from '@workbench/state/queryClient';
import { type InjectionKey, inject, onScopeDispose, ref, watch } from 'vue';

// P246: client side of the Go prompt router (internal/prompts). Entries are metadata; each kind's
// own store holds the content and the answer.

/** The router calls and pushes one app's bridge provides (createCoreControl.ts). */
export interface PromptsControl {
  promptsList(): Promise<RoutedPrompt[]>;
  promptsClaim(id: string): Promise<void>;
  quitStatus(): Promise<QuitStatus>;
  quitConfirm(): Promise<void>;
  quitCancel(): Promise<void>;
  onPromptsChanged(cb: (prompts: RoutedPrompt[]) => void): () => void;
  onPromptsReveal(cb: (event: { id: string }) => void): () => void;
}

export const promptsControlKey: InjectionKey<PromptsControl> = Symbol('promptsControl');

export function usePromptsControl(): PromptsControl {
  const control = inject(promptsControlKey);
  if (!control) throw new Error('promptsControlKey is not provided');
  return control;
}

const PROMPTS_KEY = ['prompts'];

/** Every open prompt with its target window. A push replaces the cache; focus refetches. */
export function usePrompts() {
  const control = usePromptsControl();
  const query = useQuery(
    {
      queryKey: PROMPTS_KEY,
      queryFn: () => control.promptsList(),
    },
    queryClient,
  );
  const off = control.onPromptsChanged((list) => queryClient.setQueryData(PROMPTS_KEY, list));
  onScopeDispose(off);
  const focused = useWindowFocus();
  watch(focused, (f) => {
    if (f) void query.refetch();
  });
  return query;
}

/** What this window hides locally (Escape) or was told to show first (notification click). */
export const usePromptVisibility = createSharedComposable(() => {
  const hidden = ref<string[]>([]);
  const revealed = ref<string | null>(null);
  // Set by PromptHost: a prompt is on screen in this window.
  const shown = ref(false);

  function hide(id: string): void {
    if (!hidden.value.includes(id)) hidden.value = [...hidden.value, id];
    if (revealed.value === id) revealed.value = null;
  }

  function reveal(id: string): void {
    hidden.value = hidden.value.filter((h) => h !== id);
    revealed.value = id;
  }

  function prune(live: string[]): void {
    hidden.value = hidden.value.filter((h) => live.includes(h));
    if (revealed.value && !live.includes(revealed.value)) revealed.value = null;
  }

  return { hidden, revealed, shown, hide, reveal, prune };
});

/** Moves a prompt to this window and shows it, even after Escape hid it here. */
export function useClaimPrompt() {
  const control = usePromptsControl();
  const visibility = usePromptVisibility();
  return useMutation(
    {
      mutationKey: ['prompts', 'claim'],
      mutationFn: (id: string) => control.promptsClaim(id),
      onSuccess: (_void, id) => visibility.reveal(id),
    },
    queryClient,
  );
}
