import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import { defaultLayout } from '@shared/domain/layout';
import {
  buildChannelMaps,
  type ControlMockHandle,
  type ControlSnapshot as MockSnapshot,
  resolveWailsRuntimeJsPath,
  emitWailsEvent as sharedEmitWailsEvent,
  installControlMocks as sharedInstallControlMocks,
} from '@workbench/testing/ui/mockRuntime';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { STUDIO_TAB_KIND_MODE, type StudioTabKind } from '../../../frontend/src/state/tabDomain';
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
  gitResetWindow: 'GitService.ResetWindow',
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
  openedFilesIsFilesWindow: 'OpenedFilesService.IsFilesWindow',
  openedFilesPending: 'OpenedFilesService.Pending',
  openedFilesRead: 'OpenedFilesService.Read',
  openedFilesTerminalResolve: 'OpenedFilesService.TerminalResolve',
  openedFilesTerminalOpen: 'OpenedFilesService.TerminalOpen',
  codeWorkspaceOpenWorkspace: 'CodeWorkspaceService.OpenWorkspace',
  codeWorkspaceCloseWorkspace: 'CodeWorkspaceService.CloseWorkspace',
  codeWorkspaceShutdown: 'CodeWorkspaceService.Shutdown',
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
  repoConfigList: 'RepoConfigService.List',
  repoConfigFile: 'RepoConfigService.File',
  repoConfigSaveFile: 'RepoConfigService.SaveFile',
  repoConfigForm: 'RepoConfigService.Form',
  agentContextGet: 'AgentContextService.Get',
  agentContextSave: 'AgentContextService.Save',
  agentSettingsGet: 'AgentSettingsService.Get',
  agentSettingsSave: 'AgentSettingsService.Save',
  agentSettingsSetSecret: 'AgentSettingsService.SetSecret',
  agentSettingsDeleteSecret: 'AgentSettingsService.DeleteSecret',
  agentPortList: 'AgentPortService.List',
  agentPortImportPreview: 'AgentPortService.ImportPreview',
  agentPortImportApply: 'AgentPortService.ImportApply',
  agentPortExportPreview: 'AgentPortService.ExportPreview',
  agentPortExportApply: 'AgentPortService.ExportApply',
  agentPortCancel: 'AgentPortService.Cancel',
  configFilesOpen: 'ConfigFilesService.Open',
  configFilesReveal: 'ConfigFilesService.Reveal',
  customScriptsList: 'CustomScriptsService.List',
  customScriptsCreate: 'CustomScriptsService.Create',
  customScriptsDelete: 'CustomScriptsService.Delete',
  customScriptsFile: 'CustomScriptsService.File',
  customScriptsSaveFile: 'CustomScriptsService.SaveFile',
  customScriptsForm: 'CustomScriptsService.Form',
  customScriptsSetSecrets: 'CustomScriptsService.SetSecrets',
  customScriptsSetScheduleTarget: 'CustomScriptsService.SetScheduleTarget',
  customScriptsReorder: 'CustomScriptsService.Reorder',
  customScriptsReorderCollections: 'CustomScriptsService.ReorderCollections',
  customScriptsMove: 'CustomScriptsService.Move',
  customScriptsRenameCollection: 'CustomScriptsService.RenameCollection',
  customScriptsDeleteCollection: 'CustomScriptsService.DeleteCollection',
  scriptRunsList: 'ScriptRunsService.List',
  scriptRunsNextFires: 'ScriptRunsService.NextFires',
  scriptRunsSchedulePreview: 'ScriptRunsService.SchedulePreview',
  scriptRunsRunScheduleNow: 'ScriptRunsService.RunScheduleNow',
  scriptRunsConfirmAccept: 'ScriptRunsService.ConfirmAccept',
  scriptRunsConfirmDecline: 'ScriptRunsService.ConfirmDecline',
  approvalsList: 'ApprovalsService.List',
  approvalsAnswer: 'ApprovalsService.Answer',
  promptsList: 'PromptsService.List',
  promptsMainWindow: 'PromptsService.MainWindow',
  promptsClaim: 'PromptsService.Claim',
  promptsRaise: 'PromptsService.Raise',
  promptsDismiss: 'PromptsService.Dismiss',
  promptsSendTest: 'PromptsService.SendTest',
  agentInfo: 'AgentService.Info',
  agentInfoFor: 'AgentService.InfoFor',
  agentList: 'AgentService.List',
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
  scriptRunsMcpServers: 'ScriptRunsService.McpServers',
  scriptRunsPreview: 'ScriptRunsService.Preview',
  scriptRunsStart: 'ScriptRunsService.Start',
  scriptRunsReadLog: 'ScriptRunsService.ReadLog',
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
  adeTaskStepYaml: 'AdeTaskService.StepYaml',
  adeTaskValidateStepYaml: 'AdeTaskService.ValidateStepYaml',
  adeTaskSaveStep: 'AdeTaskService.SaveStep',
  adeTaskSaveStepYaml: 'AdeTaskService.SaveStepYaml',
  adeTaskNewStep: 'AdeTaskService.NewStep',
  adeTaskDeleteStep: 'AdeTaskService.DeleteStep',
  adeTaskNewWorkflow: 'AdeTaskService.NewWorkflow',
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
  adeTaskAddNote: 'AdeTaskService.AddNote',
  adeTaskProofs: 'AdeTaskService.TaskProofs',
  adeTaskProofChoices: 'AdeTaskService.ProofChoices',
  adeTaskLinkProof: 'AdeTaskService.LinkProof',
  adeTaskUnlinkProof: 'AdeTaskService.UnlinkProof',
  adeTaskUpdateHandover: 'AdeTaskService.UpdateHandover',
  adeTaskDismissHandover: 'AdeTaskService.DismissHandover',
  adeTaskArtifacts: 'AdeTaskService.Artifacts',
  adeTaskIssue: 'IssueTrackerService.TaskIssue',
  adeTaskTrackedUpdates: 'AdeTaskService.TrackedUpdates',
  adeTaskMarkTrackedSeen: 'AdeTaskService.MarkTrackedSeen',
  cliTools: 'IssueTrackerService.Tools',
  issueTrackers: 'IssueTrackerService.Trackers',
  adeTaskAddLink: 'AdeTaskService.AddLink',
  adeTaskEditLink: 'AdeTaskService.EditLink',
  adeTaskRemoveArtifact: 'AdeTaskService.RemoveArtifact',
  adeTaskSetTaskStep: 'AdeTaskService.SetTaskStep',
  adeTaskSaveStatuses: 'AdeTaskService.SaveStatuses',
  adeTaskRetrySetup: 'AdeTaskService.RetrySetup',
  adeTaskReadLog: 'AdeTaskService.ReadLog',
  adeTaskRunSandbox: 'AdeTaskService.RunSandbox',
  adeTaskSessionSandbox: 'AdeTaskService.SessionSandbox',
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
  adeTaskReviewWorkingStamp: 'AdeTaskService.ReviewWorkingStamp',
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
  adeTaskHostProviders: 'AdeTaskService.HostProviders',

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

  // Databases module (formerly Kira Studio's own table).
  appInfo: 'AppService.Info',
  filesChooseSave: 'FilesService.ChooseSave',
  filesChooseOpen: 'FilesService.ChooseOpen',
  filesChooseOpenMany: 'FilesService.ChooseOpenMany',
  datagripScan: 'DataGripService.Scan',
  datagripImport: 'DataGripService.Import',
  connectionsList: 'ConnectionsService.List',
  connectionsCreate: 'ConnectionsService.Create',
  connectionsUpdateCredentials: 'ConnectionsService.UpdateCredentials',
  connectionsDuplicate: 'ConnectionsService.Duplicate',
  connectionsDelete: 'ConnectionsService.Delete',
  connectionsReorder: 'ConnectionsService.Reorder',
  connectionsReveal: 'ConnectionsService.Reveal',
  connectionsSecretsStatus: 'ConnectionsService.SecretsStatus',
  connectionsTest: 'ConnectionsService.Test',
  connectionsConnect: 'ConnectionsService.Connect',
  connectionsDisconnect: 'ConnectionsService.Disconnect',
  connectionsStates: 'ConnectionsService.States',
  maskRulesList: 'MaskRulesService.List',
  maskRulesUpsert: 'MaskRulesService.Upsert',
  maskRulesDelete: 'MaskRulesService.Delete',
  maskRulesRegenerateKey: 'MaskRulesService.RegenerateKey',
  maskRulesCounts: 'MaskRulesService.Counts',
  maskRulesCorrelationKey: 'MaskRulesService.CorrelationKey',
  treeChildren: 'TreeService.Children',
  treeKeyTypes: 'TreeService.KeyTypes',
  treeDescribe: 'TreeService.Describe',
  treeDefinition: 'TreeService.Definition',
  treeSchemaColumns: 'TreeService.SchemaColumns',
  treeInvalidate: 'TreeService.Invalidate',
  filtersList: 'FiltersService.List',
  filtersReplace: 'FiltersService.Replace',
  httpSend: 'HttpService.Send',
  // P90 item 2's three cookie bound calls.
  httpCookies: 'HttpService.Cookies',
  httpDeleteCookie: 'HttpService.DeleteCookie',
  httpClearCookies: 'HttpService.ClearCookies',
  queriesList: 'QueriesService.List',
  queriesSave: 'QueriesService.Save',
  queriesListConsole: 'QueriesService.ListConsole',
  queriesSaveConsole: 'QueriesService.SaveConsole',
  queriesListDiscover: 'QueriesService.ListDiscover',
  queriesSaveDiscover: 'QueriesService.SaveDiscover',
  queriesUpdateDiscover: 'QueriesService.UpdateDiscover',
  queriesUpdate: 'QueriesService.Update',
  queriesDelete: 'QueriesService.Delete',
  queriesTouch: 'QueriesService.Touch',
  queriesHistoryList: 'QueriesService.HistoryList',
  queriesHistoryRecord: 'QueriesService.HistoryRecord',
  schemaGet: 'SchemaService.Get',
  schemaSet: 'SchemaService.Set',
  kibanaDataViews: 'KibanaService.DataViews',
  kibanaDataView: 'KibanaService.DataView',
  kibanaHistogram: 'KibanaService.Histogram',
  kibanaSuggest: 'KibanaService.Suggest',
  collectionsList: 'CollectionsService.List',
  collectionsGetRequest: 'CollectionsService.GetRequest',
  collectionsSaveRequest: 'CollectionsService.SaveRequest',
  collectionsCreateCollection: 'CollectionsService.CreateCollection',
  collectionsCreateItem: 'CollectionsService.CreateItem',
  collectionsRename: 'CollectionsService.Rename',
  collectionsDeleteMany: 'CollectionsService.DeleteMany',
  collectionsMoveItems: 'CollectionsService.MoveItems',
  collectionsReorderCollections: 'CollectionsService.ReorderCollections',
  collectionsImport: 'CollectionsService.Import',
  collectionsExport: 'CollectionsService.Export',
  apiConfigView: 'ApiConfigService.View',
  apiConfigSetSecret: 'ApiConfigService.SetSecret',
  apiConfigDeleteSecret: 'ApiConfigService.DeleteSecret',
  apiConfigFile: 'ApiConfigService.File',
  apiConfigSaveFile: 'ApiConfigService.SaveFile',
  apiConfigForm: 'ApiConfigService.Form',
  apiConfigAgentAccess: 'ApiConfigService.AgentAccess',
  connectionConfigView: 'ConnectionConfigService.View',
  connectionConfigFile: 'ConnectionConfigService.File',
  connectionConfigSaveFile: 'ConnectionConfigService.SaveFile',
  connectionConfigForm: 'ConnectionConfigService.Form',
  connectionConfigDeleteFile: 'ConnectionConfigService.DeleteFile',
  connectionConfigSetSecret: 'ConnectionConfigService.SetSecret',
  connectionConfigDeleteSecret: 'ConnectionConfigService.DeleteSecret',
  variablesListEnvironments: 'VariablesService.ListEnvironments',
  variablesCreateEnvironment: 'VariablesService.CreateEnvironment',
  variablesDeleteEnvironment: 'VariablesService.DeleteEnvironment',
  variablesDuplicateEnvironment: 'VariablesService.DuplicateEnvironment',
  variablesReorderEnvironments: 'VariablesService.ReorderEnvironments',
  variablesSetActiveEnvironment: 'VariablesService.SetActiveEnvironment',
  variablesList: 'VariablesService.List',
  variablesHistory: 'VariablesService.History',
  variablesReveal: 'VariablesService.Reveal',
  variablesRevealHistory: 'VariablesService.RevealHistory',
  historyList: 'ResponseHistoryService.List',
  historyGet: 'ResponseHistoryService.Get',
  historyDelete: 'ResponseHistoryService.Delete',
  historyClear: 'ResponseHistoryService.Clear',
  historyAdopt: 'ResponseHistoryService.Adopt',
  grpcDescribe: 'GrpcService.Describe',
  grpcCall: 'GrpcService.Call',
  grpcHistoryList: 'GrpcHistoryService.List',
  grpcHistoryGet: 'GrpcHistoryService.Get',
  grpcHistoryDelete: 'GrpcHistoryService.Delete',
  grpcHistoryClear: 'GrpcHistoryService.Clear',
  grpcHistoryAdopt: 'GrpcHistoryService.Adopt',
  collectionsGetGrpcRequest: 'CollectionsService.GetGrpcRequest',
  collectionsSaveGrpcRequest: 'CollectionsService.SaveGrpcRequest',
  collectionsCreateGrpcItem: 'CollectionsService.CreateGrpcItem',
  dbMcpStatus: 'DbMcpService.Status',
  dbMcpSetEnabled: 'DbMcpService.SetEnabled',
  dbMcpRegenerate: 'DbMcpService.Regenerate',
  dbMcpInstallAgent: 'DbMcpService.InstallAgent',
  dbMcpPendingResponse: 'DbMcpService.PendingResponse',
  dbMcpTables: 'DbMcpService.Tables',
  maskReviewColumns: 'MaskRulesService.ReviewColumns',
  maskReviewPending: 'MaskRulesService.ReviewPending',
  proofSessions: 'ProofService.Sessions',
  proofSession: 'ProofService.Session',
  proofStep: 'ProofService.Step',
  proofViewState: 'ProofService.ViewState',
  proofSetViewState: 'ProofService.SetViewState',
  proofAcceptEdit: 'ProofService.AcceptEdit',
  proofRejectEdit: 'ProofService.RejectEdit',
  proofDelete: 'ProofService.Delete',
  proofMarkSeen: 'ProofService.MarkSeen',
  proofOpenWindow: 'ProofService.OpenWindow',
  proofWindowTarget: 'ProofService.WindowTarget',
  proofSavePng: 'ProofService.SavePng',
  proofChooseShotFolder: 'ProofService.ChooseShotFolder',
  proofSavePngTo: 'ProofService.SavePngTo',
  proofForgetShotFolder: 'ProofService.ForgetShotFolder',
  kubernetesClusters: 'KubernetesService.Clusters',
  kubernetesDescribe: 'KubernetesService.Describe',
  kubernetesKinds: 'KubernetesService.Kinds',
  kubernetesList: 'KubernetesService.List',
  kubernetesLogsClose: 'KubernetesService.LogsClose',
  kubernetesLogsOpen: 'KubernetesService.LogsOpen',
  kubernetesNamespaces: 'KubernetesService.Namespaces',
  kubernetesReorder: 'KubernetesService.Reorder',
  kubernetesSetMcp: 'KubernetesService.SetMcp',
  kubernetesStatus: 'KubernetesService.Status',
  dockerContainerEditSpec: 'DockerService.ContainerEditSpec',
  dockerContainerSize: 'DockerService.ContainerSize',
  dockerContainers: 'DockerService.Containers',
  dockerContexts: 'DockerService.Contexts',
  dockerDiskUsage: 'DockerService.DiskUsage',
  dockerExecClose: 'DockerService.ExecClose',
  dockerExecOpen: 'DockerService.ExecOpen',
  dockerExecResize: 'DockerService.ExecResize',
  dockerExecWrite: 'DockerService.ExecWrite',
  dockerConfirmImageRegistry: 'DockerService.ConfirmImageRegistry',
  dockerImageRegistry: 'DockerService.ImageRegistry',
  dockerImages: 'DockerService.Images',
  dockerInspect: 'DockerService.Inspect',
  dockerInspectContainer: 'DockerService.InspectContainer',
  dockerLogsClose: 'DockerService.LogsClose',
  dockerLogsOpen: 'DockerService.LogsOpen',
  dockerNetworks: 'DockerService.Networks',
  dockerRecreateContainer: 'DockerService.RecreateContainer',
  dockerRestart: 'DockerService.Restart',
  dockerRestoreContainers: 'DockerService.RestoreContainers',
  dockerRestoreDismiss: 'DockerService.RestoreDismiss',
  dockerRestoreState: 'DockerService.RestoreState',
  dockerStart: 'DockerService.Start',
  dockerStatsSubscribe: 'DockerService.StatsSubscribe',
  dockerStatsUnsubscribe: 'DockerService.StatsUnsubscribe',
  dockerStatus: 'DockerService.Status',
  dockerStop: 'DockerService.Stop',
  dockerUnwatch: 'DockerService.Unwatch',
  dockerUpdateContainer: 'DockerService.UpdateContainer',
  dockerUseContext: 'DockerService.UseContext',
  dockerVolumes: 'DockerService.Volumes',
  dockerWatch: 'DockerService.Watch',
};

