import { useQuery } from '@tanstack/vue-query';
import type { MaybeRefOrGetter } from 'vue';
import { computed, toValue } from 'vue';
import { useAdeReader } from './reader';
import type { Board, LogKind } from './wire';

// The board, change request, workflow and session queries are `shallow`: each fetch replaces the whole
// value, so deep reactive tracking of thousands of read-only fields buys nothing.
// Read side of the ADE queries, transport-agnostic (`useAdeReader`). Push-driven: the client's default
// staleTime is Infinity and the signals installed per app (`installAdeReadSignals`) invalidate.

export const boardKey = ['adetask', 'board'] as const;
export const changeRequestsKey = ['adetask', 'changeRequests'] as const;
export const workflowsKey = ['adetask', 'workflows'] as const;
export const logKey = (kind: LogKind, id: string) => ['adetask', 'log', kind, id] as const;
export const runSandboxKey = ['ade', 'runSandbox'] as const;
export const sessionsKey = ['adetask', 'sessions'] as const;
export const panelForwardPrefix = ['adetask', 'panelForward'] as const;
export const repoNamesKey = ['adetask', 'repoNames'] as const;
const archivedKey = ['adetask', 'archived'] as const;
export const archivedTaskKey = (taskId: string) => [...archivedKey, taskId] as const;
const hostProvidersKey = ['adetask', 'hostProviders'] as const;

export function useBoard() {
  const reader = useAdeReader();
  return useQuery({
    queryKey: boardKey,
    shallow: true,
    queryFn: () => reader.board(),
  });
}

const NO_HISTORY: Board['history'] = [];

/** The archived tasks of the board; the one reader of `board.history`, so history can move to a
 *  query of its own without touching callers. */
export function useBoardHistory() {
  const board = useBoard();
  return computed(() => board.data.value?.history ?? NO_HISTORY);
}

export function useChangeRequests() {
  const reader = useAdeReader();
  return useQuery({
    queryKey: changeRequestsKey,
    shallow: true,
    queryFn: () => reader.changeRequests(),
  });
}

export function useWorkflows() {
  const reader = useAdeReader();
  return useQuery({
    queryKey: workflowsKey,
    shallow: true,
    queryFn: () => reader.workflows(),
  });
}

export function useSessions() {
  const reader = useAdeReader();
  return useQuery({
    queryKey: sessionsKey,
    shallow: true,
    queryFn: () => reader.sessions(),
  });
}

/** Every registered code host provider; static for the life of the app, so never invalidated. */
export function useGitHostProviders() {
  const reader = useAdeReader();
  return useQuery({
    queryKey: hostProvidersKey,
    queryFn: () => reader.hostProviders(),
  });
}

/** Repo names for read-only clients; the desktop reads the full repo settings elsewhere. */
export function useRepoNames() {
  const reader = useAdeReader();
  return useQuery({
    queryKey: repoNamesKey,
    queryFn: () => reader.repos(),
  });
}

/** One log from its first chunk; the log push appends to this cache. */
export function useLog(kind: MaybeRefOrGetter<LogKind>, id: MaybeRefOrGetter<string>) {
  const reader = useAdeReader();
  return useQuery(() => ({
    queryKey: logKey(toValue(kind), toValue(id)),
    queryFn: () => reader.readLog({ kind: toValue(kind), id: toValue(id), afterSeq: 0 }),
    gcTime: 30_000,
  }));
}

const ARCHIVED_STALE_MS = 60_000;

/** An archived task with all it kept. It cannot change while archived, so board pushes leave it
 *  alone: each refetch costs host API calls per repo. Restore drops it. */
export function useArchivedTask(taskId: MaybeRefOrGetter<string | null>) {
  const reader = useAdeReader();
  return useQuery(() => {
    const id = toValue(taskId);
    return {
      queryKey: archivedTaskKey(id ?? ''),
      queryFn: () => reader.archivedTask({ taskId: id ?? '' }),
      enabled: id !== null,
      staleTime: ARCHIVED_STALE_MS,
    };
  });
}
