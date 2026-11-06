import type { LlmPromptPreview } from '@shared/domain/llmPrompts';
import { keepPreviousData, useMutation, useQuery } from '@tanstack/vue-query';
import { optimisticUpdate } from '@workbench/behaviour/optimistic';
import { queryClient } from '@workbench/state/queryClient';
import type { MaybeRefOrGetter } from 'vue';
import { toValue } from 'vue';
import { control } from '../../bridge/control';
import { withShelved } from './board/shelf';
import { archivedTaskKey, boardKey, runSandboxKey, sessionsKey } from './readQueries';
import type {
  AddExistingBranchArgs,
  AddLinkArgs,
  AddNoteArgs,
  AddTaskRepoArgs,
  Board,
  BranchArgs,
  CaptureTaskArgs,
  CloseSessionArgs,
  CreateTaskArgs,
  DeleteStepArgs,
  EditLinkArgs,
  FinishReviewArgs,
  FocusSessionArgs,
  ForwardReviewResult,
  HostLinkArgs,
  ImportWorkflowArgs,
  LaunchStepArgs,
  LinkProofArgs,
  NewStepArgs,
  NewWorkflowArgs,
  OntoArgs,
  RebaseArgs,
  RebasePreview,
  RebaseStart,
  RecordMergeArgs,
  RefreshArgs,
  RemoveArtifactArgs,
  RemoveTaskRowArgs,
  RepoBranches,
  RepoBranchesArgs,
  RestartSessionArgs,
  ReviewAgentLaunch,
  ReviewForwardText,
  RunArgs,
  SaveStatusesArgs,
  SaveStatusesResult,
  SaveStepArgs,
  SaveStepYamlArgs,
  SaveWorkflowArgs,
  SaveWorkflowYamlArgs,
  SendArgs,
  SendReviewArgs,
  SetBranchBaseArgs,
  SetPlanArgs,
  SetTaskBranchArgs,
  SetTaskStepArgs,
  SetTaskWorkflowArgs,
  ShelveTaskArgs,
  StartAdHocArgs,
  StartBranchArgs,
  StartRunArgs,
  StepArgs,
  StepDoneArgs,
  TakeOverArgs,
  Task,
  TaskArgs,
  UpdateHandoverArgs,
  UpdateTaskArgs,
  UpdateTaskRepoArgs,
  ValidateStepYamlArgs,
  ValidateWorkflowYamlArgs,
} from './wire';

export {
  logKey,
  useBoard,
  useChangeRequests,
  useSessions,
  useWorkflows,
} from './readQueries';

// Board state is push-driven (`kira:adetask:board`, subscribed once in `ade/queries.ts`), so the
// client default keeps data fresh forever and a write relies on the backend's own push instead of
// invalidating. The two exceptions refetch on their own: the candidate list (`staleTime: 0`, the
// popover opens on demand) and a failed optimistic write.

export const workflowYamlKey = ['adetask', 'workflowYaml'] as const;
export const stepYamlKey = ['adetask', 'stepYaml'] as const;
const candidatesKey = (codeRepoId: string) => ['adetask', 'candidates', codeRepoId] as const;
export const reviewStepPrefix = ['adetask', 'reviewStep'] as const;
export const handoversPrefix = ['adetask', 'handovers'] as const;
export const proofsPrefix = ['adetask', 'proofs'] as const;
export const proofChoicesKey = ['adetask', 'proofChoices'] as const;
export const artifactsPrefix = ['adetask', 'artifacts'] as const;
export const issuePrefix = ['adetask', 'issue'] as const;
export const reviewTargetKey = (windowKey: string) =>
  ['adetask', 'reviewTarget', windowKey] as const;

/**
 * Branches that can join the plan, of one repo or ('' ) every repo; refetched on every open so a
 * just-pushed branch shows.
 */
export function useCandidates(codeRepoId: () => string, enabled: () => boolean) {
  return useQuery(() => ({
    queryKey: candidatesKey(codeRepoId()),
    queryFn: () => control.adeTaskCandidateBranches({ codeRepoId: codeRepoId() }),
    staleTime: 0,
    enabled: enabled(),
  }));
}

export function useRefresh() {
  return useMutation({
    mutationKey: ['adetask', 'refresh'],
    mutationFn: (args: RefreshArgs) => control.adeTaskRefresh(args),
  });
}

