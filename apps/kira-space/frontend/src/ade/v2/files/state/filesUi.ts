import { defineStore } from 'pinia';
import { ref } from 'vue';

export type FilesMode = 'files' | 'search';

/** The Files window's view: which branch's worktree it shows, Files or Search, and the name filter. */
interface TaskFilesView {
  branchId: string;
  mode: FilesMode;
  filter: string;
}

// View state of the task Files window, in memory only; one window shows one task. Search results
// live in useAdeFileSearchStore, file lists in TanStack Query.
export const useAdeFilesUiStore = defineStore('adeFilesUi', () => {
  const view = ref<TaskFilesView>({ branchId: '', mode: 'files', filter: '' });
  /** Per branch: expanded directories. Absent until the tree first loads. */
  const expanded = ref<Record<string, Set<string>>>({});
  /** Per branch: the selected tree path and the file open in the viewer. */
  const selected = ref<Record<string, string | null>>({});
  const openFile = ref<Record<string, string | null>>({});
  /** Per branch: a markdown file shows its rendered Reading view. */
  const reading = ref<Record<string, boolean>>({});
  /** Per branch: why the last Reveal or Open in VS Code failed. */
  const launchError = ref<Record<string, string>>({});

  function patch(p: Partial<TaskFilesView>): void {
    view.value = { ...view.value, ...p };
  }

  const expandedOf = (branchId: string): Set<string> | undefined => expanded.value[branchId];
  function initExpanded(branchId: string, dirs: Iterable<string>): void {
    if (!expanded.value[branchId]) expanded.value[branchId] = new Set(dirs);
  }
  function toggleDir(branchId: string, path: string): void {
    const set = expanded.value[branchId] ?? new Set<string>();
    if (set.has(path)) set.delete(path);
    else set.add(path);
    expanded.value[branchId] = set;
  }

  return {
    view,
    expanded,
    selected,
    openFile,
    reading,
    launchError,
    patch,
    expandedOf,
    initExpanded,
    toggleDir,
  };
});
