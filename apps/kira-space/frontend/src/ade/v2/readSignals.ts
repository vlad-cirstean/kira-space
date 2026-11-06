import type { QueryClient } from '@tanstack/vue-query';
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
import type { Board, LogEvent, LogPage, RunsChangedEvent } from './wire';

/** Where read-side pushes come from: Wails events on the desktop, server-sent events on the
 *  phone. Each `on*` subscribes and returns its unsubscribe. */
export interface AdeSignalSource {
  onBoard(cb: () => void): () => void;
  onWorkflows(cb: () => void): () => void;
  onSessions(cb: () => void): () => void;
  onRuns(cb: (event: RunsChangedEvent) => void): () => void;
  onLog(cb: (event: LogEvent) => void): () => void;
}

/** Cache upkeep for the read queries, shared by both apps. Every board push re-reads the board and
 *  PR facts; sessions and workflows pushes re-read their list; a runs push merges into
 *  the cached board; a log push appends to the cached log. */
export function installAdeReadSignals(queryClient: QueryClient, source: AdeSignalSource): void {
  source.onBoard(() => {
    void queryClient.invalidateQueries({ queryKey: boardKey, exact: true });
    void queryClient.invalidateQueries({ queryKey: changeRequestsKey, exact: true });
    void queryClient.invalidateQueries({ queryKey: panelForwardPrefix });
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
