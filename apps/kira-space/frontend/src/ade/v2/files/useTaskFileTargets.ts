import { computed } from 'vue';
import { useRepos } from '../../../repo/state/reposQueries';
import { useBoard } from '../queries';

export interface TaskFileTarget {
  branchId: string;
  repo: string;
  branch: string;
  worktree: string;
}

/** The task's named branches that have a worktree, in the task's own order. */
export function useTaskFileTargets(taskId: () => string) {
  const board = useBoard();
  const repos = useRepos();
  const task = computed(() => board.data.value?.tasks.find((x) => x.id === taskId()));
  const repoLabel = (codeRepoId: string): string => {
    const r = repos.data.value?.repos.find((x) => x.codeRepoId === codeRepoId);
    return r ? r.nickname || r.name : codeRepoId;
  };
  const targets = computed<TaskFileTarget[]>(() => {
    const t = task.value;
    const branches = board.data.value?.branches ?? [];
    if (!t) return [];
    return t.branchIds
      .map((id) => branches.find((b) => b.id === id))
      .filter(
        (b): b is NonNullable<typeof b> => b !== undefined && b.name !== '' && b.worktree !== '',
      )
      .map((b) => ({
        branchId: b.id,
        repo: repoLabel(b.codeRepoId),
        branch: b.name,
        worktree: b.worktree,
      }));
  });
  return { task, targets };
}
