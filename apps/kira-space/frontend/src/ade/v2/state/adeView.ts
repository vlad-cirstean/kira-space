import { useAdeAgentsUiStore } from './adeAgentsUi';
import { type AdeView, useAdeBoardUiStore } from './adeBoardUi';
import { useAdeReposUiStore } from './adeReposUi';
import { useAdeWorkflowsUiStore } from './adeWorkflowsUi';

/** Opens `next` once the page being left has no unsaved edits, or the user discards them. */
export async function switchAdeView(next: AdeView): Promise<boolean> {
  const board = useAdeBoardUiStore();
  if (next === board.view) return true;
  const guard =
    board.view === 'workflows'
      ? useAdeWorkflowsUiStore().leave
      : board.view === 'agents'
        ? useAdeAgentsUiStore().leave
        : board.view === 'repos'
          ? useAdeReposUiStore().leave
          : null;
  if (guard && !(await guard())) return false;
  board.view = next;
  return true;
}