export function useForcePush() {
  return useMutation({ mutationFn: (args: BranchArgs) => control.adeTaskForcePush(args) });
}

export function useCreateTask() {
  return useMutation({ mutationFn: (args: CreateTaskArgs) => control.adeTaskCreateTask(args) });
}

export function useAddExistingBranch() {
  return useMutation({
    mutationFn: (args: AddExistingBranchArgs) => control.adeTaskAddExistingBranch(args),
  });
}

/** The branch a pasted PR or branch link names; read only, so it runs as the user types. */
export function useResolveHostLink(url: MaybeRefOrGetter<string>) {
  return useQuery(() => {
    const u = toValue(url).trim();
    return {
      queryKey: ['adetask', 'hostLink', u] as const,
      queryFn: () => control.adeTaskResolveHostLink({ url: u, taskId: '' }),
      enabled: u.startsWith('https://'),
      staleTime: 30_000,
    };
  });
}

export function useAddHostLink() {
  return useMutation({ mutationFn: (args: HostLinkArgs) => control.adeTaskAddHostLink(args) });
}

export function useAddTaskRepo() {
  return useMutation({ mutationFn: (args: AddTaskRepoArgs) => control.adeTaskAddTaskRepo(args) });
}

export function useSetTaskBranch() {
  return useMutation({
    mutationFn: (args: SetTaskBranchArgs) => control.adeTaskSetTaskBranch(args),
  });
}

export function useUpdateTaskRepo() {
  return useMutation({
    mutationFn: (args: UpdateTaskRepoArgs) => control.adeTaskUpdateTaskRepo(args),
  });
}

export function useRemoveTaskBranch() {
  return useMutation({
    mutationFn: (args: RemoveTaskRowArgs) => control.adeTaskRemoveTaskBranch(args),
  });
}

export function useRemoveTaskRepo() {
  return useMutation({
    mutationFn: (args: RemoveTaskRowArgs) => control.adeTaskRemoveTaskRepo(args),
  });
}

/** What removing a row's branch would discard; read fresh at click time, like the archive risk. */
export function useTaskBranchRisk() {
  return useMutation({ mutationFn: (args: BranchArgs) => control.adeTaskTaskBranchRisk(args) });
}

/** Applies a task patch to a board snapshot: a `null` field is unchanged, `clearJira` drops Jira. */
function withTaskPatch(board: Board, args: UpdateTaskArgs): Board {
  const p = args.patch;
  const tasks = board.tasks.map((t): Task => {
    if (t.id !== args.taskId) return t;
    return {
      ...t,
      title: p.title ?? t.title,
      jira: p.clearJira ? null : (p.jira ?? t.jira),
      hostUrl: p.hostUrl ?? t.hostUrl,
      est: p.est ?? t.est,
      color: p.color ?? t.color,
      kind: p.kind ?? t.kind,
      statusId: p.statusId ?? t.statusId,
      statusBy: p.statusId ? 'user' : t.statusBy,
    };
  });
  return { ...board, tasks };
}

export function useUpdateTask() {
  return useMutation({
    mutationFn: (args: UpdateTaskArgs) => control.adeTaskUpdateTask(args),
    ...optimisticUpdate<Board, UpdateTaskArgs>(queryClient, boardKey, withTaskPatch),
  });
}

/** The saved set replaces `board.statuses`; the backend's board push brings the reassigned tasks. */
function withSavedStatuses(board: Board, args: SaveStatusesArgs): Board {
  return {
    ...board,
    statuses: args.statuses.map((s) => ({
      ...s,
      id: s.id || board.statuses.find((o) => o.name === s.name)?.id || s.name,
    })),
    statusDefaults: args.defaults,
  };
}

export function useSaveStatuses() {
  const optimistic = optimisticUpdate<Board, SaveStatusesArgs>(
    queryClient,
    boardKey,
    withSavedStatuses,
  );
  return useMutation({
    mutationFn: (args: SaveStatusesArgs) => control.adeTaskSaveStatuses(args),
    ...optimistic,
    // The result carries the real ids of new statuses; the board push may land after this resolves.
    onSuccess: (result, args) => {
      queryClient.setQueryData<Board>(boardKey, (b) => (b ? withSavedIds(b, args, result) : b));
    },
  });
}

