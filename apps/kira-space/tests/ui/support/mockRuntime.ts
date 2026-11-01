import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import { defaultLayout } from '@shared/domain/layout';
import {
  buildChannelMaps,
  type ControlMockHandle,
  resolveWailsRuntimeJsPath,
  emitWailsEvent as sharedEmitWailsEvent,
  installControlMocks as sharedInstallControlMocks,
} from '@workbench/testing/ui/mockRuntime';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { contract } from './contract';
import { IPC } from './ipcChannels';
import type { ControlSnapshot } from './types';

export type { ControlLogEntry, ControlMockHandle } from '@workbench/testing/ui/mockRuntime';

// The real Wails runtime, served under /wails/ — Kira Studio's own tests/ui/support/mockRuntime.ts,
// ported and trimmed to this app's own bound surface (bridge/index.ts's `control` object,
// apps/kira-space/main.go's 13 services). See @workbench/testing/ui/mockRuntime's own header for
// why this is the real runtime.js bundle, not a hand-rolled stand-in, and why `go list` resolves
// its path rather than a hand-written GOPATH-shaped one.
const WAILS_RUNTIME_JS = resolveWailsRuntimeJsPath(resolve(__dirname, '../../../'));

const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge';

// One line per request/response channel — the FQN half of each pair, read off the generated
// bindings themselves (`grep -rhoE '\$Call\.ByName\("[^"]+"'
// apps/kira-space/frontend/bindings/…/bridge/*.js`),
// not retyped from memory — the same discipline Kira Studio's own copy of this table follows.
const FQN_SUFFIX_BY_IPC_KEY: Record<string, string> = {
  gitHostOpenChangeRequestUrl: 'GitHostService.OpenChangeRequestURL',
  gitHostProviders: 'GitHostService.Providers',
  hostViewOpen: 'HostViewService.Open',
  hostViewWindowTarget: 'HostViewService.WindowTarget',
  hostViewChangeRequest: 'HostViewService.ChangeRequest',
  hostViewIssue: 'HostViewService.Issue',
  hostViewPostComment: 'HostViewService.PostComment',
  linkOpenExternal: 'LinkService.OpenExternal',
  captureWindowItem: 'CaptureService.WindowItem',
  captureConfirm: 'CaptureService.Confirm',
  captureDismiss: 'CaptureService.Dismiss',
  captureStatus: 'CaptureService.Status',
  settingsGetAll: 'SettingsService.GetAll',
  settingsSet: 'SettingsService.Set',
  layoutGetAll: 'LayoutService.GetAll',
  layoutSet: 'LayoutService.Set',
  appFlushed: 'LifecycleService.Flushed',
  windowFlushed: 'LifecycleService.WindowFlushed',
  lifecycleConfirmQuit: 'LifecycleService.ConfirmQuit',
  lifecycleCancelQuit: 'LifecycleService.CancelQuit',
  lifecycleQuitStatus: 'LifecycleService.QuitStatus',
  filesChooseFolder: 'FilesService.ChooseFolder',
  mobileStatusGet: 'MobileAccessService.Status',
  mobileSetEnabled: 'MobileAccessService.SetEnabled',
  mobileSetPort: 'MobileAccessService.SetPort',
  mobileTrustNetwork: 'MobileAccessService.TrustCurrentNetwork',
  mobileForgetNetwork: 'MobileAccessService.ForgetNetwork',
  mobileSetAgentInput: 'MobileAccessService.SetAgentInputEnabled',
  mobileSetDevicePermissions: 'MobileAccessService.SetDevicePermissions',
  mobileDevicesList: 'MobileAccessService.Devices',
  mobileRevoke: 'MobileAccessService.Revoke',
  mobileTerminalHolds: 'MobileAccessService.TerminalHolds',
  mobileReclaimTerminal: 'MobileAccessService.ReclaimTerminal',
  mobileLaunchOpened: 'MobileAccessService.LaunchOpened',
  mobilePairingPending: 'MobileAccessService.PendingPairing',
  mobilePairingApprove: 'MobileAccessService.Approve',
  mobilePairingDeny: 'MobileAccessService.Deny',
  gitCredentialPending: 'GitCredentialService.Pending',
  gitCredentialProvide: 'GitCredentialService.Provide',
  tabsList: 'TabsService.List',
  tabsSave: 'TabsService.Save',
  codeWorkspaceListRepos: 'CodeWorkspaceService.ListRepos',
  codeWorkspaceRepoHeads: 'CodeWorkspaceService.RepoHeads',
  codeWorkspaceRepoWorktreeLinks: 'CodeWorkspaceService.RepoWorktreeLinks',
  codeWorkspaceImportRepo: 'CodeWorkspaceService.ImportRepo',
  codeWorkspaceRenameRepo: 'CodeWorkspaceService.RenameRepo',
  codeWorkspaceSetRepoColor: 'CodeWorkspaceService.SetRepoColor',
  codeWorkspaceSetRepoHidden: 'CodeWorkspaceService.SetRepoHidden',
  codeWorkspaceRemoveRepo: 'CodeWorkspaceService.RemoveRepo',
  codeWorkspaceReorderRepos: 'CodeWorkspaceService.ReorderRepos',
  codeWorkspaceListFiles: 'CodeWorkspaceService.ListFiles',
  codeWorkspaceReadFile: 'CodeWorkspaceService.ReadFile',
  codeWorkspaceReadFileForEdit: 'CodeWorkspaceService.ReadFileForEdit',
  codeWorkspaceWriteFile: 'CodeWorkspaceService.WriteFile',
  fileHandlersStatus: 'FileHandlersService.Status',
  fileHandlersSet: 'FileHandlersService.Set',
  openedFilesPending: 'OpenedFilesService.Pending',
  openedFilesRead: 'OpenedFilesService.Read',
  codeWorkspaceOpenWorkspace: 'CodeWorkspaceService.OpenWorkspace',
  codeWorkspaceCloseWorkspace: 'CodeWorkspaceService.CloseWorkspace',
  codeWorkspaceReadDiff: 'CodeWorkspaceService.ReadDiff',
  codeWorkspaceStartSearch: 'CodeWorkspaceService.StartSearch',
  codeWorkspaceCancelSearch: 'CodeWorkspaceService.CancelSearch',
  terminalDefaultCwd: 'TerminalService.DefaultCwd',
  terminalOpen: 'TerminalService.Open',
  terminalWrite: 'TerminalService.Write',
  terminalResize: 'TerminalService.Resize',
  terminalClose: 'TerminalService.Close',

  // P116 G5/G6: the two new bound methods window-chrome parity adds.
  windowsOpenNew: 'WindowsService.OpenNew',
  configFilesOpen: 'ConfigFilesService.Open',
  configFilesReveal: 'ConfigFilesService.Reveal',
  customScriptsList: 'CustomScriptsService.List',
  customScriptsCreate: 'CustomScriptsService.Create',
  customScriptsUpdate: 'CustomScriptsService.Update',
  customScriptsDelete: 'CustomScriptsService.Delete',
  customScriptsCreateCollection: 'CustomScriptsService.CreateCollection',
  customScriptsRenameCollection: 'CustomScriptsService.RenameCollection',
  customScriptsDeleteCollection: 'CustomScriptsService.DeleteCollection',
  customScriptsMove: 'CustomScriptsService.Move',
  customScriptsReorder: 'CustomScriptsService.Reorder',
  customScriptsReorderCollections: 'CustomScriptsService.ReorderCollections',
  scriptRunsList: 'ScriptRunsService.List',
  scriptRunsNextFires: 'ScriptRunsService.NextFires',
  scriptRunsSchedulePreview: 'ScriptRunsService.SchedulePreview',
  scriptRunsRunScheduleNow: 'ScriptRunsService.RunScheduleNow',
  scriptRunsConfirmAccept: 'ScriptRunsService.ConfirmAccept',
  scriptRunsConfirmDecline: 'ScriptRunsService.ConfirmDecline',
  promptsList: 'PromptsService.List',
  promptsMainWindow: 'PromptsService.MainWindow',
  promptsClaim: 'PromptsService.Claim',
  promptsRaise: 'PromptsService.Raise',
  promptsDismiss: 'PromptsService.Dismiss',
  promptsSendTest: 'PromptsService.SendTest',
  agentInfo: 'AgentService.Info',
  agentInfoFor: 'AgentService.InfoFor',
  llmPromptsList: 'LlmPromptsService.List',
  llmPromptsSave: 'LlmPromptsService.Save',
  llmPromptsPreview: 'LlmPromptsService.Preview',
  scriptRunsGet: 'ScriptRunsService.Get',
  scriptRunsStop: 'ScriptRunsService.Stop',
  scriptRunsResume: 'ScriptRunsService.Resume',
  scriptRunsResumeCommand: 'ScriptRunsService.ResumeCommand',
  scriptRunsRestoreState: 'ScriptRunsService.RestoreState',
  scriptRunsRestore: 'ScriptRunsService.Restore',
  scriptRunsRestoreDismiss: 'ScriptRunsService.RestoreDismiss',
  scriptRunsResolveDir: 'ScriptRunsService.ResolveDir',
  scriptRunsPreview: 'ScriptRunsService.Preview',
  scriptRunsStart: 'ScriptRunsService.Start',
  scriptRunsReadLog: 'ScriptRunsService.ReadLog',
  scriptRunsMcpServers: 'ScriptRunsService.McpServers',
  keepAwakeStatus: 'KeepAwakeService.Status',
  keepAwakeSetManual: 'KeepAwakeService.SetManual',
  agentNotifyReportFocus: 'AgentNotifyService.ReportFocus',
  agentNotifySendTest: 'AgentNotifyService.SendTest',
  agentUsageGet: 'AgentUsageService.Get',

  // P128 §2.2/§2.6: this app now persists a per-window module mode too, via the same shared
  // internal/windowsvc.Service Kira Studio's own WindowsService embeds.
  windowsEnsure: 'WindowsService.Ensure',
  windowsSetMode: 'WindowsService.SetMode',

  // P132 Part 2: the operations dock's own two bound calls.
  opsRecent: 'OpsService.Recent',
  opsCancel: 'OpsService.Cancel',

  // P119: the in-app update dialog's three bound calls.
  updateStatus: 'UpdateService.Status',
  updateCheck: 'UpdateService.Check',
  updateInstall: 'UpdateService.InstallUpdate',
  updateCancelInstall: 'UpdateService.CancelInstall',

  // Boot: the ade module's agent-session hydration.
  terminalAgentSessions: 'TerminalService.AgentSessions',

  // P145: the ade v2 board surface (AdeTaskService, bridge/index.ts `adeTask*`).
  adeTaskBoard: 'AdeTaskService.Board',
  adeTaskChangeRequests: 'AdeTaskService.ChangeRequests',
  adeTaskRefresh: 'AdeTaskService.Refresh',
  adeTaskForcePush: 'AdeTaskService.ForcePush',
  adeTaskCreateTask: 'AdeTaskService.CreateTask',
  adeTaskCandidateBranches: 'AdeTaskService.CandidateBranches',
  adeTaskAddExistingBranch: 'AdeTaskService.AddExistingBranch',
  adeTaskResolveHostLink: 'AdeTaskService.ResolveHostLink',
  adeTaskAddHostLink: 'AdeTaskService.AddHostLink',
  adeTaskUpdateTask: 'AdeTaskService.UpdateTask',
  adeTaskAddTaskRepo: 'AdeTaskService.AddTaskRepo',
  adeTaskSetPlan: 'AdeTaskService.SetPlan',
  adeTaskShelveTask: 'AdeTaskService.ShelveTask',
  adeTaskCaptureTask: 'AdeTaskService.CaptureTask',
  adeTaskWorkflows: 'AdeTaskService.Workflows',
  adeTaskRepos: 'AdeTaskService.Repos',
  adeTaskWorkflowYaml: 'AdeTaskService.WorkflowYaml',
  adeTaskValidateWorkflowYaml: 'AdeTaskService.ValidateWorkflowYaml',
  adeTaskSaveWorkflow: 'AdeTaskService.SaveWorkflow',
  adeTaskSaveWorkflowYaml: 'AdeTaskService.SaveWorkflowYaml',
  adeTaskImportWorkflow: 'AdeTaskService.ImportWorkflow',
  adeTaskNewWorkflow: 'AdeTaskService.NewWorkflow',
  adeTaskUpdateRepo: 'AdeTaskService.UpdateRepo',
  adeTaskAddFolder: 'AdeTaskService.AddFolder',
  adeTaskSetFolderWatch: 'AdeTaskService.SetFolderWatch',
  adeTaskSetFolderHidden: 'AdeTaskService.SetFolderHidden',
  adeTaskRemoveFolder: 'AdeTaskService.RemoveFolder',
  adeTaskStartRun: 'AdeTaskService.StartRun',
  adeTaskSetTaskWorkflow: 'AdeTaskService.SetTaskWorkflow',
  adeTaskApprove: 'AdeTaskService.Approve',
  adeTaskRetryRun: 'AdeTaskService.RetryRun',
  adeTaskResumeRun: 'AdeTaskService.ResumeRun',
  adeTaskStepDone: 'AdeTaskService.StepDone',
  adeTaskHandovers: 'AdeTaskService.Handovers',
  adeTaskUpdateHandover: 'AdeTaskService.UpdateHandover',
  adeTaskDismissHandover: 'AdeTaskService.DismissHandover',
  adeTaskArtifacts: 'AdeTaskService.Artifacts',
  adeTaskIssue: 'IssueTrackerService.TaskIssue',
  cliTools: 'IssueTrackerService.Tools',
  issueTrackers: 'IssueTrackerService.Trackers',
  adeTaskAddArtifact: 'AdeTaskService.AddArtifact',
  adeTaskRemoveArtifact: 'AdeTaskService.RemoveArtifact',
  adeTaskSetTaskStep: 'AdeTaskService.SetTaskStep',
  adeTaskSaveStatuses: 'AdeTaskService.SaveStatuses',
  adeTaskRetrySetup: 'AdeTaskService.RetrySetup',
  adeTaskReadLog: 'AdeTaskService.ReadLog',
  adeTaskRunSandbox: 'AdeTaskService.RunSandbox',
  adeTaskSessions: 'AdeTaskService.Sessions',
  adeTaskStopRun: 'AdeTaskService.StopRun',
  adeTaskReviewStep: 'AdeTaskService.ReviewStep',
  adeTaskSendReview: 'AdeTaskService.SendReview',
  adeTaskFinishReview: 'AdeTaskService.FinishReview',
  adeTaskTakeOver: 'AdeTaskService.TakeOver',
  adeTaskRestartSession: 'AdeTaskService.RestartSession',
  adeTaskCloseSession: 'AdeTaskService.CloseSession',
  adeTaskLaunchStep: 'AdeTaskService.LaunchStep',
  adeTaskStartBranch: 'AdeTaskService.StartBranch',
  adeTaskStartAdHoc: 'AdeTaskService.StartAdHoc',
  adeTaskSend: 'AdeTaskService.Send',
  adeTaskFocusSession: 'AdeTaskService.FocusSession',
  adeTaskOpenReviewWindow: 'AdeTaskService.OpenReviewWindow',
  adeTaskReviewWindowTarget: 'AdeTaskService.ReviewWindowTarget',
  adeTaskReviewWorking: 'AdeTaskService.ReviewWorking',
  adeTaskReviewWorkingDiff: 'AdeTaskService.ReviewWorkingDiff',
  adeTaskReviewWorkingFile: 'AdeTaskService.ReviewWorkingFile',
  adeTaskReviewWorkingMark: 'AdeTaskService.ReviewWorkingMark',
  adeTaskReviewWorkingComments: 'AdeTaskService.ReviewWorkingComments',
  adeTaskReviewWorkingCommentAdd: 'AdeTaskService.ReviewWorkingCommentAdd',
  adeTaskReviewAgent: 'AdeTaskService.ReviewAgent',
  adeTaskForwardReviewComments: 'AdeTaskService.ForwardReviewComments',
  adeTaskReviewForwardText: 'AdeTaskService.ReviewForwardText',
  adeTaskLaunchReviewAgent: 'AdeTaskService.LaunchReviewAgent',
  adeTaskArchiveRisk: 'AdeTaskService.ArchiveRisk',
  adeTaskArchiveTask: 'AdeTaskService.ArchiveTask',
  adeTaskArchivedTask: 'AdeTaskService.ArchivedTask',
  adeTaskBranchRequest: 'AdeTaskService.BranchRequest',
  adeTaskApproveBranchRequest: 'AdeTaskService.ApproveBranchRequest',
  adeTaskDenyBranchRequest: 'AdeTaskService.DenyBranchRequest',
  adeTaskRestoreTask: 'AdeTaskService.RestoreTask',
  adeTaskRecordMerge: 'AdeTaskService.RecordMerge',
  adeTaskSetQueuedAfter: 'AdeTaskService.SetQueuedAfter',
  adeTaskRebasePreview: 'AdeTaskService.RebasePreview',
  adeTaskRebase: 'AdeTaskService.Rebase',
  adeTaskAbortRebase: 'AdeTaskService.AbortRebase',
  adeTaskSetBranchBase: 'AdeTaskService.SetBranchBase',
  adeTaskRepoBranches: 'AdeTaskService.RepoBranches',
  adeTaskSetTaskBranch: 'AdeTaskService.SetTaskBranch',
  adeTaskUpdateTaskRepo: 'AdeTaskService.UpdateTaskRepo',
  adeTaskRemoveTaskBranch: 'AdeTaskService.RemoveTaskBranch',
  adeTaskRemoveTaskRepo: 'AdeTaskService.RemoveTaskRepo',
  adeTaskTaskBranchRisk: 'AdeTaskService.TaskBranchRisk',

  memorySearch: 'MemoryService.Search',
  memoryRecent: 'MemoryService.Recent',
  memoryHistory: 'MemoryService.History',
  memoryStore: 'MemoryService.Store',
  memoryContext: 'MemoryService.Context',
  memoryProjects: 'MemoryService.Projects',
  memoryMcpStatus: 'MemoryService.McpStatus',
  memoryCheckHook: 'MemoryService.CheckHook',
  memoryMcpInstall: 'MemoryService.InstallAgent',
  memorySemanticStatus: 'MemoryService.SemanticStatus',
  memorySemanticInstall: 'MemoryService.InstallSemanticModel',
  memorySemanticRetry: 'MemoryService.RetrySemantic',
  memoryImportChoose: 'MemoryImportService.Choose',
  memoryImportCreate: 'MemoryImportService.Create',
  memoryImportJobs: 'MemoryImportService.Jobs',
  memoryImportJob: 'MemoryImportService.Job',
  memoryImportStart: 'MemoryImportService.Start',
  memoryImportPause: 'MemoryImportService.Pause',
  memoryImportResume: 'MemoryImportService.Resume',
  memoryImportCancel: 'MemoryImportService.Cancel',
  memoryImportDiscard: 'MemoryImportService.Discard',
  memoryImportDismiss: 'MemoryImportService.Dismiss',
  memoryImportRetryFailed: 'MemoryImportService.RetryFailed',
  memoryImportRetryFile: 'MemoryImportService.RetryFile',
  memoryDistillRun: 'MemoryDistillService.Distill',
  memoryDistillLimits: 'MemoryDistillService.Limits',
  memoryDistillChoose: 'MemoryDistillService.Choose',
  memoryDistillToImport: 'MemoryDistillService.ToImport',
};

