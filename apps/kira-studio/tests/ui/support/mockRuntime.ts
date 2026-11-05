import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import { defaultLayout } from '@shared/domain/layout';
import {
  buildChannelMaps,
  type ControlSnapshot,
  resolveWailsRuntimeJsPath,
  emitWailsEvent as sharedEmitWailsEvent,
  installControlMocks as sharedInstallControlMocks,
} from '@workbench/testing/ui/mockRuntime';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { STUDIO_TAB_KIND_MODE, type StudioTabKind } from '../../../frontend/src/state/tabDomain';
import { contract } from './contract';
import { IPC } from './ipcChannels';

export type { ControlLogEntry, ControlMockHandle } from '@workbench/testing/ui/mockRuntime';

// The real Wails runtime, served under /wails/ so the app's own `import ... from
// '/wails/runtime.js'` (control.ts, port.ts, every generated binding) loads real
// Call/Events/Stream logic in the browser — not a hand-rolled stand-in of it.
//
// This is NOT `node_modules/@wailsio/runtime/dist/index.js` — that package is the *unbundled* ESM
// source (a dozen files, each `import`ing its siblings by relative path), meant to be bundled
// together with the rest of the app at Vite/esbuild build time, the way the generated bindings
// already are. A real packaged Wails app serves something else entirely at the literal URL
// `/wails/runtime.js`: a single, dependency-free, pre-minified bundle the `wails3` CLI embeds,
// found by reading the pinned module (tried serving the npm package's
// dist/ tree directly first; a same-name collision surfaced immediately: `calls.js`'s own `import
// … from "./runtime.js"` resolves, relative to its own URL `/wails/calls.js`, to the exact same
// `/wails/runtime.js` path the *app* imports for the aggregate bundle, but needs the low-level
// dist/runtime.js file instead — two different files, one URL, unresolvable by aliasing alone).
// `go list` resolves the on-disk path for whatever version go.mod pins, rather than
// hand-writing a GOPATH-shaped path that would silently go stale on a version bump.
const WAILS_RUNTIME_JS = resolveWailsRuntimeJsPath(resolve(__dirname, '../../../'));

const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge';

