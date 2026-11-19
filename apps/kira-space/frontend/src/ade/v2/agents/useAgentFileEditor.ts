import { useAsyncAction } from '@workbench/behaviour/asyncAction';
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

  const view = computed(() => query.data.value);
  const bytes = computed(() => new TextEncoder().encode(draft.value).length);
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

  const saving = useAsyncAction(async (text: string) => {
    const next = await save.mutateAsync({ text, baseHash: baseHash.value });
    baseHash.value = next.hash;
    if (draft.value === text) dirty.value = false;
  });
  const saveError = saving.error;
  const conflict = computed(() => saving.cause.value?.code === 'E_CONFLICT');

  async function saveNow(): Promise<void> {
    if (!dirty.value) return;
    await saving.run(draft.value);
  }

  function resetTo(v: { text: string; hash: string } | undefined): void {
    draft.value = v?.text ?? '';
    baseHash.value = v?.hash ?? '';
    dirty.value = false;
    saving.reset();
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
    over,
    saveNow,
    discard,
    reload,
    overwrite,
    onInput,
  };
}