/** Swaps the placeholder id of each new status for the one the backend assigned. Positions line up
 *  only while the cache still holds the optimistic set; any other shape is left to the board push. */
function withSavedIds(board: Board, args: SaveStatusesArgs, result: SaveStatusesResult): Board {
  const n = args.statuses.length;
  if (board.statuses.length !== n || result.statuses.length !== n) return board;
  return {
    ...board,
    statuses: board.statuses.map((s, i) =>
      args.statuses[i]?.id === '' ? { ...s, id: result.statuses[i]?.id ?? s.id } : s,
    ),
  };
}

/** Applies a plan write to a board snapshot the way the backend does: `order` replaces, a `null`
 *  day sends the task to Later. */
function withPlan(board: Board, args: SetPlanArgs): Board {
  const day = { ...board.plan.day };
  for (const [id, iso] of Object.entries(args.days)) {
    if (iso === null) delete day[id];
    else day[id] = iso;
  }
  const backlog = board.plan.backlog.filter((id) => !(id in args.days));
  return { ...board, plan: { ...board.plan, order: args.order, day, backlog } };
}

export function useSetPlan() {
  return useMutation({
    mutationFn: (args: SetPlanArgs) => control.adeTaskSetPlan(args),
    ...optimisticUpdate<Board, SetPlanArgs>(queryClient, boardKey, withPlan),
  });
}

export function useShelveTask() {
  return useMutation({
    mutationFn: (args: ShelveTaskArgs) => control.adeTaskShelveTask(args),
    ...optimisticUpdate<Board, ShelveTaskArgs>(queryClient, boardKey, withShelved),
  });
}

/** Adds a title-only task at the top of Later. */
export function useCaptureTask() {
  return useMutation({ mutationFn: (args: CaptureTaskArgs) => control.adeTaskCaptureTask(args) });
}

/** The stored sandbox of a run; the badge popover and profile dialog read it. */
export function useRunSandbox(runId: MaybeRefOrGetter<string>, enabled: MaybeRefOrGetter<boolean>) {
  return useQuery(() => ({
    queryKey: [...runSandboxKey, toValue(runId)] as const,
    queryFn: () => control.adeTaskRunSandbox({ runId: toValue(runId) }),
    enabled: toValue(enabled) && toValue(runId) !== '',
  }));
}

/** The sandbox a manual step session started with; the badge popover reads it. */
export function useSessionSandbox(
  sessionId: MaybeRefOrGetter<string>,
  enabled: MaybeRefOrGetter<boolean>,
) {
  return useQuery(() => ({
    queryKey: [...runSandboxKey, 'session', toValue(sessionId)] as const,
    queryFn: () => control.adeTaskSessionSandbox({ sessionId: toValue(sessionId) }),
    enabled: toValue(enabled) && toValue(sessionId) !== '',
  }));
}

export function useWorkflowYaml(fileName: MaybeRefOrGetter<string>) {
  return useQuery(() => ({
    queryKey: [...workflowYamlKey, toValue(fileName)] as const,
    queryFn: () => control.adeTaskWorkflowYaml({ fileName: toValue(fileName) }),
    enabled: toValue(fileName) !== '',
  }));
}

export function useValidateWorkflowYaml() {
  return useMutation({
    mutationFn: (args: ValidateWorkflowYamlArgs) => control.adeTaskValidateWorkflowYaml(args),
  });
}

/** Saves run one at a time, in call order: a debounce, a blur and an unmount can overlap, and an
 *  older text landing last would be pushed back into the editor. */
const workflowSaveScope = { id: 'ade-workflow-save' } as const;

const stepSaveScope = { id: 'ade-step-save' } as const;

export function useSaveWorkflow() {
  return useMutation({
    mutationFn: (args: SaveWorkflowArgs) => control.adeTaskSaveWorkflow(args),
    scope: workflowSaveScope,
  });
}

export function useSaveWorkflowYaml() {
  return useMutation({
    mutationFn: (args: SaveWorkflowYamlArgs) => control.adeTaskSaveWorkflowYaml(args),
    scope: workflowSaveScope,
  });
}

export function useStepYaml(fileName: MaybeRefOrGetter<string>) {
  return useQuery(() => ({
    queryKey: [...stepYamlKey, toValue(fileName)] as const,
    queryFn: () => control.adeTaskStepYaml({ fileName: toValue(fileName) }),
    enabled: toValue(fileName) !== '',
  }));
}

