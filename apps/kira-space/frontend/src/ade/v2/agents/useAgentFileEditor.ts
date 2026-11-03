import { errorText } from '@workbench/bridge/codedError';
import { computed, onBeforeUnmount, type Ref, ref, watch } from 'vue';
import { useAdeAgentsUiStore } from '../state/adeAgentsUi';

interface FileView {
  text: string;
  hash: string;
  maxBytes: number;
}

interface SaveArgs {
  text: string;
  baseHash: string;
}

// Shared draft editor for the agent context and settings files. The draft is the source while it has
// unsaved edits, so a push never resets it. Save sends the hash of the text the draft was edited
// from; a file changed elsewhere is refused with Reload and Overwrite.
export function useAgentFileEditor<V extends FileView>(
  query: { data: Ref<V | undefined>; refetch: () => Promise<{ data: V | undefined }> },
  save: { mutateAsync: (args: SaveArgs) => Promise<V>; isPending: Ref<boolean> },
) {
  const agentsUi = useAdeAgentsUiStore();

  const draft = ref('');
  const dirty = ref(false);
  const baseHash = ref('');
  const saveError = ref('');
  const conflict = ref(false);

  const view = computed(() => query.data.value);
  const bytes = computed(() => new TextEncoder().encode(draft.value).length);
  const actionable = computed(() => dirty.value && !save.isPending.value);
  const over = computed(() => view.value !== undefined && bytes.value > view.value.maxBytes);

  watch(
    view,
    (v) => {
      if (!v || dirty.value) return;
      draft.value = v.text;
      baseHash.value = v.hash;
    },
    { immediate: true },
  );
  watch(dirty, (v) => {
    agentsUi.dirty = v;
  });
  onBeforeUnmount(() => {
    agentsUi.dirty = false;
  });

  async function saveNow(): Promise<void> {
    if (!dirty.value) return;
    const text = draft.value;
    try {
      const next = await save.mutateAsync({ text, baseHash: baseHash.value });
      saveError.value = '';
      conflict.value = false;
      baseHash.value = next.hash;
      if (draft.value === text) dirty.value = false;
    } catch (err) {
      saveError.value = errorText(err);
      conflict.value = (err as { code?: string }).code === 'E_CONFLICT';
    }
  }

  function resetTo(v: { text: string; hash: string } | undefined): void {
    draft.value = v?.text ?? '';
    baseHash.value = v?.hash ?? '';
    dirty.value = false;
    saveError.value = '';
    conflict.value = false;
  }

  function discard(): void {
    resetTo(view.value);
  }

  async function reload(): Promise<void> {
    resetTo((await query.refetch()).data);
  }

  async function overwrite(): Promise<void> {
    const fresh = (await query.refetch()).data;
    if (!fresh) return;
    baseHash.value = fresh.hash;
    conflict.value = false;
    await saveNow();
  }

  function onInput(v: string | number): void {
    draft.value = String(v);
    dirty.value = true;
  }

  return {
    view,
    draft,
    dirty,
    saveError,
    conflict,
    bytes,
    actionable,
    over,
    saveNow,
    discard,
    reload,
    overwrite,
    onInput,
  };
}