export const { channelToFqn: CHANNEL_TO_FQN, fqnToChannel: FQN_TO_CHANNEL } = buildChannelMaps(
  IPC,
  FQN_SUFFIX_BY_IPC_KEY,
  BRIDGE_PKG,
);

// A call this mock never expects a fixture to cover, answered the same way regardless of a
// spec's own args — pre-serialised JSON, keyed by channel; used only when a channel has *no*
// fixture-supplied snapshot at all (a spec that provides its own still wins). Every member here is
// one of main.ts's own unconditional-every-boot Promise.all calls (bootstrap()'s own doc comment)
// no committed fixture will ever snapshot, or a fire-and-forget call no spec asserts on the echo
// of — the same reasoning Kira Studio's own WILDCARD_DEFAULTS carries per entry, trimmed to this
// app's own boot sequence. `windowsEnsure` joins this table with a static `'git'`
// answer, unlike Kira Studio's own copy — this app has no existing spec whose boot mode needs
// inferring from an already-provided fixture (Studio's own inferredBootMode helper exists only
// because some of its specs restore an Api-mode tab as active); every Space spec boots into `git`
// today, and a spec that wants `terminal`/`ade` instead (modules.spec.ts) provides its own
// `windowsEnsure` snapshot, which always wins over this default.
const WILDCARD_DEFAULTS: Readonly<Record<string, string>> = Object.freeze({
  [IPC.tabsSave]: 'null',
  // Boot drains Finder-opened files; none by default.
  [IPC.openedFilesPending]: '[]',
  [IPC.windowsEnsure]: JSON.stringify({ mode: 'git' }),
  [IPC.windowsSetMode]: 'null',
  // P246: the prompt router's calls; a spec drives the list through IPC.promptsChanged pushes.
  [IPC.promptsList]: '[]',
  [IPC.promptsClaim]: 'null',
  [IPC.promptsRaise]: 'null',
  [IPC.promptsDismiss]: 'null',
  [IPC.promptsSendTest]: 'null',
  // The active agent provider's names: the claude provider, as the Go flow test writes it.
  [IPC.agentInfo]: JSON.stringify(contract('agent', 'AgentService.Info')),
  [IPC.agentInfoFor]: JSON.stringify(contract('agent', 'AgentService.Info')),
  // The prompt editor's calls; a spec answers List/Save from tests/contract/llm-prompts.json.
  [IPC.llmPromptsList]: '{"prompts":[],"storeError":"","revision":0}',
  [IPC.llmPromptsSave]: '{"prompts":[],"storeError":"","revision":0}',
  [IPC.llmPromptsPreview]: '{"errors":[],"parts":[]}',
  [IPC.layoutSet]: JSON.stringify(defaultLayout),
  [IPC.settingsSet]: JSON.stringify(defaultSettings),
  // P132 Part 2: main.ts's bootstrap() hydrates the op log on every boot.
  [IPC.opsRecent]: '[]',
  [IPC.opsCancel]: 'null',
  [IPC.mobileStatusGet]: JSON.stringify({
    enabled: false,
    running: false,
    port: 7790,
    appUrl: '',
    agentInput: false,
    error: '',
    stopReason: '',
    stopDetail: '',
    current: null,
    trusted: null,
    trustedAt: 0,
  }),
  [IPC.mobileDevicesList]: '[]',
  [IPC.mobileTerminalHolds]: '[]',
  [IPC.mobilePairingPending]: JSON.stringify({ pending: null, queued: 0 }),
  [IPC.gitCredentialPending]: '[]',
  [IPC.gitCredentialProvide]: 'false',
  [IPC.codeWorkspaceListRepos]: '[]',
  [IPC.codeWorkspaceRepoHeads]: '[]',
  [IPC.codeWorkspaceRepoWorktreeLinks]: '[]',
  [IPC.terminalDefaultCwd]: JSON.stringify({ path: '/home/test' }),
  [IPC.codeWorkspaceOpenWorkspace]: 'null',
  [IPC.codeWorkspaceCloseWorkspace]: 'null',
  [IPC.codeWorkspaceCancelSearch]: 'null',
  // main.ts's bootstrap() joins keepAwakeStore.initKeepAwake() into the optional
  // Promise.allSettled group, same reasoning as the rest of this table's every-boot calls. The
  // plan (docs/HISTORY.md) names `supported: false` here, but this
  // deliberately follows Kira Studio's own WILDCARD_DEFAULTS value instead (`supported: true`) —
  // Studio's own comment explains why: the UI suite runs against a static server, not a real Go
  // build, and a spec that never cares about keep-awake should still see the titlebar button it
  // will ship with. A spec that DOES care (window-chrome.spec.ts's own keep-awake cases) still wins
  // with its own snapshot.
  // Every window reports its focus at boot.
  [IPC.agentNotifyReportFocus]: 'null',
  [IPC.agentNotifySendTest]: 'null',
  // P239: nothing reported yet.
  [IPC.agentUsageGet]: JSON.stringify({
    state: 'waiting',
    source: '',
    fiveHour: null,
    sevenDay: null,
    updatedAt: 0,
  }),
  [IPC.keepAwakeStatus]: JSON.stringify({ manual: false, supported: true, error: '' }),
  // P119: no update available by default — same shape as Kira Studio's own WILDCARD_DEFAULTS
  // entry, so every other existing Space spec keeps booting unchanged.
  [IPC.updateStatus]: JSON.stringify({
    updateAvailable: false,
    currentVersion: '0.0.0-dev',
    latestVersion: '',
    installLogPath: '',
  }),
  [IPC.updateCheck]: JSON.stringify({
    updateAvailable: false,
    currentVersion: '0.0.0-dev',
    latestVersion: '',
    installLogPath: '',
  }),
  // Every boot calls TerminalService.AgentSessions (createAgentSessionsStore's own initAgentSessions);
  // no committed fixture will ever snapshot it.
  [IPC.terminalAgentSessions]: JSON.stringify({ sessions: [] }),
  // main.ts asks every window whether it is a host view or review window; none of the specs' windows is one.
  [IPC.hostViewWindowTarget]: 'null',
  [IPC.captureWindowItem]: 'null',
  [IPC.captureStatus]: JSON.stringify({ state: 'active', accelerator: 'CmdOrCtrl+Shift+K' }),
  [IPC.adeTaskReviewWindowTarget]: 'null',
  [IPC.adeTaskReviewWorking]: JSON.stringify({
    worktree: '',
    head: '',
    files: [],
    reviews: {},
    promoted: [],
  }),
  [IPC.adeTaskReviewWorkingComments]: JSON.stringify({ comments: [] }),
  // P286: the review window asks for its branch's review step; none by default.
  [IPC.adeTaskReviewStep]: JSON.stringify({
    run: null,
    stepName: '',
    results: [],
    route: 'implementer',
    targetRunId: '',
    defaultTargetRunId: '',
    candidates: [],
    round: null,
  }),
  // P201 Part 2: the Memory module lists recent memories as soon as its mode opens.
  [IPC.memoryRecent]: '[]',
  // P272: no project keys by default.
  [IPC.memoryProjects]: '[]',
  // P273: no check commands by default.
  [IPC.memoryCheckHook]: JSON.stringify({ executable: '', installCommand: '', hookLine: '' }),
  // P211: no imports by default.
  [IPC.memoryImportJobs]: '[]',
  // P210: no embedder by default, so the semantic row stays hidden.
  [IPC.memorySemanticStatus]: JSON.stringify({
    state: 'off',
    message: '',
    model: '',
    done: 0,
    total: 0,
  }),
});