export function useValidateStepYaml() {
  return useMutation({
    mutationFn: (args: ValidateStepYamlArgs) => control.adeTaskValidateStepYaml(args),
  });
}

export function useSaveStep() {
  return useMutation({
    mutationFn: (args: SaveStepArgs) => control.adeTaskSaveStep(args),
    scope: stepSaveScope,
  });
}

export function useSaveStepYaml() {
  return useMutation({
    mutationFn: (args: SaveStepYamlArgs) => control.adeTaskSaveStepYaml(args),
    scope: stepSaveScope,
  });
}

/** Creates a library step, or saves `from` (an inline workflow step) to the library. */
export function useNewStep() {
  return useMutation({ mutationFn: (args: NewStepArgs) => control.adeTaskNewStep(args) });
}

export function useDeleteStep() {
  return useMutation({ mutationFn: (args: DeleteStepArgs) => control.adeTaskDeleteStep(args) });
}

export function useImportWorkflow() {
  return useMutation({
    mutationFn: (args: ImportWorkflowArgs) => control.adeTaskImportWorkflow(args),
  });
}

export function useNewWorkflow() {
  return useMutation({ mutationFn: (args: NewWorkflowArgs) => control.adeTaskNewWorkflow(args) });
}

export function useSetTaskWorkflow() {
  return useMutation({
    mutationFn: (args: SetTaskWorkflowArgs) => control.adeTaskSetTaskWorkflow(args),
  });
}

export function useStartRun() {
  return useMutation({ mutationFn: (args: StartRunArgs) => control.adeTaskStartRun(args) });
}

export function useApprove() {
  return useMutation({ mutationFn: (args: StepArgs) => control.adeTaskApprove(args) });
}

export function useRetryRun() {
  return useMutation({ mutationFn: (args: RunArgs) => control.adeTaskRetryRun(args) });
}

export function useResumeRun() {
  return useMutation({ mutationFn: (args: RunArgs) => control.adeTaskResumeRun(args) });
}

export function useStepDone() {
  return useMutation({ mutationFn: (args: StepDoneArgs) => control.adeTaskStepDone(args) });
}

/** Applies a step move to a board snapshot; the backend's push reconciles the run-derived fields. */
function withStep(board: Board, args: SetTaskStepArgs): Board {
  const tasks = board.tasks.map(
    (t): Task => (t.id === args.taskId ? { ...t, entryStepId: args.stepId } : t),
  );
  return { ...board, tasks };
}

export function useSetTaskStep() {
  return useMutation({
    mutationFn: (args: SetTaskStepArgs) => control.adeTaskSetTaskStep(args),
    ...optimisticUpdate<Board, SetTaskStepArgs>(queryClient, boardKey, withStep),
  });
}

export function useRetrySetup() {
  return useMutation({ mutationFn: (args: BranchArgs) => control.adeTaskRetrySetup(args) });
}

export function useOpenReviewWindow() {
  return useMutation({ mutationFn: (args: BranchArgs) => control.adeTaskOpenReviewWindow(args) });
}

export function useTakeOver() {
  return useMutation({ mutationFn: (args: TakeOverArgs) => control.adeTaskTakeOver(args) });
}

export function useRestartSession() {
  return useMutation({
    mutationFn: (args: RestartSessionArgs) => control.adeTaskRestartSession(args),
  });
}

export function useCloseSession() {
  return useMutation({
    mutationFn: (args: CloseSessionArgs) => control.adeTaskCloseSession(args),
  });
}

export function useLaunchStep() {
  return useMutation({ mutationFn: (args: LaunchStepArgs) => control.adeTaskLaunchStep(args) });
}

export function useStartBranch() {
  return useMutation({ mutationFn: (args: StartBranchArgs) => control.adeTaskStartBranch(args) });
}

export function useStartAdHoc() {
  return useMutation({ mutationFn: (args: StartAdHocArgs) => control.adeTaskStartAdHoc(args) });
}

export function useSend() {
  return useMutation({ mutationFn: (args: SendArgs) => control.adeTaskSend(args) });
}

export function useStopRun() {
  return useMutation({ mutationFn: (args: RunArgs) => control.adeTaskStopRun(args) });
}

