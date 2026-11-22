import type { ApiUnlockResult, ApiVariableEdit } from '@shared/domain/apiConfig';
import { useTaskAction } from '@workbench/behaviour/asyncAction';
import { queryClient } from '@workbench/state/queryClient';
import { defineStore } from 'pinia';
import { ref } from 'vue';
import { editVariables, unlockVariables } from './apiQueries';

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

  async function edit(edits: ApiVariableEdit[]): Promise<boolean> {
    // The task thunk is the mutation's variable; it hands the edits over once so the typed secret
    // is not kept by the last mutation of this never-unmounting store.
    let held: ApiVariableEdit[] | null = edits;
    return surface.run(() => {
      const mine = held ?? [];
      held = null;
      return editVariables(mine);
    });
  }

  /** Asks `confirm` first, every time: unlocking writes plain text to the file. A yes goes through
   *  the reveal gate, which still authenticates where the OS can. False when nothing was unlocked. */
  async function unlock(
    edits: ApiVariableEdit[],
    confirm: () => Promise<boolean>,
  ): Promise<boolean> {
    if (!(await confirm())) return false;
    let outcome = 'done' as ApiUnlockResult['outcome'];
    const ran = await surface.run(async () => {
      outcome = await unlockVariables(edits, true);
    });
    return ran && outcome === 'done';
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
    unlock,
    startEditing,
    stopEditing,
    focusCollection,
  };
});