// `windowKey`/`tabId` are excluded outright — a per-window or per-tab id this app generates at
// runtime, never reproducible from a fixture.
const CANONICAL_OPTIONS = { excludeKeys: ['windowKey', 'tabId', 'terminalId'] };

/**
 * Replaces the control channel's answers at the network layer — `page.route` intercepts every
 * request under `/wails/`: the real runtime bundle itself and the one RPC endpoint bound calls
 * POST to. The mocked HTTP response is exactly what `unwrap`/`trust` in `bridge/rpc.ts` are written
 * to consume, so a frontend spec still exercises that code for real.
 */
export async function installControlMocks(
  page: Page,
  snapshots: readonly ControlSnapshot[],
): Promise<ControlMockHandle> {
  const handle = await sharedInstallControlMocks(page, snapshots, {
    fqnToChannel: FQN_TO_CHANNEL,
    wildcardDefaults: WILDCARD_DEFAULTS,
    runtimeJsPath: WAILS_RUNTIME_JS,
    canonicalOptions: CANONICAL_OPTIONS,
  });
  emulateAdeTaskPush(page);
  return handle;
}

// Every ade write emits its `kira:adetask:*` push and the renderer refetches off that push alone,
// so the mock repeats the push after every successful write (the refetch answers the same static
// snapshot the spec supplied). Move is left out: its optimistic order is the final state.
const ADE_TASK_PUSHES: Readonly<Record<string, readonly string[]>> = {
  [IPC.adeTaskSetPlan]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskShelveTask]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskCaptureTask]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskCreateTask]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskAddExistingBranch]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskAddHostLink]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskForcePush]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskRefresh]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskUpdateTask]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskAddTaskRepo]: [IPC.adeTaskBoardChanged, IPC.adeTaskReposChanged],
  [IPC.adeTaskSetTaskWorkflow]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskStartRun]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskApprove]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskRetryRun]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskResumeRun]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskStepDone]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskSetTaskStep]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskUpdateHandover]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskDismissHandover]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskAddArtifact]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskRemoveArtifact]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskSaveStatuses]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskRetrySetup]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskStopRun]: [IPC.adeTaskSessionsChanged],
  [IPC.adeTaskSendReview]: [IPC.adeTaskBoardChanged, IPC.adeTaskReviewChanged],
  [IPC.adeTaskFinishReview]: [IPC.adeTaskBoardChanged, IPC.adeTaskReviewChanged],
  [IPC.adeTaskTakeOver]: [IPC.adeTaskSessionsChanged],
  [IPC.adeTaskRestartSession]: [IPC.adeTaskSessionsChanged],
  [IPC.adeTaskCloseSession]: [IPC.adeTaskSessionsChanged],
  [IPC.adeTaskLaunchStep]: [IPC.adeTaskSessionsChanged],
  [IPC.adeTaskStartBranch]: [IPC.adeTaskSessionsChanged],
  [IPC.adeTaskStartAdHoc]: [IPC.adeTaskSessionsChanged],
  [IPC.adeTaskArchiveTask]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskRestoreTask]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskRecordMerge]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskSetQueuedAfter]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskRebase]: [IPC.adeTaskBoardChanged, IPC.adeTaskRunsChanged],
  [IPC.adeTaskAbortRebase]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskSetBranchBase]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskSetTaskBranch]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskUpdateTaskRepo]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskRemoveTaskBranch]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskRemoveTaskRepo]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskSaveWorkflow]: [IPC.adeTaskWorkflowsChanged],
  [IPC.adeTaskSaveWorkflowYaml]: [IPC.adeTaskWorkflowsChanged],
  [IPC.adeTaskImportWorkflow]: [IPC.adeTaskWorkflowsChanged],
  [IPC.adeTaskNewWorkflow]: [IPC.adeTaskWorkflowsChanged],
  [IPC.adeTaskUpdateRepo]: [IPC.adeTaskReposChanged],
  [IPC.adeTaskAddFolder]: [IPC.adeTaskReposChanged],
  [IPC.adeTaskSetFolderWatch]: [IPC.adeTaskReposChanged],
  [IPC.adeTaskSetFolderHidden]: [IPC.adeTaskReposChanged],
  [IPC.adeTaskRemoveFolder]: [IPC.adeTaskReposChanged],
};

function emulateAdeTaskPush(page: Page): void {
  page.on('requestfinished', async (request) => {
    if (request.method() !== 'POST' || !request.url().includes('/wails/')) return;
    try {
      const body = JSON.parse(request.postData() ?? '{}') as { args?: { methodName?: string } };
      const channel = FQN_TO_CHANNEL[body.args?.methodName ?? ''];
      const pushes = channel ? ADE_TASK_PUSHES[channel] : undefined;
      if (!pushes) return;
      const response = await request.response();
      if (!response?.ok()) return;
      for (const push of pushes) await emitWailsEvent(page, push, null);
    } catch {
      // page closed mid-request
    }
  });
}

export const emitWailsEvent = sharedEmitWailsEvent;