export function useRecordMerge() {
  return useMutation({ mutationFn: (args: RecordMergeArgs) => control.adeTaskRecordMerge(args) });
}

export function useArchiveRisk() {
  return useMutation({ mutationFn: (args: TaskArgs) => control.adeTaskArchiveRisk(args) });
}

export function useArchiveTask() {
  return useMutation({ mutationFn: (args: TaskArgs) => control.adeTaskArchiveTask(args) });
}

/** Unarchives a task; the board and sessions refetch and its archived read is dropped (the task
 *  is live, so the read would fail). */
export function useRestoreTask() {
  return useMutation({
    mutationFn: (args: TaskArgs) => control.adeTaskRestoreTask(args),
    onSuccess: (_task, args) => {
      queryClient.removeQueries({ queryKey: archivedTaskKey(args.taskId), exact: true });
      return Promise.all([
        queryClient.invalidateQueries({ queryKey: boardKey, exact: true }),
        queryClient.invalidateQueries({ queryKey: sessionsKey, exact: true }),
      ]);
    },
  });
}

export function useFocusSession() {
  return useMutation({ mutationFn: (args: FocusSessionArgs) => control.adeTaskFocusSession(args) });
}

/** The task's review agent and where it runs; a sessions push refetches it. */
export function useReviewAgent(taskId: MaybeRefOrGetter<string>) {
  return useQuery({
    queryKey: ['adetask', 'reviewAgent', taskId] as const,
    queryFn: () => control.adeTaskReviewAgent({ taskId: toValue(taskId) }),
  });
}

/** The task's handover history, newest first; a board push refetches it. */
export function useHandovers(taskId: MaybeRefOrGetter<string>) {
  return useQuery({
    queryKey: [...handoversPrefix, taskId] as const,
    queryFn: () => control.adeTaskHandovers({ taskId: toValue(taskId) }),
  });
}

/** The proof sessions attached to a task, newest closed first; a proof push refetches it. */
export function useTaskProofs(taskId: MaybeRefOrGetter<string>) {
  return useQuery({
    queryKey: [...proofsPrefix, taskId] as const,
    queryFn: () => control.adeTaskProofs({ taskId: toValue(taskId) }),
  });
}

/** Closed proof sessions no task holds; read only while the attach menu is open. */
export function useProofChoices(enabled: MaybeRefOrGetter<boolean>) {
  return useQuery({
    queryKey: proofChoicesKey,
    queryFn: () => control.adeTaskProofChoices(),
    staleTime: 0,
    enabled: () => toValue(enabled),
  });
}

function refreshProofs(): Promise<unknown> {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: proofsPrefix }),
    queryClient.invalidateQueries({ queryKey: proofChoicesKey }),
  ]);
}

export function useLinkProof() {
  return useMutation({
    mutationFn: (args: LinkProofArgs) => control.adeTaskLinkProof(args),
    onSuccess: refreshProofs,
  });
}

export function useUnlinkProof() {
  return useMutation({
    mutationFn: (args: LinkProofArgs) => control.adeTaskUnlinkProof(args),
    onSuccess: refreshProofs,
  });
}

/** The task's artifacts, detected and stored; a board push refetches it while mounted. */
export function useTaskArtifacts(taskId: MaybeRefOrGetter<string>) {
  return useQuery({
    queryKey: [...artifactsPrefix, taskId] as const,
    queryFn: () => control.adeTaskArtifacts({ taskId: toValue(taskId) }),
  });
}

/**
 * The issue a task links, as its tracker reports it. The tracker caches for a minute, so the same
 * window keeps a remount from asking again; a board push refetches it.
 */
export function useTaskIssue(taskId: MaybeRefOrGetter<string>, link: MaybeRefOrGetter<string>) {
  return useQuery({
    queryKey: [...issuePrefix, taskId, link] as const,
    queryFn: () => control.adeTaskIssue({ taskId: toValue(taskId) }),
    staleTime: 60_000,
  });
}

const refetchArtifacts = () => queryClient.invalidateQueries({ queryKey: artifactsPrefix });

export function useAddLink() {
  return useMutation({
    mutationFn: (args: AddLinkArgs) => control.adeTaskAddLink(args),
    onSuccess: refetchArtifacts,
  });
}

