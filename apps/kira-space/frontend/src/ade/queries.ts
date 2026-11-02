import type { QueryClient } from '@tanstack/vue-query';
import { watch } from 'vue';
import { control } from '../bridge/control';
import { reposKey } from '../repo/state/reposQueries';
import type { useAgentSessionsStore } from './state';
import { agentContextKey } from './v2/context/queries';
import { installMobileLaunch } from './v2/dialog/mobileLaunch';
import { adeTurns } from './v2/dialog/turnWatch';
import {
  artifactsPrefix,
  handoversPrefix,
  issuePrefix,
  reviewStepPrefix,
  workflowYamlKey,
} from './v2/queries';
import { installAdeReadSignals } from './v2/readSignals';
import { useAdeBoardUiStore } from './v2/state/adeBoardUi';

const reviewAgentPrefix = ['adetask', 'reviewAgent'] as const;

/** Called once from `main.ts`, app lifetime, no teardown. The shared read upkeep is
 *  `installAdeReadSignals`; on top, an open-session push selects that session, an agent hook event
 *  feeds the dialog turn watchers, a repos push re-reads the repo settings (the shared code repo
 *  list re-reads in `state/coderepos.ts`), a workflows push re-reads any open YAML and a sessions
 *  push the review agent. */
export function installAdeSignals(
  queryClient: QueryClient,
  agent: Pick<ReturnType<typeof useAgentSessionsStore>, 'sessions'>,
): void {
  // A launch not yet listed among live terminals is not `ended`; once per window, so a watch in
  // the review window resolves when its terminal dies too.
  watch(
    () => agent.sessions.map((s) => s.terminalId),
    (ids) => adeTurns.onLive(ids),
    { immediate: true },
  );
  installAdeReadSignals(queryClient, {
    onBoard: control.onAdeTaskBoard,
    onWorkflows: control.onAdeTaskWorkflows,
    onSessions: control.onAdeTaskSessions,
    onRuns: control.onAdeTaskRuns,
    onLog: control.onAdeTaskLog,
  });
  control.onAdeTaskRepos(() => {
    void queryClient.invalidateQueries({ queryKey: reposKey, exact: true });
  });
  // Desktop-only extras on top of the shared read upkeep.
  control.onAdeTaskWorkflows(() => {
    void queryClient.invalidateQueries({ queryKey: workflowYamlKey });
  });
  control.onAdeTaskSessions(() => {
    void queryClient.invalidateQueries({ queryKey: reviewAgentPrefix });
  });
  control.onAgentContextChanged(() => {
    void queryClient.invalidateQueries({ queryKey: agentContextKey });
  });
  // A review step moves with the board, its runs and its own pushes.
  const refreshReviewStep = (): void => {
    void queryClient.invalidateQueries({ queryKey: reviewStepPrefix });
  };
  control.onAdeTaskReview(refreshReviewStep);
  control.onAdeTaskBoard(refreshReviewStep);
  control.onAdeTaskBoard(() => {
    void queryClient.invalidateQueries({ queryKey: handoversPrefix });
    void queryClient.invalidateQueries({ queryKey: artifactsPrefix });
    void queryClient.invalidateQueries({ queryKey: issuePrefix });
  });
  control.onAdeTaskRuns(refreshReviewStep);
  control.onAdeTaskOpenSession((event) => {
    useAdeBoardUiStore().openSession(event);
  });
  control.onAgentEvent((event) => adeTurns.onEvent(event));
  installMobileLaunch();
}
