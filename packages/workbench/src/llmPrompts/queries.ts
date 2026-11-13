import type {
  LlmPromptChange,
  LlmPromptKind,
  LlmPromptList,
  LlmPromptPreview,
  LlmPromptPreviewArgs,
  LlmPromptsChangedEvent,
} from '@shared/domain/llmPrompts';
import { keepPreviousData, useMutation, useQuery } from '@tanstack/vue-query';
import { refDebounced } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { queryClient } from '@workbench/state/queryClient';
import {
  computed,
  type InjectionKey,
  inject,
  type MaybeRefOrGetter,
  onScopeDispose,
  toValue,
} from 'vue';

// Client side of the editable LLM prompt registry (internal/llmprompts). The list is server state;
// a push from either window or an external edit of the overrides file invalidates it.

/** The calls and push one app's bridge provides (createCoreControl.ts). */
export interface LlmPromptsControl {
  llmPromptsList(): Promise<LlmPromptList>;
  llmPromptsSave(changes: LlmPromptChange[]): Promise<LlmPromptList>;
  llmPromptsPreview(args: LlmPromptPreviewArgs): Promise<LlmPromptPreview>;
  onLlmPromptsChanged(cb: (event: LlmPromptsChangedEvent) => void): () => void;
}

export const llmPromptsControlKey: InjectionKey<LlmPromptsControl> = Symbol('llmPromptsControl');

export function useLlmPromptsControl(): LlmPromptsControl {
  const control = inject(llmPromptsControlKey);
  if (!control) throw new Error('llmPromptsControlKey is not provided');
  return control;
}

const LIST_KEY = ['llmPrompts'];

/** Catalog key of a kind's explanation. */
export function llmPromptKindKey(kind: LlmPromptKind): string {
  return `llmPrompts.kind.${kind}.description`;
}

/** Catalog key of a variable's explanation; `name` is the Go field name. */
export function llmPromptVarKey(name: string): string {
  return `llmPrompts.var.${name.charAt(0).toLowerCase()}${name.slice(1)}.description`;
}

/** This app's prompts with their store state. */
export function useLlmPrompts() {
  const control = useLlmPromptsControl();
  const query = useQuery(
    { queryKey: LIST_KEY, queryFn: () => control.llmPromptsList() },
    queryClient,
  );
  const off = control.onLlmPromptsChanged(() => {
    void queryClient.invalidateQueries({ queryKey: LIST_KEY });
  });
  onScopeDispose(off);
  return query;
}

/** Renders a draft; the text is debounced so typing does not render per keystroke. */
export function useLlmPromptPreview(args: MaybeRefOrGetter<LlmPromptPreviewArgs | null>) {
  const control = useLlmPromptsControl();
  const text = refDebounced(
    computed(() => toValue(args)?.text),
    TIMING.preview,
  );
  const current = computed(() => {
    const a = toValue(args);
    return a ? { ...a, text: text.value } : null;
  });
  return useQuery(
    {
      queryKey: computed(() => [...LIST_KEY, 'preview', current.value]),
      queryFn: () => {
        const a = current.value;
        if (!a) throw new Error('no prompt to preview');
        return control.llmPromptsPreview(a);
      },
      enabled: computed(() => current.value !== null),
      placeholderData: keepPreviousData,
    },
    queryClient,
  );
}

/** Saves a batch all or nothing; the list is replaced with the result. */
export function useSaveLlmPrompts() {
  const control = useLlmPromptsControl();
  return useMutation(
    {
      mutationKey: [...LIST_KEY, 'save'],
      mutationFn: (changes: LlmPromptChange[]) => control.llmPromptsSave(changes),
      onSuccess: (list) => {
        queryClient.setQueryData(LIST_KEY, list);
        // Cached renders of a saved prompt (Run dialog previews) are stale now.
        void queryClient.invalidateQueries({ queryKey: [...LIST_KEY, 'adePreview'] });
      },
    },
    queryClient,
  );
}