export const { channelToFqn: CHANNEL_TO_FQN, fqnToChannel: FQN_TO_CHANNEL } = buildChannelMaps(
  IPC,
  FQN_SUFFIX_BY_IPC_KEY,
  BRIDGE_PKG,
);

const EMPTY_OBJECT_META = {
  path: '',
  kind: 'table',
  name: '',
  qualifiedName: '',
  columns: [],
  primaryKey: null,
  foreignKeys: [],
  referencedBy: [],
  indexes: [],
  rowEstimate: null,
  comment: null,
};

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
  // Boot resets the window's git connection (repo/git/boundGit.ts).
  [IPC.gitResetWindow]: 'null',
  [IPC.tabsSave]: 'null',
  // Boot drains Finder-opened files; none by default.
  [IPC.openedFilesIsFilesWindow]: 'false',
  [IPC.openedFilesPending]: '[]',
  [IPC.openedFilesTerminalResolve]: '{"openable":[]}',
  [IPC.openedFilesTerminalOpen]: 'null',
  [IPC.windowsEnsure]: JSON.stringify({ mode: 'git' }),
  [IPC.windowsSetMode]: 'null',
  // P520: nothing pending unless a spec pushes the queue (pushApprovals).
  [IPC.approvalsList]: '[]',
  // P246: the prompt router's calls; a spec drives the list through IPC.promptsChanged pushes.
  [IPC.promptsList]: '[]',
  [IPC.promptsClaim]: 'null',
  [IPC.promptsRaise]: 'null',
  [IPC.promptsDismiss]: 'null',
  [IPC.promptsSendTest]: 'null',
  // The active agent provider's names: the claude provider, as the Go flow test writes it.
  [IPC.agentInfo]: JSON.stringify(contract('agent', 'AgentService.Info')),
  [IPC.agentInfoFor]: JSON.stringify(contract('agent', 'AgentService.Info')),
  // Only claude is installed unless a spec says otherwise.
  [IPC.agentList]: JSON.stringify(contract('agent', 'AgentService.List#no-opencode')),
  // The prompt editor's calls; a spec answers List/Save from tests/contract/llm-prompts.json.
  [IPC.llmPromptsList]: '{"prompts":[],"storeError":"","revision":0}',
  [IPC.llmPromptsSave]: '{"prompts":[],"storeError":"","revision":0}',
  [IPC.llmPromptsPreview]: '{"errors":[],"parts":[]}',
  [IPC.layoutSet]: JSON.stringify(defaultLayout),
  [IPC.settingsSet]: JSON.stringify(defaultSettings),
  // P132 Part 2: main.ts's bootstrap() hydrates the op log on every boot.
  [IPC.opsRecent]: '[]',
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
  [IPC.adeTaskReviewWorkingStamp]: JSON.stringify({ stamp: '' }),
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
  // Databases module (formerly Kira Studio's own table).
  [IPC.filtersList]: JSON.stringify({ hiddenKinds: [], hiddenPaths: [] }),
  // The connection files' status: no file has a problem unless a spec says so.
  [IPC.connectionConfigView]: JSON.stringify({
    dir: '',
    files: [],
    secrets: [],
    storage: { available: true, backend: 'keychain', insecureFallback: false, reason: null },
  }),
  [IPC.connectionConfigFile]: JSON.stringify({
    file: '',
    id: '',
    path: '',
    text: '',
    hash: '',
    problem: null,
    lastGood: false,
    editable: true,
    secrets: [],
  }),
  // The connection file dialog applies its draft text through the form call; an empty file has no
  // model and no problem unless a spec says so.
  [IPC.connectionConfigForm]: JSON.stringify({ text: '', value: null, problem: null }),
  // Every SQL console loads its connection's own DDL document on mount
  // (state/schemas.ts's ensureDdl) — a spec with no Schema (DDL)… of its own gets the same
  // "absent until the user writes one" empty document a fresh connection gets, not a fixture
  // miss. connectionId is echoed as '' here since the frontend only reads `.ddl` off this call.
  [IPC.schemaGet]: JSON.stringify({ connectionId: '', ddl: '', updatedAt: '' }),
  [IPC.linkOpenExternal]: 'null',
  // P420: the proof window's calls, answered from tests/contract/proof.json.
  [IPC.proofSessions]: JSON.stringify(contract('proof', 'ProofService.Sessions')),
  [IPC.proofSession]: JSON.stringify(contract('proof', 'ProofService.Session')),
  [IPC.proofStep]: JSON.stringify(contract('proof', 'ProofService.Step#table')),
  [IPC.proofViewState]: '[]',
  [IPC.proofSetViewState]: 'null',
  [IPC.proofAcceptEdit]: 'null',
  [IPC.proofRejectEdit]: 'null',
  [IPC.proofDelete]: 'null',
  [IPC.proofMarkSeen]: 'null',
  [IPC.proofOpenWindow]: '{"key":"proof-1","opened":true}',
  [IPC.proofWindowTarget]: '{"sessionId":null}',
  [IPC.proofSavePng]: '{"saved":false,"path":null}',
  [IPC.proofChooseShotFolder]: '{"canceled":true,"id":"","path":""}',
  [IPC.proofSavePngTo]: '{"path":""}',
  [IPC.proofForgetShotFolder]: 'null',
  // Boot queries of the code-repo stores, issued in every mode.
  [IPC.adeTaskRepos]: '{"repos":[],"folders":[]}',
  [IPC.adeTaskSessions]: '{"sessions":[]}',
  [IPC.repoConfigList]: '{"dir":"","files":[]}',
  [IPC.queriesHistoryRecord]: 'null',
  [IPC.treeInvalidate]: 'null',
  [IPC.treeDescribe]: JSON.stringify({ meta: EMPTY_OBJECT_META, source: 'server' }),
  // SlickGridHost.vue's own onMounted loads a tab's connection's mask rules
  // unconditionally (so folding is ready the instant the preview is toggled on) — a spec with no
  // maskRulesList snapshot of its own gets "no rules configured", the correct default for every
  // connection this chapter's own fixtures predate, not a fixture miss.
  [IPC.maskRulesList]: '[]',
  [IPC.maskRulesCounts]: '{}',
  [IPC.maskReviewColumns]: '{"applicable":false,"columns":[],"checkedAt":0}',
  [IPC.maskReviewPending]: '{"count":0}',
  [IPC.maskRulesCorrelationKey]: '""',
  // Every SQL console/data-tab fetches its container's cached columns on
  // mount/activation (state/schemaColumns.ts's ensureSchemaColumns) — a spec with no
  // treeSchemaColumns snapshot of its own gets "nothing cached for this container yet", the same
  // reasoning treeDescribe's own wildcard above already carries, not a fixture miss.
  [IPC.treeSchemaColumns]: JSON.stringify({ relations: [], source: 'server' }),
  // BrowseView.vue's own ensureKeyTypes fires on every VirtualList visible-range tick for a
  // redis browse tab — `paths` is whatever happens to be on screen at whatever moment a headless
  // run's own scroll/render timing lands on, so unlike treeChildren there is no fixed args shape
  // any fixture could capture even in principle. ensureKeyTypes itself already treats a failed
  // batch as silent (a decorative badge must never raise the error strip a failed *listing*
  // owns) — this answers "nothing yet" instead of a network-level miss for the same call.
  [IPC.treeKeyTypes]: '[]',
  // Http's left panel fetches its whole tree on mount, and the call is NOT wrapped in a
  // try/catch — a fixture miss would leave the panel permanently empty rather than degrade. Every
  // spec that switches to Http mode without seeding a collections fixture (mode-switch.spec.ts
  // boots with none and asserts the panel still says "Collections") gets "no collections yet",
  // the same reasoning `queriesList: '[]'` already carries.
  [IPC.collectionsList]: JSON.stringify({ collections: [], items: [] }),
  // P5 F9: the environment switcher and the variables dialog both fetch on mount, uncaught, the
  // same trap collectionsList already needed a wildcard for — mode-switch.spec.ts,
  // http-request.spec.ts and http-request-body.spec.ts all boot with no variables fixture at all.
  [IPC.variablesListEnvironments]: '[]',
  [IPC.variablesList]: '[]',
  [IPC.queriesList]: '[]',
  [IPC.queriesListConsole]: '[]',
  [IPC.queriesHistoryList]: '[]',
  // mode-switch.spec.ts and both existing http-request*.spec.ts specs boot with no history fixture
  // at all — ResponsePane.vue's History segment fetches on mount, so "no history yet for a request
  // never seen before" is the right default rather than a fixture miss.
  [IPC.historyList]: '[]',
  // M1 §6.2: hydrateDbMcp() joins the same unconditional-every-boot Promise.all, same
  // reasoning — nothing in tests/ui/ seeds a dbmcp fixture, so
  // "off, nothing running" is the honest default for a dev-server run under Playwright.
  [IPC.dbMcpStatus]: JSON.stringify({
    running: false,
    command: '',
    agentAvailable: false,
    probed: [],
    expiresAt: '',
    error: '',
  }),
  // P85: main.ts's bootstrap() joins hydrateCustomScripts() to the same unconditional-every-boot
  // Promise.all as updateStatus above, same reasoning — a spec that never configures a
  // script gets "no scripts yet", not a fixture miss.
  [IPC.customScriptsList]:
    '{"dir":"/home/test/runs","collections":[],"scripts":[],"problems":[],"secretStorage":{"available":true,"backend":"keychain","insecureFallback":false,"reason":null}}',
});