export function useEditLink() {
  return useMutation({
    mutationFn: (args: EditLinkArgs) => control.adeTaskEditLink(args),
    onSuccess: refetchArtifacts,
  });
}

export function useRemoveArtifact() {
  return useMutation({
    mutationFn: (args: RemoveArtifactArgs) => control.adeTaskRemoveArtifact(args),
    onSuccess: refetchArtifacts,
  });
}

/** Appends a user note to the task's chain. */
export function useAddNote() {
  return useMutation({
    mutationFn: (args: AddNoteArgs) => control.adeTaskAddNote(args),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: handoversPrefix }),
  });
}

export function useUpdateHandover() {
  return useMutation({
    mutationFn: (args: UpdateHandoverArgs) => control.adeTaskUpdateHandover(args),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: handoversPrefix }),
  });
}

export function useDismissHandover() {
  return useMutation({ mutationFn: (args: TaskArgs) => control.adeTaskDismissHandover(args) });
}

/** A branch's review step for its review window; the review, board and runs pushes refetch it. */
export function useReviewStep(branchId: MaybeRefOrGetter<string>) {
  return useQuery({
    queryKey: [...reviewStepPrefix, branchId] as const,
    queryFn: () => control.adeTaskReviewStep({ branchId: toValue(branchId) }),
  });
}

export function useSendReview() {
  return useMutation({ mutationFn: (args: SendReviewArgs) => control.adeTaskSendReview(args) });
}

export function useFinishReview() {
  return useMutation({ mutationFn: (args: FinishReviewArgs) => control.adeTaskFinishReview(args) });
}

export function useForwardReviewComments() {
  return useMutation({
    mutationFn: (args: BranchArgs): Promise<ForwardReviewResult> =>
      control.adeTaskForwardReviewComments(args),
  });
}

export function useReviewForwardText() {
  return useMutation({
    mutationFn: (args: BranchArgs): Promise<ReviewForwardText> =>
      control.adeTaskReviewForwardText(args),
  });
}

export function useLaunchReviewAgent() {
  return useMutation({
    mutationFn: (args: TaskArgs): Promise<ReviewAgentLaunch> =>
      control.adeTaskLaunchReviewAgent(args),
  });
}

/** The bases the picker offers for a repo; read on open (git moves between opens), never pushed. */
export function useRepoBranches(args: MaybeRefOrGetter<RepoBranchesArgs | null>) {
  return useQuery(() => {
    const a = toValue(args);
    return {
      queryKey: ['adetask', 'repoBranches', a] as const,
      queryFn: (): Promise<RepoBranches> => control.adeTaskRepoBranches(a as RepoBranchesArgs),
      staleTime: 0,
      enabled: a !== null,
    };
  });
}

/** The prompt, stack and blockers a Rebase would use; the key holds the pick, so a pick refetches. */
export function useRebasePreview(args: MaybeRefOrGetter<OntoArgs | null>) {
  return useQuery(() => {
    const a = toValue(args);
    return {
      queryKey: ['adetask', 'rebasePreview', a] as const,
      queryFn: (): Promise<RebasePreview> => control.adeTaskRebasePreview(a as OntoArgs),
      staleTime: 0,
      enabled: a !== null,
    };
  });
}

export function useRebase() {
  return useMutation({
    mutationFn: (args: RebaseArgs): Promise<RebaseStart> => control.adeTaskRebase(args),
  });
}

export function useAbortRebase() {
  return useMutation({ mutationFn: (args: BranchArgs) => control.adeTaskAbortRebase(args) });
}

export function useSetBranchBase() {
  return useMutation({
    mutationFn: (args: SetBranchBaseArgs) => control.adeTaskSetBranchBase(args),
  });
}

/** Renders registry prompt `id` in Go for `vars`; idle while `vars` is null. */
export function useAdePromptPreview(
  id: string,
  vars: MaybeRefOrGetter<Record<string, unknown> | null>,
) {
  return useQuery(() => {
    const v = toValue(vars);
    return {
      queryKey: ['llmPrompts', 'adePreview', id, v] as const,
      queryFn: (): Promise<LlmPromptPreview> =>
        control.llmPromptsPreview({ id, vars: v as Record<string, unknown> }),
      enabled: v !== null,
      placeholderData: keepPreviousData,
      staleTime: 0,
    };
  });
}
