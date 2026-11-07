import type { QueryClient } from '@tanstack/vue-query';
import { useDebounceFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { watch } from 'vue';
import { control } from '../bridge/control';
import { repoConfigKey } from '../repo/state/repoConfigQueries';
import { reposKey } from '../repo/state/reposQueries';
import { control as studioControl } from '../studio/bridge/control';
import type { useAgentSessionsStore } from './state';
import { agentContextKey } from './v2/context/queries';
import { installMobileLaunch } from './v2/dialog/mobileLaunch';
import { adeTurns } from './v2/dialog/turnWatch';
import {
  artifactsPrefix,
  dbAuditPrefix,
  handoversPrefix,
  issuePrefix,
  proofChoicesKey,
  proofsPrefix,
  reviewStepPrefix,
  stepYamlKey,
  workflowYamlKey,
} from './v2/queries';
import { installAdeReadSignals } from './v2/readSignals';
import { agentSettingsKey } from './v2/settings/queries';
import { useAdeBoardUiStore } from './v2/state/adeBoardUi';
import { trackedUpdatesKey } from './v2/updates/queries';

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
    void queryClient.invalidateQueries({ queryKey: stepYamlKey });
  });
  control.onAdeTaskSessions(() => {
    void queryClient.invalidateQueries({ queryKey: reviewAgentPrefix });
  });
  control.onRepoConfigChanged(() => {
    void queryClient.invalidateQueries({ queryKey: repoConfigKey });
    void queryClient.invalidateQueries({ queryKey: reposKey, exact: true });
  });
  control.onAgentContextChanged(() => {
    void queryClient.invalidateQueries({ queryKey: agentContextKey });
  });
  control.onAgentSettingsChanged(() => {
    void queryClient.invalidateQueries({ queryKey: agentSettingsKey });
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
  // A proof session closing, attaching or deleting changes a task's chain.
  studioControl.onProofChanged(() => {
    void queryClient.invalidateQueries({ queryKey: proofsPrefix });
    void queryClient.invalidateQueries({ queryKey: proofChoicesKey });
  });
  // A burst of Kira Studio calls refetches the trail once.
  const refreshDbAudit = useDebounceFn(() => {
    void queryClient.invalidateQueries({ queryKey: dbAuditPrefix });
  }, TIMING.search);
  control.onAdeTaskDbAudit(() => {
    void refreshDbAudit();
  });
  control.onAdeTaskRuns(refreshReviewStep);
  control.onAdeTaskUpdates(() => {
    void queryClient.invalidateQueries({ queryKey: trackedUpdatesKey, exact: true });
  });
  control.onAdeTaskOpenSession((event) => {
    useAdeBoardUiStore().openSession(event);
  });
  control.onAgentEvent((event) => adeTurns.onEvent(event));
  installMobileLaunch();
}