// `windowKey`/`tabId` are excluded outright — a per-window or per-tab id this app generates at
// runtime, never reproducible from a fixture.
// `refresh` defaults to `false` at every call site, so a live call always carries it explicitly
// while an older captured fixture may omit it; dropping an explicit `false` makes them compare equal.
const CANONICAL_OPTIONS = {
  excludeKeys: ['windowKey', 'tabId', 'terminalId'],
  dropKey: (key: string, value: unknown) => key === 'refresh' && value === false,
};

// The mode a fresh boot answers windowsEnsure with when a spec gives no snapshot for it: the
// active tab of its (single, args-less) tabsList snapshot decides, so a spec restoring a Studio
// tab as active boots displaying it. Anything else falls to the wildcard default.
function inferredBootMode(byChannel: ReadonlyMap<string, MockSnapshot[]>): string | undefined {
  const tabsSnaps = byChannel.get(IPC.tabsList);
  const tabs = (tabsSnaps?.length === 1 ? tabsSnaps[0].response : undefined) as
    | { kind?: string; active?: boolean }[]
    | undefined;
  if (!tabs || tabs.length === 0) return undefined;
  const kind = (tabs.find((t) => t.active) ?? tabs[0])?.kind;
  return kind && kind in STUDIO_TAB_KIND_MODE
    ? STUDIO_TAB_KIND_MODE[kind as StudioTabKind]
    : undefined;
}

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
    // `refresh:true` never appears in a captured fixture (capture always reads `refresh:false`
    // first); a static fixture world answers both the same way.
    resolveSnapshotFallback: (_channel, args, findSnap) => {
      if (args && typeof args === 'object' && (args as { refresh?: unknown }).refresh === true) {
        return findSnap({ ...args, refresh: false });
      }
      return undefined;
    },
    // opsCancel (never snapshotted) releases the stream replies held by `untilCancel`
    // snapshots; a spec asserts the cancelled op id from `control.log()`.
    resolveMissingBody: (channel, byChannel) => {
      if (channel === IPC.opsCancel) {
        void page
          .evaluate(() =>
            (
              globalThis as unknown as { __kiraReleaseCancelled?: () => void }
            ).__kiraReleaseCancelled?.(),
          )
          .catch(() => {});
        return 'null';
      }
      if (channel === IPC.windowsEnsure) {
        const mode = inferredBootMode(byChannel);
        return mode ? JSON.stringify({ mode }) : undefined;
      }
      return undefined;
    },
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
  [IPC.adeTaskAddNote]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskUpdateHandover]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskDismissHandover]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskAddLink]: [IPC.adeTaskBoardChanged],
  [IPC.adeTaskEditLink]: [IPC.adeTaskBoardChanged],
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
  [IPC.adeTaskSaveStep]: [IPC.adeTaskWorkflowsChanged],
  [IPC.adeTaskSaveStepYaml]: [IPC.adeTaskWorkflowsChanged],
  [IPC.adeTaskNewStep]: [IPC.adeTaskWorkflowsChanged],
  [IPC.adeTaskDeleteStep]: [IPC.adeTaskWorkflowsChanged],
  [IPC.repoConfigSaveFile]: [IPC.repoConfigChanged, IPC.adeTaskReposChanged],
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
