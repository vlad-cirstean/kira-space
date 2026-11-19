import { defineStore } from 'pinia';
import { ref } from 'vue';

export type FilesMode = 'files' | 'search';

/** One task's Files tab: which branch's worktree it shows, Files or Search, and the name filter. */
interface TaskFilesView {
  branchId: string;
  mode: FilesMode;
  filter: string;
}

// View state of the task Files tab, in memory only. Entries of tasks and branches that left the
// board are pruned. Search results live in useAdeFileSearchStore, file lists in TanStack Query.
export const useAdeFilesUiStore = defineStore('adeFilesUi', () => {
  const views = ref<Record<string, TaskFilesView>>({});
  /** Width of the tree and search pane beside the viewer; 0 until the user drags it. */
  const paneWidth = ref(0);
  /** Per branch: expanded directories. Absent until the tree first loads. */
  const expanded = ref<Record<string, Set<string>>>({});
  /** Per branch: the selected tree path and the file open in the viewer. */
  const selected = ref<Record<string, string | null>>({});
  const openFile = ref<Record<string, string | null>>({});
  /** Per branch: a markdown file shows its rendered Reading view. */
  const reading = ref<Record<string, boolean>>({});

  const viewOf = (taskId: string): TaskFilesView =>
    views.value[taskId] ?? { branchId: '', mode: 'files', filter: '' };
  function patch(taskId: string, p: Partial<TaskFilesView>): void {
    views.value[taskId] = { ...viewOf(taskId), ...p };
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

  function dropMissing(rec: Record<string, unknown>, keep: ReadonlySet<string>): void {
    for (const k of Object.keys(rec)) if (!keep.has(k)) delete rec[k];
  }
  /** Drops entries of tasks and branches no longer on the board (archived, removed). */
  function prune(taskIds: ReadonlySet<string>, branchIds: ReadonlySet<string>): void {
    dropMissing(views.value, taskIds);
    for (const rec of [expanded.value, selected.value, openFile.value, reading.value]) {
      dropMissing(rec, branchIds);
    }
  }

  return {
    views,
    paneWidth,
    expanded,
    selected,
    openFile,
    reading,
    viewOf,
    patch,
    expandedOf,
    initExpanded,
    toggleDir,
    prune,
  };
});
