import { useGuardedDraft } from '@workbench/behaviour/guardedDraft';
import { computed, onBeforeUnmount, type Ref, watch } from 'vue';
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
  save: { mutateAsync: (args: SaveArgs) => Promise<V> },
) {
  const agentsUi = useAdeAgentsUiStore();

  const view = computed(() => query.data.value);
  const core = useGuardedDraft<string>({
    initial: '',
    source: () => (view.value ? { value: view.value.text, hash: view.value.hash } : undefined),
    refetch: async () => {
      const v = (await query.refetch()).data;
      return v ? { value: v.text, hash: v.hash } : undefined;
    },
    write: (text, baseHash) => save.mutateAsync({ text, baseHash }),
  });
  const { draft, dirty } = core;
  const bytes = computed(() => new TextEncoder().encode(draft.value).length);
  const over = computed(() => view.value !== undefined && bytes.value > view.value.maxBytes);

  watch(dirty, (v) => {
    agentsUi.dirty = v;
  });
  onBeforeUnmount(() => {
    agentsUi.dirty = false;
  });

  async function saveNow(): Promise<void> {
    await core.save();
  }

  function onInput(v: string | number): void {
    core.edit(String(v));
  }

  return {
    view,
    draft,
    dirty,
    saving: core.saving,
    saveError: core.saveError,
    conflict: core.conflict,
    bytes,
    over,
    saveNow,
    discard: core.discard,
    reload: core.reload,
    overwrite: core.overwrite,
    onInput,
  };
}
