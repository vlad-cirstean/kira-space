import * as AdeTaskService from '@bindings/adetaskservice.js';
import * as AgentNotifyService from '@bindings/agentnotifyservice.js';
import * as AgentService from '@bindings/agentservice.js';
import * as AgentUsageService from '@bindings/agentusageservice.js';
import * as CaptureService from '@bindings/captureservice.js';
import * as CodeWorkspaceService from '@bindings/codeworkspaceservice.js';
import * as CustomScriptsService from '@bindings/customscriptsservice.js';
import * as FileHandlersService from '@bindings/filehandlersservice.js';
import * as FilesService from '@bindings/filesservice.js';
import * as GitCredentialService from '@bindings/gitcredentialservice.js';
import * as GitHostService from '@bindings/githostservice.js';
import * as HostViewService from '@bindings/hostviewservice.js';
import * as IssueTrackerService from '@bindings/issuetrackerservice.js';
import * as KeepAwakeService from '@bindings/keepawakeservice.js';
import * as LayoutService from '@bindings/layoutservice.js';
import * as LifecycleService from '@bindings/lifecycleservice.js';
import * as LinkService from '@bindings/linkservice.js';
import * as LlmPromptsService from '@bindings/llmpromptsservice.js';
import * as MobileAccessService from '@bindings/mobileaccessservice.js';
import * as OpenedFilesService from '@bindings/openedfilesservice.js';
import * as OpsService from '@bindings/opsservice.js';
import * as PromptsService from '@bindings/promptsservice.js';
import * as ScriptRunsService from '@bindings/scriptrunsservice.js';
import * as SettingsService from '@bindings/settingsservice.js';
import * as TabsService from '@bindings/tabsservice.js';
import * as TerminalService from '@bindings/terminalservice.js';
import * as UpdateService from '@bindings/updateservice.js';
import * as WindowsService from '@bindings/windowsservice.js';
import type { HeadState, ReviewComment } from '@kira/git-ipc';
import type { AgentEvent, AgentSessionsEvent } from '@shared/domain/agent';
import type { PaletteColor } from '@shared/domain/color';
import type { GitCredentialPrompt } from '@shared/domain/git';
import type { Layout } from '@shared/domain/layout';
import type {
  MobileDevice,
  MobilePairingActionResult,
  MobilePairingSnapshot,
  MobileStatus,
  MobileTerminalHold,
} from '@shared/domain/mobile';
import type {
  CodeSearchEvent,
  DiffContent,
  DiffRead,
  EditContent,
  FileContent,
  FileHandlersSetResult,
  FileHandlersStatus,
  FileListing,
  FileRead,
  FileStat,
  OpenedFileTarget,
  RepoSummary,
  SearchRequest,
  WriteFileResult,
} from '@shared/domain/repo';
import type {
  ScriptRun,
  ScriptRunArgs,
  ScriptRunLogChunk,
  ScriptRunLogPage,
  ScriptRunPreview,
  ScriptRunRestoreChoice,
  ScriptRunRestoreResult,
  ScriptRunRestoreState,
  ScriptRunStarted,
} from '@shared/domain/scriptRuns';
import type {
  CustomScript,
  CustomScriptFields,
  ScriptCollection,
  ScriptDir,
  ScriptMcpServer,
  ScriptsSnapshot,
  SecretValues,
} from '@shared/domain/scripts';
import { CHANNEL } from '@shared/protocol/events';
import { scriptsSnapshotOf } from '@workbench/automations/createCustomScriptsStore';
import { createCoreControl } from '@workbench/bridge/createCoreControl';
import { createOpenExternal } from '@workbench/bridge/openExternal';
import { on, trust, unwrap, windowKey } from '@workbench/bridge/rpc';
import type * as V2 from '../ade/v2/wire';
import type { CaptureItem, CaptureResult, HotkeyState } from '../capture/wire';
import { CHANNEL_CAPTURE_ITEM, CHANNEL_CAPTURE_STATUS } from '../capture/wire';
import type {
  HostViewChangeRequest,
  HostViewIssue,
  HostViewOpen,
  HostViewPosted,
  HostViewTarget,
} from '../hostview/wire';
import type { CliToolsResult, IssueTrackers } from '../state/cliToolsDomain';
import type { SpaceMode } from '../state/modeDomain';
import type { SpaceOpRecord } from '../state/opsDomain';
import type { Settings, SettingsPatch } from '../state/settingsDomain';
import type { TabRecord } from '../state/tabDomain';
import { memoryControl } from './memoryControl';

