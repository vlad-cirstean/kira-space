import type { QueryClient } from '@tanstack/vue-query';
import { type BoardChange, invalidateTaskFamily } from './boardEvent';
import { appendChunks, mergeRuns } from './pushMerge';
import {
  boardKey,
  changeRequestsKey,
  logKey,
  panelForwardPrefix,
  runSandboxKey,
  sessionsKey,
  workflowsKey,
} from './readQueries';
import { createRefetchGate } from './refetchGate';
import type { Board, LogEvent, LogPage, RunsChangedEvent } from './wire';

/** Where read-side pushes come from: Wails events on the desktop, server-sent events on the
 *  phone. Each `on*` subscribes and returns its unsubscribe. */
export interface AdeSignalSource {
  /** The payload arrives normalized (`normalizeBoardEvent`): a push without one reads as all changed. */
  onBoard(cb: (change: BoardChange) => void): () => void;
  onWorkflows(cb: () => void): () => void;
  onSessions(cb: () => void): () => void;
  onRuns(cb: (event: RunsChangedEvent) => void): () => void;
  onLog(cb: (event: LogEvent) => void): () => void;
}

/** Cache upkeep for the read queries, shared by both apps. Every board push re-reads the board and
 *  PR facts (through a single-flight gate, and the PR facts only when refs changed); sessions and workflows pushes re-read their list; a runs push merges into
 *  the cached board; a log push appends to the cached log. */
export function installAdeReadSignals(queryClient: QueryClient, source: AdeSignalSource): void {
  const boardGate = createRefetchGate(queryClient, boardKey);
  const changeRequestsGate = createRefetchGate(queryClient, changeRequestsKey);
  source.onBoard((change) => {
    boardGate.signal();
    if (change.refs) changeRequestsGate.signal();
    invalidateTaskFamily(queryClient, panelForwardPrefix, change);
  });
  source.onWorkflows(() => {
    void queryClient.invalidateQueries({ queryKey: workflowsKey, exact: true });
  });
  source.onSessions(() => {
    void queryClient.invalidateQueries({ queryKey: sessionsKey, exact: true });
    void queryClient.invalidateQueries({ queryKey: panelForwardPrefix });
    void queryClient.invalidateQueries({ queryKey: [...runSandboxKey, 'session'] });
  });
  source.onRuns((event) => {
    queryClient.setQueryData<Board>(boardKey, (board) => (board ? mergeRuns(board, event) : board));
    for (const run of event.runs) {
      if (run.sandbox) {
        void queryClient.invalidateQueries({ queryKey: [...runSandboxKey, run.id], exact: true });
      }
    }
  });
  source.onLog((event) => {
    queryClient.setQueryData<LogPage>(logKey(event.kind, event.id), (page) =>
      page ? appendChunks(page, event.chunks) : page,
    );
  });
}
