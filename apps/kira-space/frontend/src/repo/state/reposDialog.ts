import { defineStore } from 'pinia';
import { ref } from 'vue';
import { useLayoutStore } from '../../state/layout';
import { useModeStore } from '../../state/mode';

export type ReposDialogTab = 'repos' | 'folders' | 'files';

export const useReposDialogStore = defineStore('reposDialog', () => {
  const open = ref(false);
  const tab = ref<ReposDialogTab>('repos');
  /** Selected repo (`null` = first listed). */
  const selectedRepoId = ref<string | null>(null);

  /** The dialog lives in the Git panel: switch to the Git module and unhide the panel first. */
  function show(opts: { repoId?: string; tab?: ReposDialogTab } = {}): void {
    useModeStore().setMode('git');
    const layout = useLayoutStore();
    if (!layout.panel.project.visible) layout.toggleProjectPanel();
    if (opts.repoId) selectedRepoId.value = opts.repoId;
    tab.value = opts.tab ?? 'repos';
    open.value = true;
  }

  return { open, tab, selectedRepoId, show };
});
