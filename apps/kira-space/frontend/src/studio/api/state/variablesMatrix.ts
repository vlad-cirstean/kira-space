import type { ApiVariableEdit } from '@shared/domain/apiConfig';
import { useTaskAction } from '@workbench/behaviour/asyncAction';
import { queryClient } from '@workbench/state/queryClient';
import { defineStore } from 'pinia';
import { ref } from 'vue';
import { useEditVariables } from './apiQueries';

/** One cell's identity: the owner file's row id and the variable name. */
export function cellKey(ownerId: string, name: string): string {
  return `${ownerId}\u0000${name}`;
}

// The Variables tab's UI state: filter, which collection supplies the default column, the one
// environment column narrow mode shows, the cell or row being edited, and the failure of the last
// edit.
// Rows themselves are query data (useVariableMatrix); every edit goes through `edit`.
export const useVariablesMatrixStore = defineStore('variablesMatrix', () => {
  const filter = ref('');
  /** The collection behind the default column; '' means the first one. */
  const collectionId = ref('');
  /** Narrow mode's one column (an environment or the default); null follows the active environment. */
  const shownOwnerId = ref<string | null>(null);
  const editing = ref<string | null>(null);
  /** A row's own inline edit: its name or its description. */
  const rowEdit = ref<{ name: string; mode: 'rename' | 'describe' } | null>(null);
  /** The new-environment name input is open. */
  const addingEnvironment = ref(false);
  /** The last rename: requests are not rewritten, so the tab offers a search for the old name. */
  const renamed = ref<{ from: string; to: string } | null>(null);
  /** Bumped to move focus to the new-row name input. */
  const newRowFocus = ref(0);
  const surface = useTaskAction({ queryClient });
  const editMutation = useEditVariables();

  async function edit(edits: ApiVariableEdit[]): Promise<boolean> {
    return surface.run(() => editMutation.mutateAsync(edits));
  }

  function startEditing(key: string): void {
    editing.value = key;
  }

  function stopEditing(): void {
    editing.value = null;
  }

  /** Opens the tab on a collection's default column. */
  function focusCollection(id: string): void {
    collectionId.value = id;
  }

  return {
    filter,
    collectionId,
    shownOwnerId,
    editing,
    rowEdit,
    addingEnvironment,
    renamed,
    newRowFocus,
    error: surface.error,
    dismissError: surface.reset,
    fail: surface.fail,
    edit,
    startEditing,
    stopEditing,
    focusCollection,
  };
});