// bridge/index.ts is this app's own composition root — Kira Studio's own bridge/index.ts, trimmed
// to the 15 services apps/kira-space/main.go binds (LinkService added alongside this file —
// repo/git/hostHandlers.ts's link.openExternal handler had no Go counterpart here; see
// internal/bridge/link.go). No apiControl.ts equivalent: this app has exactly one bound-call
// surface, not two composed halves.
// A growing subset of that surface's own methods — the ones byte-for-byte identical with Kira
// Studio's own studioControl — now come from createCoreControl.ts, shared with Kira Studio's own
// copy of this file; this app's own remaining methods are defined right here.
// Ten more (open-settings/toggle-project-panel/tab-next/prev/close, keep-awake, app-metrics,
// windowsOpenNew) moved into that same shared factory once this app grew its own window-chrome
// parity — this app now has its own metrics ticker (main.go's own metrics.NewAppTicker) and
// keep-awake controller, so both are wired the same way Kira Studio's own copy of this file is.
const spaceControl = {
  customScriptsList: (): Promise<ScriptsSnapshot> =>
    unwrap(CustomScriptsService.List()).then((r) => scriptsSnapshotOf(r)),
  customScriptsCreate: (
    fields: CustomScriptFields,
    secretValues: SecretValues | null,
  ): Promise<CustomScript> =>
    unwrap(CustomScriptsService.Create({ fields, secretValues })).then((r) =>
      trust<CustomScript>(r),
    ),
  customScriptsUpdate: (
    id: string,
    fields: CustomScriptFields,
    secretValues: SecretValues | null,
  ): Promise<CustomScript> =>
    unwrap(CustomScriptsService.Update({ id, fields, secretValues })).then((r) =>
      trust<CustomScript>(r),
    ),
  customScriptsDelete: (id: string): Promise<void> => unwrap(CustomScriptsService.Delete({ id })),
  customScriptsCreateCollection: (name: string): Promise<ScriptCollection> =>
    unwrap(CustomScriptsService.CreateCollection({ name })).then((r) => trust<ScriptCollection>(r)),
  customScriptsRenameCollection: (id: string, name: string): Promise<void> =>
    unwrap(CustomScriptsService.RenameCollection({ id, name })),
  customScriptsDeleteCollection: (id: string): Promise<void> =>
    unwrap(CustomScriptsService.DeleteCollection({ id })),
  customScriptsMove: (
    id: string,
    collectionId: string | null,
    beforeId: string | null = null,
  ): Promise<void> => unwrap(CustomScriptsService.Move({ id, collectionId, beforeId })),
  customScriptsReorder: (collectionId: string | null, ids: string[]): Promise<void> =>
    unwrap(CustomScriptsService.Reorder({ collectionId, ids })),
  customScriptsReorderCollections: (ids: string[]): Promise<void> =>
    unwrap(CustomScriptsService.ReorderCollections({ ids })),
  onCustomScriptsChanged: (cb: (snapshot: ScriptsSnapshot) => void): (() => void) =>
    on(CHANNEL.customScriptsChanged, (r) => cb(scriptsSnapshotOf(r))),

  scriptRunsList: (limit?: number, taskId?: string): Promise<ScriptRun[]> =>
    unwrap(ScriptRunsService.List({ limit: limit ?? 0, taskId: taskId ?? '' })).then((r) =>
      trust<ScriptRun[]>(r ?? []),
    ),
  scriptRunsStop: (id: string): Promise<void> => unwrap(ScriptRunsService.Stop({ id })),
  scriptRunsResolveDir: (scriptId: string): Promise<ScriptDir> =>
    unwrap(ScriptRunsService.ResolveDir({ scriptId })).then((r) => trust<ScriptDir>(r)),
  onScriptRunsChanged: (cb: (run: ScriptRun) => void): (() => void) =>
    on(CHANNEL.scriptRunsChanged, (r) => cb(trust<ScriptRun>(r))),
  scriptRunsPreview: (args: ScriptRunArgs): Promise<ScriptRunPreview> =>
    unwrap(ScriptRunsService.Preview({ ...args, listTasks: args.listTasks ?? false })).then((r) =>
      trust<ScriptRunPreview>(r),
    ),
  scriptRunsStart: (args: ScriptRunArgs, hash: string): Promise<ScriptRunStarted> =>
    unwrap(ScriptRunsService.Start({ ...args, listTasks: args.listTasks ?? false, hash })).then(
      (r) => trust<ScriptRunStarted>(r),
    ),
  scriptRunsResume: (id: string): Promise<ScriptRunStarted> =>
    unwrap(ScriptRunsService.Resume({ id })).then((r) => trust<ScriptRunStarted>(r)),
  scriptRunsRestoreState: (): Promise<ScriptRunRestoreState> =>
    unwrap(ScriptRunsService.RestoreState()).then((r) => trust<ScriptRunRestoreState>(r)),
  scriptRunsRestore: (items: ScriptRunRestoreChoice[]): Promise<ScriptRunRestoreResult> =>
    unwrap(ScriptRunsService.Restore({ items })).then((r) => trust<ScriptRunRestoreResult>(r)),
  scriptRunsRestoreDismiss: (): Promise<void> => unwrap(ScriptRunsService.RestoreDismiss()),
  scriptRunsResumeCommand: (runId: string): Promise<string> =>
    unwrap(ScriptRunsService.ResumeCommand({ runId })),
  scriptRunsReadLog: (id: string, afterSeq: number): Promise<ScriptRunLogPage> =>
    unwrap(ScriptRunsService.ReadLog({ id, afterSeq })).then((r) => {
      const page = trust<Partial<ScriptRunLogPage>>(r);
      return { chunks: page.chunks ?? [], truncated: page.truncated ?? false };
    }),
  onScriptRunLog: (
    cb: (push: { runId: string; chunks: ScriptRunLogChunk[] }) => void,
  ): (() => void) =>
    on(CHANNEL.scriptRunLog, (r) => cb(trust<{ runId: string; chunks: ScriptRunLogChunk[] }>(r))),
  scriptRunsNextFires: (cron: string, timezone: string, count?: number): Promise<number[]> =>
    unwrap(ScriptRunsService.NextFires({ cron, timezone, count: count ?? 0 })).then((r) =>
      trust<number[]>(r ?? []),
    ),
  scriptRunsSchedulePreview: (
    scriptId: string,
    secrets: Record<string, string[]>,
  ): Promise<ScriptRunPreview> =>
    unwrap(ScriptRunsService.SchedulePreview({ scriptId, secrets })).then((r) =>
      trust<ScriptRunPreview>(r),
    ),
  scriptRunsRunScheduleNow: (
    scriptId: string,
    hash: string,
    secrets: Record<string, string[]>,
  ): Promise<ScriptRunStarted> =>
    unwrap(ScriptRunsService.RunScheduleNow({ scriptId, hash, secrets })).then((r) =>
      trust<ScriptRunStarted>(r),
    ),
  scriptRunsConfirmAccept: (
    runId: string,
    hash: string,
    secrets: Record<string, string[]>,
  ): Promise<ScriptRunStarted> =>
    unwrap(ScriptRunsService.ConfirmAccept({ runId, hash, secrets })).then((r) =>
      trust<ScriptRunStarted>(r),
    ),
  scriptRunsConfirmDecline: (runId: string): Promise<void> =>
    unwrap(ScriptRunsService.ConfirmDecline({ id: runId })),
  scriptRunsMcpServers: (): Promise<ScriptMcpServer[]> =>
    unwrap(ScriptRunsService.McpServers()).then((r) => trust<ScriptMcpServer[]>(r ?? [])),

  // P120: openExternal (LinkService) moved off createCoreControl.ts's now-deleted shared `link` binding —
  // Kira Studio never had a real use for it, so this app's own LinkService.OpenExternal call
  // stays here instead.
  openExternal: createOpenExternal((url) => unwrap(LinkService.OpenExternal({ url }))),
  onCommandPalette: (cb: () => void): (() => void) => on(CHANNEL.commandPalette, cb),
  onViewFind: (cb: () => void): (() => void) => on(CHANNEL.viewFind, cb),
  onReviewCode: (cb: () => void): (() => void) => on(CHANNEL.reviewCode, cb),

  gitHostOpenChangeRequestUrl: (url: string): Promise<void> =>
    unwrap(GitHostService.OpenChangeRequestURL({ url })),
  captureWindowItem: (args: { windowKey: string }): Promise<CaptureItem | null> =>
    unwrap(CaptureService.WindowItem(args)).then((r) => trust<CaptureItem | null>(r)),
  captureConfirm: (args: {
    windowKey: string;
    itemId: string;
    action: string;
    title: string;
  }): Promise<CaptureResult> =>
    unwrap(CaptureService.Confirm(args)).then((r) => trust<CaptureResult>(r)),
  captureDismiss: (windowKey: string): Promise<void> =>
    unwrap(CaptureService.Dismiss({ windowKey })),
  captureStatus: (): Promise<HotkeyState> =>
    unwrap(CaptureService.Status()).then((r) => trust<HotkeyState>(r)),
  onCaptureItem: (cb: (item: CaptureItem) => void): (() => void) => on(CHANNEL_CAPTURE_ITEM, cb),
  onCaptureStatus: (cb: (state: HotkeyState) => void): (() => void) =>
    on(CHANNEL_CAPTURE_STATUS, cb),
  hostViewOpen: (url: string): Promise<HostViewOpen> =>
    unwrap(HostViewService.Open({ url })).then((r) => trust<HostViewOpen>(r)),
  hostViewWindowTarget: (args: { windowKey: string }): Promise<HostViewTarget | null> =>
    unwrap(HostViewService.WindowTarget(args)).then((r) => trust<HostViewTarget | null>(r)),
  hostViewChangeRequest: (args: { url: string }): Promise<HostViewChangeRequest> =>
    unwrap(HostViewService.ChangeRequest(args)).then((r) => trust<HostViewChangeRequest>(r)),
  hostViewIssue: (args: { ref: string }): Promise<HostViewIssue> =>
    unwrap(HostViewService.Issue(args)).then((r) => trust<HostViewIssue>(r)),
  hostViewPostComment: (args: {
    windowKey: string;
    body: string;
    replyTo: string;
  }): Promise<HostViewPosted> =>
    unwrap(HostViewService.PostComment(args)).then((r) => trust<HostViewPosted>(r)),
  gitHostProviders: (): Promise<V2.HostProviders> =>
    unwrap(GitHostService.Providers()).then((r) => trust<V2.HostProviders>(r)),

  gitCredentialPending: (): Promise<GitCredentialPrompt[]> =>
    unwrap(GitCredentialService.Pending()).then((r) => trust<GitCredentialPrompt[]>(r ?? [])),
  gitCredentialProvide: (requestId: string, secret: string | null): Promise<boolean> =>
    unwrap(GitCredentialService.Provide({ requestId, secret })),
  onGitCredentialChanged: (cb: (prompts: GitCredentialPrompt[]) => void): (() => void) =>
    on(CHANNEL.gitCredential, cb),
  // P212: the Mobile access pane and the phone pairing prompt.
  mobileStatus: (): Promise<MobileStatus> =>
    unwrap(MobileAccessService.Status()).then((r) => trust<MobileStatus>(r)),
  mobileSetEnabled: (enabled: boolean): Promise<MobileStatus> =>
    unwrap(MobileAccessService.SetEnabled({ enabled })).then((r) => trust<MobileStatus>(r)),
  mobileSetPort: (port: number): Promise<MobileStatus> =>
    unwrap(MobileAccessService.SetPort({ port })).then((r) => trust<MobileStatus>(r)),
  mobileTrustNetwork: (): Promise<MobileStatus> =>
    unwrap(MobileAccessService.TrustCurrentNetwork()).then((r) => trust<MobileStatus>(r)),
  mobileForgetNetwork: (): Promise<MobileStatus> =>
    unwrap(MobileAccessService.ForgetNetwork()).then((r) => trust<MobileStatus>(r)),
  mobileSetAgentInput: (enabled: boolean): Promise<MobileStatus> =>
    unwrap(MobileAccessService.SetAgentInputEnabled({ enabled })).then((r) =>
      trust<MobileStatus>(r),
    ),
  mobileSetDevicePermissions: (id: string, write: boolean, agentInput: boolean): Promise<void> =>
    unwrap(MobileAccessService.SetDevicePermissions({ id, write, agentInput })),
  mobileDevices: (): Promise<MobileDevice[]> =>
    unwrap(MobileAccessService.Devices()).then((r) => trust<MobileDevice[]>(r ?? [])),
  mobileRevoke: (id: string): Promise<void> => unwrap(MobileAccessService.Revoke({ id })),
  mobilePairingPending: (): Promise<MobilePairingSnapshot> =>
    unwrap(MobileAccessService.PendingPairing()).then((r) => trust<MobilePairingSnapshot>(r)),
  mobilePairingApprove: (id: string): Promise<MobilePairingActionResult> =>
    unwrap(MobileAccessService.Approve({ id })).then((r) => trust<MobilePairingActionResult>(r)),
  mobilePairingDeny: (id: string): Promise<MobilePairingActionResult> =>
    unwrap(MobileAccessService.Deny({ id })).then((r) => trust<MobilePairingActionResult>(r)),
  mobileLaunchOpened: (terminalId: string, error: string): Promise<void> =>
    unwrap(MobileAccessService.LaunchOpened({ terminalId, error })),
  mobileTerminalHolds: (): Promise<MobileTerminalHold[]> =>
    unwrap(MobileAccessService.TerminalHolds()).then((r) => trust<MobileTerminalHold[]>(r ?? [])),
  mobileReclaimTerminal: (terminalId: string, cols: number, rows: number): Promise<void> =>
    unwrap(MobileAccessService.ReclaimTerminal({ terminalId, cols, rows })),
  onMobileTerminals: (cb: (holds: MobileTerminalHold[]) => void): (() => void) =>
    on(CHANNEL.mobileTerminals, (holds: MobileTerminalHold[] | null) => cb(holds ?? [])),
  onMobileOpenLaunch: (cb: (event: V2.MobileOpenLaunchEvent) => void): (() => void) =>
    on(CHANNEL.mobileOpenLaunch, cb),
  onMobileStatusChanged: (cb: (status: MobileStatus) => void): (() => void) =>
    on(CHANNEL.mobileStatus, cb),
  onMobileDevicesChanged: (cb: (devices: MobileDevice[]) => void): (() => void) =>
    on(CHANNEL.mobileDevices, cb),
  onMobilePairingChanged: (cb: (snap: MobilePairingSnapshot) => void): (() => void) =>
    on(CHANNEL.mobilePairing, cb),

  // C5 §3.3: the native code-viewing workspace's bound surface — repo import/rename/reorder/remove plus
  // the two read primitives (ListFiles/ReadFile). A rejected ImportRepo/RenameRepo/ReorderRepos call carries a
  // structured ipcerr that unwrap() already turns into a rejected promise.
  codeWorkspaceListRepos: (): Promise<RepoSummary[]> =>
    unwrap(CodeWorkspaceService.ListRepos()).then((r) => trust<RepoSummary[]>(r ?? [])),
  codeWorkspaceRepoHeads: (
    ids?: string[],
  ): Promise<Array<{ id: string; head: HeadState | null; error?: string }>> =>
    unwrap(CodeWorkspaceService.RepoHeads({ ids: ids ?? null })).then((r) =>
      trust<Array<{ id: string; head: HeadState | null; error?: string }>>(r ?? []),
    ),
  codeWorkspaceRepoWorktreeLinks: (): Promise<
    Array<{ id: string; parentId: string; error?: string }>
  > =>
    unwrap(CodeWorkspaceService.RepoWorktreeLinks()).then((r) =>
      trust<Array<{ id: string; parentId: string; error?: string }>>(r ?? []),
    ),
  codeWorkspaceImportRepo: (path: string): Promise<RepoSummary> =>
    unwrap(CodeWorkspaceService.ImportRepo({ path })).then((r) => trust<RepoSummary>(r)),
  codeWorkspaceRenameRepo: (id: string, name: string): Promise<RepoSummary> =>
    unwrap(CodeWorkspaceService.RenameRepo({ id, name })).then((r) => trust<RepoSummary>(r)),
  codeWorkspaceSetRepoHidden: (id: string, hidden: boolean): Promise<RepoSummary> =>
    unwrap(CodeWorkspaceService.SetRepoHidden({ id, hidden })).then((r) => trust<RepoSummary>(r)),
  codeWorkspaceSetRepoColor: (id: string, color: PaletteColor): Promise<RepoSummary> =>
    unwrap(CodeWorkspaceService.SetRepoColor({ id, color })).then((r) => trust<RepoSummary>(r)),
  codeWorkspaceReorderRepos: (ids: string[]): Promise<RepoSummary[]> =>
    unwrap(CodeWorkspaceService.ReorderRepos({ ids })).then((r) => trust<RepoSummary[]>(r ?? [])),
  codeWorkspaceRemoveRepo: (id: string): Promise<void> =>
    unwrap(CodeWorkspaceService.RemoveRepo({ id })),
  codeWorkspaceListFiles: (id: string): Promise<FileListing> =>
    unwrap<Awaited<ReturnType<typeof CodeWorkspaceService.ListFiles>>>(
      CodeWorkspaceService.ListFiles({ id }),
    ).then((r) => trust<FileListing>({ ...r, paths: r.paths ?? [], status: r.status ?? {} })),
  codeWorkspaceReadFile: (id: string, path: string, ifStat?: FileStat): Promise<FileRead> =>
    unwrap(CodeWorkspaceService.ReadFile({ id, path, ifStat })).then((r) => trust<FileRead>(r)),
  codeWorkspaceReadFileForEdit: (id: string, path: string): Promise<EditContent> =>
    unwrap(CodeWorkspaceService.ReadFileForEdit({ id, path })).then((r) => trust<EditContent>(r)),
  codeWorkspaceWriteFile: (
    id: string,
    path: string,
    text: string,
    baseHash: string,
  ): Promise<WriteFileResult> =>
    unwrap(CodeWorkspaceService.WriteFile({ id, path, text, baseHash })).then((r) =>
      trust<WriteFileResult>(r),
    ),

  // External opens: the files this window was opened for (once), and a standalone file's content.
  openedFilesPending: (): Promise<OpenedFileTarget[]> =>
    unwrap(OpenedFilesService.Pending({ windowKey })).then((r) =>
      trust<OpenedFileTarget[]>(r ?? []),
    ),
  openedFilesRead: (path: string): Promise<FileContent> =>
    unwrap(OpenedFilesService.Read({ path })).then((r) => trust<FileContent>(r)),

  // Default app for code and data files: the live handler of every associated extension, and the
  // switch that makes Kira Space (or the previous app) the default for some of them.
  fileHandlersStatus: (): Promise<FileHandlersStatus> =>
    unwrap(FileHandlersService.Status()).then((r) => trust<FileHandlersStatus>(r)),
  fileHandlersSet: (exts: string[], on: boolean): Promise<FileHandlersSetResult> =>
    unwrap<{ unrestored: string[] | null }>(FileHandlersService.Set({ exts, on })).then((r) =>
      trust<FileHandlersSetResult>({ unrestored: r.unrestored ?? [] }),
    ),

  // C6 §7/§8.5: the index lifecycle (fire-and-forget — a failed start/stop must never block
  // opening or leaving a workspace) plus the diff read.
  codeWorkspaceOpenWorkspace: (id: string): Promise<void> =>
    unwrap(CodeWorkspaceService.OpenWorkspace({ id })),
  codeWorkspaceCloseWorkspace: (id: string): Promise<void> =>
    unwrap(CodeWorkspaceService.CloseWorkspace({ id })),
  codeWorkspaceReadDiff: (
    id: string,
    path: string,
    gate?: { ifStat?: FileStat; ifHeadOid?: string },
  ): Promise<DiffRead> =>
    unwrap(CodeWorkspaceService.ReadDiff({ id, path, ...gate })).then((r) => trust<DiffRead>(r)),

  // C7 §5/D7: StartSearch returns as soon as the background scan starts — its own searchId is how
  // the renderer matches a later onCodeSearch event to the run that's waiting on it.
  codeWorkspaceStartSearch: (id: string, req: SearchRequest): Promise<{ searchId: string }> =>
    unwrap(CodeWorkspaceService.StartSearch({ id, windowKey, ...req })).then((r) =>
      trust<{ searchId: string }>(r),
    ),
  codeWorkspaceCancelSearch: (id: string): Promise<void> =>
    unwrap(CodeWorkspaceService.CancelSearch({ id })),
  onCodeSearch: (cb: (event: CodeSearchEvent) => void): (() => void) => on(CHANNEL.codeSearch, cb),

  // Agent-monitor surface: `terminalAgentSessions`/
  // `onAgentSessions`/`onAgentEvent` satisfy `AgentSessionsControl` structurally (P127's shared
  // store factory, `ade/state.ts`'s own call), the same "no adapter" shape
  // `createAgentSessionsStore`'s own doc comment states.
  terminalAgentSessions: (): Promise<AgentSessionsEvent> =>
    unwrap(TerminalService.AgentSessions()).then((r) => trust<AgentSessionsEvent>(r)),
  onAgentSessions: (cb: (event: AgentSessionsEvent) => void): (() => void) =>
    on(CHANNEL.agentSessions, cb),
  onAgentEvent: (cb: (event: AgentEvent) => void): (() => void) => on(CHANNEL.agentEvent, cb),

  // P132 Part 2: the in-memory git op log's snapshot, cancel and live-update push.
  opsRecent: (limit: number): Promise<SpaceOpRecord[]> =>
    unwrap(OpsService.Recent({ limit })).then((r) => trust<SpaceOpRecord[]>(r ?? [])),
  opsCancel: (opId: string): Promise<void> => unwrap(OpsService.Cancel({ opId })),
  onOpUpdate: (cb: (record: SpaceOpRecord) => void): (() => void) => on(CHANNEL.opUpdate, cb),
  onOpBatch: (cb: (records: SpaceOpRecord[]) => void): (() => void) => on(CHANNEL.opBatch, cb),

  // P144: AdeTaskService (v2 task board). Types from ../ade/v2/wire; payload-free pushes invalidate queries.
  adeTaskBoard: (): Promise<V2.Board> =>
    unwrap(AdeTaskService.Board()).then((r) => trust<V2.Board>(r)),
  adeTaskChangeRequests: (): Promise<V2.ChangeRequestsResult> =>
    unwrap(AdeTaskService.ChangeRequests()).then((r) => trust<V2.ChangeRequestsResult>(r)),
  adeTaskRefresh: (args: V2.RefreshArgs): Promise<V2.RefreshResult> =>
    unwrap(AdeTaskService.Refresh(args)).then((r) => trust<V2.RefreshResult>(r)),
  adeTaskForcePush: (args: V2.BranchArgs): Promise<V2.ForcePushResult> =>
    unwrap(AdeTaskService.ForcePush(args)).then((r) => trust<V2.ForcePushResult>(r)),
  adeTaskCreateTask: (args: V2.CreateTaskArgs): Promise<V2.Task> =>
    unwrap(AdeTaskService.CreateTask(args)).then((r) => trust<V2.Task>(r)),
  adeTaskUpdateTask: (args: V2.UpdateTaskArgs): Promise<V2.Task> =>
    unwrap(AdeTaskService.UpdateTask(args)).then((r) => trust<V2.Task>(r)),
  adeTaskAddTaskRepo: (args: V2.AddTaskRepoArgs): Promise<V2.Branch> =>
    unwrap(AdeTaskService.AddTaskRepo(args)).then((r) => trust<V2.Branch>(r)),
  adeTaskSetTaskBranch: (args: V2.SetTaskBranchArgs): Promise<V2.Branch> =>
    unwrap(AdeTaskService.SetTaskBranch(args)).then((r) => trust<V2.Branch>(r)),
  adeTaskUpdateTaskRepo: (args: V2.UpdateTaskRepoArgs): Promise<V2.Branch> =>
    unwrap(AdeTaskService.UpdateTaskRepo(args)).then((r) => trust<V2.Branch>(r)),
  adeTaskRemoveTaskBranch: (args: V2.RemoveTaskRowArgs): Promise<void> =>
    unwrap(AdeTaskService.RemoveTaskBranch(args)),
  adeTaskRemoveTaskRepo: (args: V2.RemoveTaskRowArgs): Promise<void> =>
    unwrap(AdeTaskService.RemoveTaskRepo(args)),
  adeTaskTaskBranchRisk: (args: V2.BranchArgs): Promise<V2.BranchRisk> =>
    unwrap(AdeTaskService.TaskBranchRisk(args)).then((r) => trust<V2.BranchRisk>(r)),
  adeTaskCandidateBranches: (args: V2.CandidateBranchesArgs): Promise<V2.CandidateBranchesResult> =>
    unwrap(AdeTaskService.CandidateBranches(args)).then((r) =>
      trust<V2.CandidateBranchesResult>(r),
    ),
  adeTaskAddExistingBranch: (args: V2.AddExistingBranchArgs): Promise<V2.AddExistingBranchResult> =>
    unwrap(AdeTaskService.AddExistingBranch(args)).then((r) =>
      trust<V2.AddExistingBranchResult>(r),
    ),
  adeTaskResolveHostLink: (args: V2.HostLinkArgs): Promise<V2.HostLinkTarget> =>
    unwrap(AdeTaskService.ResolveHostLink(args)).then((r) => trust<V2.HostLinkTarget>(r)),
  adeTaskAddHostLink: (args: V2.HostLinkArgs): Promise<V2.AddExistingBranchResult> =>
    unwrap(AdeTaskService.AddHostLink(args)).then((r) => trust<V2.AddExistingBranchResult>(r)),
  adeTaskSetPlan: (args: V2.SetPlanArgs): Promise<void> => unwrap(AdeTaskService.SetPlan(args)),
  adeTaskShelveTask: (args: V2.ShelveTaskArgs): Promise<void> =>
    unwrap(AdeTaskService.ShelveTask(args)),
  adeTaskCaptureTask: (args: V2.CaptureTaskArgs): Promise<V2.Task> =>
    unwrap(AdeTaskService.CaptureTask(args)).then((r) => trust<V2.Task>(r)),
  adeTaskSetQueuedAfter: (args: V2.SetQueuedAfterArgs): Promise<void> =>
    unwrap(AdeTaskService.SetQueuedAfter(args)),
  adeTaskWorkflows: (): Promise<V2.WorkflowsResult> =>
    unwrap(AdeTaskService.Workflows()).then((r) => trust<V2.WorkflowsResult>(r)),
  adeTaskRepos: (): Promise<V2.ReposResult> =>
    unwrap(AdeTaskService.Repos()).then((r) => trust<V2.ReposResult>(r)),
  adeTaskWorkflowYaml: (args: V2.FileNameArgs): Promise<V2.WorkflowYaml> =>
    unwrap(AdeTaskService.WorkflowYaml(args)).then((r) => trust<V2.WorkflowYaml>(r)),
  adeTaskValidateWorkflowYaml: (
    args: V2.ValidateWorkflowYamlArgs,
  ): Promise<V2.WorkflowValidation> =>
    unwrap(AdeTaskService.ValidateWorkflowYaml(args)).then((r) => trust<V2.WorkflowValidation>(r)),
  adeTaskSaveWorkflow: (args: V2.SaveWorkflowArgs): Promise<V2.WorkflowEntry> =>
    unwrap(AdeTaskService.SaveWorkflow(args)).then((r) => trust<V2.WorkflowEntry>(r)),
  adeTaskSaveWorkflowYaml: (args: V2.SaveWorkflowYamlArgs): Promise<V2.WorkflowEntry> =>
    unwrap(AdeTaskService.SaveWorkflowYaml(args)).then((r) => trust<V2.WorkflowEntry>(r)),
  adeTaskImportWorkflow: (args: V2.ImportWorkflowArgs): Promise<V2.WorkflowEntry> =>
    unwrap(AdeTaskService.ImportWorkflow(args)).then((r) => trust<V2.WorkflowEntry>(r)),
  adeTaskNewWorkflow: (args: V2.NewWorkflowArgs): Promise<V2.WorkflowEntry> =>
    unwrap(AdeTaskService.NewWorkflow(args)).then((r) => trust<V2.WorkflowEntry>(r)),
  adeTaskUpdateRepo: (args: V2.UpdateRepoArgs): Promise<V2.Repo> =>
    unwrap(AdeTaskService.UpdateRepo(args)).then((r) => trust<V2.Repo>(r)),
  adeTaskAddFolder: (args: V2.FolderArgs): Promise<V2.FolderImportResult> =>
    unwrap(AdeTaskService.AddFolder(args)).then((r) => trust<V2.FolderImportResult>(r)),
  adeTaskSetFolderWatch: (args: V2.FolderArgs): Promise<V2.Folder> =>
    unwrap(AdeTaskService.SetFolderWatch(args)).then((r) => trust<V2.Folder>(r)),
  adeTaskSetFolderHidden: (args: V2.FolderHiddenArgs): Promise<V2.Folder> =>
    unwrap(AdeTaskService.SetFolderHidden(args)).then((r) => trust<V2.Folder>(r)),
  adeTaskRemoveFolder: (args: V2.PathArgs): Promise<void> =>
    unwrap(AdeTaskService.RemoveFolder(args)),
  adeTaskRecordMerge: (args: V2.RecordMergeArgs): Promise<void> =>
    unwrap(AdeTaskService.RecordMerge(args)),
  adeTaskStartRun: (args: V2.StartRunArgs): Promise<V2.StartRunResult> =>
    unwrap(AdeTaskService.StartRun(args)).then((r) => trust<V2.StartRunResult>(r)),
  adeTaskSetTaskWorkflow: (args: V2.SetTaskWorkflowArgs): Promise<V2.Task> =>
    unwrap(AdeTaskService.SetTaskWorkflow(args)).then((r) => trust<V2.Task>(r)),
  adeTaskApprove: (args: V2.StepArgs): Promise<void> => unwrap(AdeTaskService.Approve(args)),
  adeTaskRetryRun: (args: V2.RunArgs): Promise<void> => unwrap(AdeTaskService.RetryRun(args)),
  adeTaskResumeRun: (args: V2.RunArgs): Promise<void> => unwrap(AdeTaskService.ResumeRun(args)),
  adeTaskStepDone: (args: V2.StepDoneArgs): Promise<V2.Task> =>
    unwrap(AdeTaskService.StepDone(args)).then((r) => trust<V2.Task>(r)),
  adeTaskHandovers: (args: V2.TaskArgs): Promise<V2.HandoversResult> =>
    unwrap(AdeTaskService.Handovers(args)).then((r) => trust<V2.HandoversResult>(r)),
  adeTaskUpdateHandover: (args: V2.UpdateHandoverArgs): Promise<void> =>
    unwrap(AdeTaskService.UpdateHandover(args)),
  adeTaskDismissHandover: (args: V2.TaskArgs): Promise<void> =>
    unwrap(AdeTaskService.DismissHandover(args)),
  adeTaskArtifacts: (args: V2.TaskArgs): Promise<V2.ArtifactsResult> =>
    unwrap(AdeTaskService.Artifacts(args)).then((r) => trust<V2.ArtifactsResult>(r)),
  adeTaskIssue: (args: V2.TaskArgs): Promise<V2.TaskIssue> =>
    unwrap(IssueTrackerService.TaskIssue(args)).then((r) => trust<V2.TaskIssue>(r)),
  cliTools: (refresh: boolean): Promise<CliToolsResult> =>
    unwrap(IssueTrackerService.Tools({ refresh })).then((r) => trust<CliToolsResult>(r)),
  issueTrackers: (): Promise<IssueTrackers> =>
    unwrap(IssueTrackerService.Trackers()).then((r) => trust<IssueTrackers>(r)),
  adeTaskAddArtifact: (args: V2.AddArtifactArgs): Promise<V2.Artifact> =>
    unwrap(AdeTaskService.AddArtifact(args)).then((r) => trust<V2.Artifact>(r)),
  adeTaskRemoveArtifact: (args: V2.RemoveArtifactArgs): Promise<void> =>
    unwrap(AdeTaskService.RemoveArtifact(args)),
  adeTaskSetTaskStep: (args: V2.SetTaskStepArgs): Promise<V2.Task> =>
    unwrap(AdeTaskService.SetTaskStep(args)).then((r) => trust<V2.Task>(r)),
  adeTaskSaveStatuses: (args: V2.SaveStatusesArgs): Promise<V2.SaveStatusesResult> =>
    unwrap(AdeTaskService.SaveStatuses(args)).then((r) => trust<V2.SaveStatusesResult>(r)),
  adeTaskRetrySetup: (args: V2.BranchArgs): Promise<void> =>
    unwrap(AdeTaskService.RetrySetup(args)),
  adeTaskReadLog: (args: V2.ReadLogArgs): Promise<V2.LogPage> =>
    unwrap(AdeTaskService.ReadLog(args)).then((r) => trust<V2.LogPage>(r)),
  adeTaskRunSandbox: (args: V2.RunArgs): Promise<V2.RunSandboxDetail> =>
    unwrap(AdeTaskService.RunSandbox(args)).then((r) => trust<V2.RunSandboxDetail>(r)),
  adeTaskSessions: (): Promise<V2.SessionsResult> =>
    unwrap(AdeTaskService.Sessions()).then((r) => trust<V2.SessionsResult>(r)),
  adeTaskStopRun: (args: V2.RunArgs): Promise<void> => unwrap(AdeTaskService.StopRun(args)),
  adeTaskReviewStep: (args: V2.BranchArgs): Promise<V2.ReviewStepState> =>
    unwrap(AdeTaskService.ReviewStep(args)).then((r) => trust<V2.ReviewStepState>(r)),
  adeTaskSendReview: (args: V2.SendReviewArgs): Promise<void> =>
    unwrap(AdeTaskService.SendReview(args)),
  adeTaskFinishReview: (args: V2.FinishReviewArgs): Promise<void> =>
    unwrap(AdeTaskService.FinishReview(args)),
  adeTaskTakeOver: (args: V2.TakeOverArgs): Promise<V2.Launch> =>
    unwrap(AdeTaskService.TakeOver(args)).then((r) => trust<V2.Launch>(r)),
  adeTaskRestartSession: (args: V2.RestartSessionArgs): Promise<V2.Launch> =>
    unwrap(AdeTaskService.RestartSession(args)).then((r) => trust<V2.Launch>(r)),
  adeTaskCloseSession: (args: V2.CloseSessionArgs): Promise<void> =>
    unwrap(AdeTaskService.CloseSession(args)),
  adeTaskLaunchStep: (args: V2.LaunchStepArgs): Promise<V2.Launch> =>
    unwrap(AdeTaskService.LaunchStep(args)).then((r) => trust<V2.Launch>(r)),
  adeTaskStartBranch: (args: V2.StartBranchArgs): Promise<V2.Launch> =>
    unwrap(AdeTaskService.StartBranch(args)).then((r) => trust<V2.Launch>(r)),
  adeTaskStartAdHoc: (args: V2.BranchArgs): Promise<V2.Launch> =>
    unwrap(AdeTaskService.StartAdHoc(args)).then((r) => trust<V2.Launch>(r)),
  adeTaskRebasePreview: (args: V2.OntoArgs): Promise<V2.RebasePreview> =>
    unwrap(AdeTaskService.RebasePreview(args)).then((r) => trust<V2.RebasePreview>(r)),
  adeTaskRebase: (args: V2.RebaseArgs): Promise<V2.RebaseStart> =>
    unwrap(AdeTaskService.Rebase(args)).then((r) => trust<V2.RebaseStart>(r)),
  adeTaskAbortRebase: (args: V2.BranchArgs): Promise<void> =>
    unwrap(AdeTaskService.AbortRebase(args)),
  adeTaskSetBranchBase: (args: V2.SetBranchBaseArgs): Promise<void> =>
    unwrap(AdeTaskService.SetBranchBase(args)),
  adeTaskRepoBranches: (args: V2.RepoBranchesArgs): Promise<V2.RepoBranches> =>
    unwrap(AdeTaskService.RepoBranches(args)).then((r) => trust<V2.RepoBranches>(r)),
  adeTaskSend: (args: V2.SendArgs): Promise<void> => unwrap(AdeTaskService.Send(args)),
  adeTaskFocusSession: (args: V2.FocusSessionArgs): Promise<boolean> =>
    unwrap(AdeTaskService.FocusSession(args)),
  adeTaskOpenReviewWindow: (args: V2.BranchArgs): Promise<boolean> =>
    unwrap(AdeTaskService.OpenReviewWindow(args)),
  adeTaskReviewWindowTarget: (args: V2.WindowKeyArgs): Promise<V2.ReviewWindowTarget | null> =>
    unwrap(AdeTaskService.ReviewWindowTarget(args)).then((r) =>
      trust<V2.ReviewWindowTarget | null>(r),
    ),
  adeTaskReviewWorking: (args: V2.BranchArgs): Promise<V2.ReviewWorking> =>
    unwrap(AdeTaskService.ReviewWorking(args)).then((r) => trust<V2.ReviewWorking>(r)),
  adeTaskReviewWorkingDiff: (args: V2.ReviewWorkingDiffArgs): Promise<DiffContent> =>
    unwrap(AdeTaskService.ReviewWorkingDiff(args)).then((r) => trust<DiffContent>(r)),
  adeTaskReviewWorkingFile: (args: V2.ReviewWorkingDiffArgs): Promise<V2.ReviewWorkingFileReview> =>
    unwrap(AdeTaskService.ReviewWorkingFile(args)).then((r) =>
      trust<V2.ReviewWorkingFileReview>(r),
    ),
  adeTaskReviewWorkingMark: (args: V2.ReviewWorkingMarkArgs): Promise<V2.ReviewWorkingMarkResult> =>
    unwrap(AdeTaskService.ReviewWorkingMark(args)).then((r) =>
      trust<V2.ReviewWorkingMarkResult>(r),
    ),
  adeTaskReviewWorkingComments: (args: V2.BranchArgs): Promise<V2.ReviewWorkingCommentsResult> =>
    unwrap(AdeTaskService.ReviewWorkingComments(args)).then((r) =>
      trust<V2.ReviewWorkingCommentsResult>(r),
    ),
  adeTaskReviewWorkingCommentAdd: (args: V2.ReviewWorkingCommentAddArgs): Promise<ReviewComment> =>
    unwrap(AdeTaskService.ReviewWorkingCommentAdd(args)).then((r) => trust<ReviewComment>(r)),
  adeTaskReviewAgent: (args: V2.TaskArgs): Promise<V2.ReviewAgentState> =>
    unwrap(AdeTaskService.ReviewAgent(args)).then((r) => trust<V2.ReviewAgentState>(r)),
  adeTaskLaunchReviewAgent: (args: V2.TaskArgs): Promise<V2.ReviewAgentLaunch> =>
    unwrap(AdeTaskService.LaunchReviewAgent(args)).then((r) => trust<V2.ReviewAgentLaunch>(r)),
  adeTaskForwardReviewComments: (args: V2.BranchArgs): Promise<V2.ForwardReviewResult> =>
    unwrap(AdeTaskService.ForwardReviewComments(args)).then((r) =>
      trust<V2.ForwardReviewResult>(r),
    ),
  adeTaskReviewForwardText: (args: V2.BranchArgs): Promise<V2.ReviewForwardText> =>
    unwrap(AdeTaskService.ReviewForwardText(args)).then((r) => trust<V2.ReviewForwardText>(r)),
  adeTaskArchiveRisk: (args: V2.TaskArgs): Promise<V2.ArchiveRisk> =>
    unwrap(AdeTaskService.ArchiveRisk(args)).then((r) => trust<V2.ArchiveRisk>(r)),
  adeTaskArchiveTask: (args: V2.TaskArgs): Promise<void> =>
    unwrap(AdeTaskService.ArchiveTask(args)),
  adeTaskArchivedTask: (args: V2.TaskArgs): Promise<V2.ArchivedTask> =>
    unwrap(AdeTaskService.ArchivedTask(args)).then((r) => trust<V2.ArchivedTask>(r)),
  adeTaskBranchRequest: (args: V2.BranchRequestArgs): Promise<V2.BranchRequest> =>
    unwrap(AdeTaskService.BranchRequest(args)).then((r) => trust<V2.BranchRequest>(r)),
  adeTaskApproveBranchRequest: (
    args: V2.ApproveBranchRequestArgs,
  ): Promise<V2.BranchRequestAnswer> =>
    unwrap(AdeTaskService.ApproveBranchRequest(args)).then((r) => trust<V2.BranchRequestAnswer>(r)),
  adeTaskDenyBranchRequest: (args: V2.BranchRequestArgs): Promise<V2.BranchRequestAnswer> =>
    unwrap(AdeTaskService.DenyBranchRequest(args)).then((r) => trust<V2.BranchRequestAnswer>(r)),
  adeTaskRestoreTask: (args: V2.TaskArgs): Promise<V2.Task> =>
    unwrap(AdeTaskService.RestoreTask(args)).then((r) => trust<V2.Task>(r)),
  onAdeTaskBoard: (cb: () => void): (() => void) => on(CHANNEL.adeTaskBoard, cb),
  onAdeTaskWorkflows: (cb: () => void): (() => void) => on(CHANNEL.adeTaskWorkflows, cb),
  onAdeTaskRepos: (cb: () => void): (() => void) => on(CHANNEL.adeTaskRepos, cb),
  onAdeTaskRuns: (cb: (event: V2.RunsChangedEvent) => void): (() => void) =>
    on(CHANNEL.adeTaskRuns, cb),
  onAdeTaskLog: (cb: (event: V2.LogEvent) => void): (() => void) => on(CHANNEL.adeTaskLog, cb),
  onAdeTaskSessions: (cb: () => void): (() => void) => on(CHANNEL.adeTaskSessions, cb),
  onAdeTaskReview: (cb: (event: V2.ReviewEvent) => void): (() => void) =>
    on(CHANNEL.adeTaskReview, cb),
  onAdeTaskOpenSession: (cb: (event: V2.OpenSessionEvent) => void): (() => void) =>
    on(CHANNEL.adeTaskOpenSession, cb),

  // P238: desktop agent notifications. The window reports what it shows; a notification click
  // pushes the reveal events back to the owning window.
  agentNotifyReportFocus: (args: Omit<AgentNotifyFocus, 'windowKey'>): Promise<void> =>
    unwrap(AgentNotifyService.ReportFocus({ windowKey, ...args })),
  agentNotifySendTest: (): Promise<void> => unwrap(AgentNotifyService.SendTest()),
  onAgentRevealTerminal: (cb: (event: { terminalId: string }) => void): (() => void) =>
    on('kira:agent:revealTerminal', cb),
  onAgentRevealTask: (cb: (event: { taskId: string }) => void): (() => void) =>
    on('kira:agent:revealTask', cb),
  onAgentRevealScriptRun: (cb: (event: { runId: string; label: string }) => void): (() => void) =>
    on('kira:agent:revealScriptRun', cb),
  // P239: Claude Code usage limits for the ADE status bar.
  agentUsageGet: (): Promise<AgentUsageSnapshot> => unwrap(AgentUsageService.Get()),
  onAgentUsage: (cb: (snapshot: AgentUsageSnapshot) => void): (() => void) =>
    on('kira:agent:usage', cb),
};