// One line per request/response channel — the FQN half of each pair, read off the generated
// bindings themselves (`grep -rhoE '\$Call\.ByName\("[^"]+"' apps/kira-studio/frontend/bindings/.../bridge/*.ts`),
// not retyped from memory.
const FQN_SUFFIX_BY_IPC_KEY: Record<string, string> = {
  appInfo: 'AppService.Info',
  settingsGetAll: 'SettingsService.GetAll',
  settingsSet: 'SettingsService.Set',
  layoutGetAll: 'LayoutService.GetAll',
  layoutSet: 'LayoutService.Set',
  appFlushed: 'LifecycleService.Flushed',
  windowFlushed: 'LifecycleService.WindowFlushed',
  lifecycleConfirmQuit: 'LifecycleService.ConfirmQuit',
  lifecycleCancelQuit: 'LifecycleService.CancelQuit',
  lifecycleQuitStatus: 'LifecycleService.QuitStatus',
  filesChooseSave: 'FilesService.ChooseSave',
  filesChooseOpen: 'FilesService.ChooseOpen',
  filesChooseOpenMany: 'FilesService.ChooseOpenMany',
  filesChooseFolder: 'FilesService.ChooseFolder',
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
  opsRecent: 'OpsService.Recent',
  opsCancel: 'OpsService.Cancel',
  httpSend: 'HttpService.Send',
  // P90 item 2's three cookie bound calls.
  httpCookies: 'HttpService.Cookies',
  httpDeleteCookie: 'HttpService.DeleteCookie',
  httpClearCookies: 'HttpService.ClearCookies',
  windowsEnsure: 'WindowsService.Ensure',
  windowsSetMode: 'WindowsService.SetMode',
  windowsOpenNew: 'WindowsService.OpenNew',
  tabsList: 'TabsService.List',
  tabsSave: 'TabsService.Save',
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

  updateStatus: 'UpdateService.Status',
  updateCheck: 'UpdateService.Check',
  updateInstall: 'UpdateService.InstallUpdate',
  updateCancelInstall: 'UpdateService.CancelInstall',

  terminalOpen: 'TerminalService.Open',
  terminalWrite: 'TerminalService.Write',
  terminalResize: 'TerminalService.Resize',
  terminalClose: 'TerminalService.Close',
  terminalDefaultCwd: 'TerminalService.DefaultCwd',

  customScriptsList: 'CustomScriptsService.List',
  customScriptsCreate: 'CustomScriptsService.Create',
  configFilesOpen: 'ConfigFilesService.Open',
  configFilesReveal: 'ConfigFilesService.Reveal',
  customScriptsDelete: 'CustomScriptsService.Delete',
  customScriptsFile: 'CustomScriptsService.File',
  customScriptsSaveFile: 'CustomScriptsService.SaveFile',
  customScriptsForm: 'CustomScriptsService.Form',
  customScriptsSetSecrets: 'CustomScriptsService.SetSecrets',
  customScriptsReorder: 'CustomScriptsService.Reorder',
  customScriptsReorderCollections: 'CustomScriptsService.ReorderCollections',
  customScriptsMove: 'CustomScriptsService.Move',
  customScriptsRenameCollection: 'CustomScriptsService.RenameCollection',
  customScriptsDeleteCollection: 'CustomScriptsService.DeleteCollection',
  scriptRunsList: 'ScriptRunsService.List',
  scriptRunsNextFires: 'ScriptRunsService.NextFires',
  scriptRunsMcpServers: 'ScriptRunsService.McpServers',
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

  linkOpenExternal: 'LinkService.OpenExternal',
  keepAwakeStatus: 'KeepAwakeService.Status',
  keepAwakeSetManual: 'KeepAwakeService.SetManual',

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

/** ipc.ts's legacy channel string (what every `ControlSnapshot.channel` and fixture is keyed by)
 *  mapped onto the FQN `$Call.ByName` sends over the wire today. This table
 *  is the one piece of new coupling the bridge rewrite introduces —
 *  tests/unit/mock-runtime-bindings.spec.ts asserts its values equal the generated bindings' own
 *  `$Call.ByName("…")` literals. */
export const { channelToFqn: CHANNEL_TO_FQN, fqnToChannel: FQN_TO_CHANNEL } = buildChannelMaps(
  IPC,
  FQN_SUFFIX_BY_IPC_KEY,
  BRIDGE_PKG,
);

// A call this mock never expects a fixture to cover, answered the same way regardless of a
// spec's own args — pre-serialised JSON, keyed by channel; used only when a channel has *no*
// fixture-supplied snapshot at all (a spec that provides its own still wins). Every member here
// is a call `tests/ipc/**/*.fixture.ts` never captures because the pre-P57 mock
// (`tests/ipc/support/mockControl.ts`) only overrode the channels it had snapshots for —
// everything else fell through to a real (if temporary) backend, so these are that same
// no-op/empty answer made explicit, not a gap in the fixtures:
//   - `filtersList`: `project/state/tree.ts` calls it once for every connection the instant it
//     reaches `connected` (tree.ts:118) — "no filters yet" for a connection never seen before,
//     `EMPTY_VISIBILITY` (`shared/domain/treeFilter.ts`) made explicit.
//   - `windowsEnsure` (P8, mode added P22 D12): `main.ts`'s `bootstrap()` awaits this once, before
//     hydrateTabs() or anything else window-scoped runs — always a no-op (bar the mode read) on the
//     real native shell (the window's own `windows` row already exists by the time its URL loads),
//     so there is nothing meaningful to snapshot beyond the app's own default mode.
//   - `windowsSetMode` (P22 D12): the debounced mode-persistence writer — fire-and-forget, a spec
//     never asserts on its own echo. Void.
//   - `tabsSave`: `state/tabs.ts` debounce-persists on every tab mutation (typing in a console
//     tab, opening/closing a tab, …) with a tab array keyed by a fresh UUID every test run —
//     never a value any committed fixture could match on args, so it cannot be captured even in
//     principle. Void.
//   - `layoutSet`/`settingsSet`: the same debounced-persist shape as `tabsSave`, for panel
//     resizes/toggles and settings edits — echoed back as the untouched default, since no ipc
//     frontend spec asserts on the echo itself.
//   - `opsCancel`/`queriesHistoryRecord`: fire-and-forget, `opId`/free-form-filter args a spec
//     never asserts on. Void.
//   - `treeInvalidate` (P24 D7): `project/state/tree.ts`'s `refresh`/`refreshConnection`/
//     `refreshObject` all await it before the read that follows — a spec asserts on THAT read
//     (the `treeChildren`/`treeDescribe` call it produces, or the op count it changes), never on
//     this call's own echo, so a real backend's void response is the only answer worth mocking.
//     `path` varies per row (a connection's own Refresh omits it entirely), so no single args
//     shape could be captured as a normal fixture snapshot even in principle. Void.
//   - `treeDescribe`: `views/grid/state.ts`'s `loadMeta` calls it once per data tab opened,
//     purely to feed the projection menu — wrapped in its own `try { … } catch {}` ("a failure
//     here must not block reading rows", the function's own comment), so a real, uncaptured miss
//     already degrades silently in the app; this just stops it from also being *loud* at the
//     network layer. The placeholder `ObjectMeta` is never read by anything a spec asserts on.
//   - `queriesList`/`queriesListConsole`/`queriesHistoryList`: unlike `treeDescribe`, these are
//     NOT caught — `project/state/tree.ts`'s `loadSavedQueries` runs uncaught "right before
//     opening a relation's context menu" (its own comment), so a real miss here doesn't just
//     degrade a menu, it silently stops the context menu from opening at all. "No saved queries
//     yet for a path never seen before" is the correct empty-list answer regardless.
// Mirrors `mockStreamBrowser.js`'s own `'ping'` special case for the identical reason.
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
const WILDCARD_DEFAULTS: Readonly<Record<string, string>> = Object.freeze({
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
  [IPC.windowsSetMode]: 'null',
  [IPC.linkOpenExternal]: 'null',
  // P246: the prompt router's calls; a spec drives the list through IPC.promptsChanged pushes.
  [IPC.promptsList]: '[]',
  [IPC.promptsClaim]: 'null',
  [IPC.promptsRaise]: 'null',
  [IPC.promptsDismiss]: 'null',
  [IPC.promptsSendTest]: 'null',
  // The prompt editor's calls; a spec answers List/Save from tests/contract/llm-prompts.json.
  // The active agent provider's names: the claude provider, as the Go flow test writes it.
  [IPC.agentInfo]: JSON.stringify(contract('agent', 'AgentService.Info')),
  [IPC.agentInfoFor]: JSON.stringify(contract('agent', 'AgentService.Info')),
  [IPC.agentList]: '[]',
  [IPC.llmPromptsList]: '{"prompts":[],"storeError":"","revision":0}',
  [IPC.llmPromptsSave]: '{"prompts":[],"storeError":"","revision":0}',
  [IPC.llmPromptsPreview]: '{"errors":[],"parts":[]}',
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
  [IPC.tabsSave]: 'null',
  [IPC.layoutSet]: JSON.stringify(defaultLayout),
  [IPC.settingsSet]: JSON.stringify(defaultSettings),
  [IPC.opsCancel]: 'null',
  [IPC.queriesHistoryRecord]: 'null',
  [IPC.treeInvalidate]: 'null',
  [IPC.treeDescribe]: JSON.stringify({ meta: EMPTY_OBJECT_META, source: 'server' }),
  // SlickGridHost.vue's own onMounted loads a tab's connection's mask rules
  // unconditionally (so folding is ready the instant the preview is toggled on) — a spec with no
  // maskRulesList snapshot of its own gets "no rules configured", the correct default for every
  // connection this chapter's own fixtures predate, not a fixture miss.
  [IPC.maskRulesList]: '[]',
  [IPC.maskRulesCounts]: '{}',
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
  // P520: nothing pending unless a spec pushes the queue (pushApprovals).
  [IPC.approvalsList]: '[]',
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
  // initAppUpdate() polls this unconditionally right after mount, on every boot — the same
  // no-committed-fixture-will-ever-snapshot-this reasoning as dbMcpStatus above. "no update" is
  // the honest default for a dev-server run under Playwright, which never reports a tagged release
  // version.
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
  // P85: main.ts's bootstrap() joins hydrateCustomScripts() to the same unconditional-every-boot
  // Promise.all as updateStatus above, same reasoning — a spec that never configures a
  // script gets "no scripts yet", not a fixture miss.
  [IPC.customScriptsList]:
    '{"dir":"/home/test/runs","collections":[],"scripts":[],"problems":[],"secretStorage":{"available":true,"backend":"keychain","insecureFallback":false,"reason":null}}',
  // main.ts's bootstrap() joins hydrateTerminalDefaults() to the same unconditional-every-boot
  // Promise.all, same reasoning — a spec that never cares about the Automations module's own
  // default cwd gets a plausible home directory, not a fixture miss. A spec that DOES care
  // (terminal- module.spec.ts's own unscoped-launch case) still wins with its own snapshot.
  [IPC.terminalDefaultCwd]: JSON.stringify({ path: '/home/test' }),
  // main.ts's bootstrap() joins initKeepAwake() to the same unconditional-every-boot
  // Promise.all above, same reasoning. supported: true so the titlebar button renders in every
  // spec — the UI suite runs against a static server, not a real Go build, and a spec that never
  // cares about keep-awake should still see the titlebar it will ship with. A spec that DOES care
  // (workbench.spec.ts's own keep-awake cases) still wins with its own snapshot.
  [IPC.keepAwakeStatus]: JSON.stringify({ manual: false, supported: true, error: '' }),
});

// Structured clone (what ipcRenderer.invoke used) preserves a key whose value
// is `undefined`; the wire format here is JSON, which drops it outright — both are normalised the
// same way before comparing, so a fixture recorded from either transport matches. `tabId` is
// excluded outright — a per-tab UUID the renderer generates at tab-open time, never reproducible
// from a fixture (same reasoning mockPort.ts's own matchKey already applies). `refresh` defaults
// to `false` at every call site (control.ts's own `refresh ?? false`), so a live call always
// carries it explicitly — but a captured fixture snapshot can predate that default becoming part
// of the args shape and omit the key; dropping an explicit `false` too means the two
// compare equal, the same way an absent key already does.
const CANONICAL_OPTIONS = {
  excludeKeys: ['tabId'],
  dropKey: (key: string, value: unknown) => key === 'refresh' && value === false,
};

// The mode a fresh boot answers windowsEnsure with, when a spec provides no explicit
// snapshot for it — read the (mode-independent) tabsList snapshot the spec *did* configure, so a
// spec restoring an Api-mode tab as active boots displaying it. Only ever consults the first
// tabsList snapshot (a spec keying tabsList by args to answer differently per window is not a shape
// any spec uses today) and only its args-less/single form — args-matched multi-snapshot tabsList
// specs fall back to 'studio', same as providing no tabsList snapshot at all.
function inferredBootMode(byChannel: ReadonlyMap<string, ControlSnapshot[]>): string {
  const tabsSnaps = byChannel.get(IPC.tabsList);
  const tabs = (tabsSnaps?.length === 1 ? tabsSnaps[0].response : undefined) as
    | { kind?: string; active?: boolean }[]
    | undefined;
  if (!tabs || tabs.length === 0) return 'studio';
  const bootTab = tabs.find((t) => t.active) ?? tabs[0];
  const kind = bootTab?.kind;
  return kind && kind in STUDIO_TAB_KIND_MODE
    ? STUDIO_TAB_KIND_MODE[kind as StudioTabKind]
    : 'studio';
}

/**
 * Replaces the control channel's answers at the network layer — `page.route`
 * intercepts every request under `/wails/`: the real runtime bundle itself (served for real, off
 * disk) and the one RPC endpoint bound calls POST to. Unlike the
 * `tests/ipc/support/mockControl.ts` (which sat behind a real `contextBridge`/`ipcRenderer.invoke`
 * inside an Electron main process), this sits behind nothing — the mocked HTTP response is exactly
 * what `unwrap`/`trust` in `bridge/control.ts` are written to consume, so a frontend spec still
 * exercises that code for real.
 */
export async function installControlMocks(
  page: Page,
  snapshots: readonly ControlSnapshot[],
): ReturnType<typeof sharedInstallControlMocks> {
  return sharedInstallControlMocks(page, snapshots, {
    fqnToChannel: FQN_TO_CHANNEL,
    wildcardDefaults: WILDCARD_DEFAULTS,
    runtimeJsPath: WAILS_RUNTIME_JS,
    canonicalOptions: CANONICAL_OPTIONS,
    // `refresh:true` never appears in a captured fixture (the write-mode capture always reads
    // `refresh:false` first, the same discipline tests/ipc/support/harness.ts's own cache-aside
    // stand-in follows) — a real Wails backend still answers it with the same data a
    // `refresh:false` read would, since nothing in a static fixture's world ever changes between
    // the two.
    resolveSnapshotFallback: (_channel, args, findSnap) => {
      if (args && typeof args === 'object' && (args as { refresh?: unknown }).refresh === true) {
        return findSnap({ ...args, refresh: false });
      }
      return undefined;
    },
    // See `inferredBootMode`'s own doc comment above.
    // opsCancel (never snapshotted) releases the stream replies held by `untilCancel` snapshots.
    // The hook gets no args, so every held reply is released; a spec asserts the cancelled op id
    // from `control.log()`.
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
      return channel === IPC.windowsEnsure
        ? JSON.stringify({ mode: inferredBootMode(byChannel) })
        : undefined;
    },
  });
}

export const emitWailsEvent = sharedEmitWailsEvent;