export interface AgentUsageWindow {
  usedPercent: number;
  /** Unix milliseconds. */
  resetsAt: number;
}

export interface AgentUsageSnapshot {
  /** ok, waiting or off. */
  state: string;
  /** session or run; empty until a reading arrives. */
  source: string;
  fiveHour: AgentUsageWindow | null;
  sevenDay: AgentUsageWindow | null;
  /** Unix milliseconds. */
  updatedAt: number;
}

interface AgentNotifyFocus {
  windowKey: string;
  focused: boolean;
  module: string;
  activeTerminalId: string;
  adeTaskId: string;
  activeScriptRunId: string;
}

// P103 Part 2 (§5.6): the shared methods (createCoreControl.ts, P116 H5/P119 grew that set) plus
// this app's own remaining ones (spaceControl, above) — every `control.xxx()` call site in the app
// is unchanged, since neither the method names nor their bound-call FQNs moved.
export const control = {
  ...createCoreControl<Settings, Layout, TabRecord, SettingsPatch, SpaceMode>({
    settings: SettingsService,
    layout: LayoutService,
    tabs: TabsService,
    lifecycle: LifecycleService,
    terminal: TerminalService,
    files: FilesService,
    windows: WindowsService,
    keepAwake: KeepAwakeService,
    update: UpdateService,
    prompts: PromptsService,
    llmPrompts: LlmPromptsService,
    agent: AgentService,
  }),
  ...memoryControl,
  ...spaceControl,
};
