import * as AgentService from '@bindings/agentservice.js';
import * as AppService from '@bindings/appservice.js';
import * as ConnectionsService from '@bindings/connectionsservice.js';
import * as CustomScriptsService from '@bindings/customscriptsservice.js';
import * as DataGripService from '@bindings/datagripservice.js';
import * as DbMcpService from '@bindings/dbmcpservice.js';
import * as FilesService from '@bindings/filesservice.js';
import * as FiltersService from '@bindings/filtersservice.js';
import * as KeepAwakeService from '@bindings/keepawakeservice.js';
import * as KibanaService from '@bindings/kibanaservice.js';
import * as LayoutService from '@bindings/layoutservice.js';
import * as LifecycleService from '@bindings/lifecycleservice.js';
import * as LinkService from '@bindings/linkservice.js';
import * as LlmPromptsService from '@bindings/llmpromptsservice.js';
import * as MaskRulesService from '@bindings/maskrulesservice.js';
import type * as WailsModels from '@bindings/models.js';
import * as OpsService from '@bindings/opsservice.js';
import * as PromptsService from '@bindings/promptsservice.js';
import * as ProofService from '@bindings/proofservice.js';
import * as QueriesService from '@bindings/queriesservice.js';
import * as SchemaService from '@bindings/schemaservice.js';
import * as ScriptRunsService from '@bindings/scriptrunsservice.js';
import * as SettingsService from '@bindings/settingsservice.js';
import * as TabsService from '@bindings/tabsservice.js';
import * as TerminalService from '@bindings/terminalservice.js';
import * as TreeService from '@bindings/treeservice.js';
import * as UpdateService from '@bindings/updateservice.js';
import * as WindowsService from '@bindings/windowsservice.js';
import type * as DataGripModels from '@bindings-internal/datagrip/models.js';
import type {
  ConnectionInput,
  ConnectionState,
  ConnectionSummary,
} from '@shared/domain/connection';
import type { DataGripPreview, DataGripReport } from '@shared/domain/datagrip';
import type {
  DbMcpApprovalSnapshot,
  DbMcpInstallResult,
  DbMcpPendingResponse,
  DbMcpStatus,
} from '@shared/domain/dbmcp';
import type { ObjectDefinition } from '@shared/domain/definition';
import {
  type KibanaDataView,
  type KibanaDiscoverBody,
  type KibanaDiscoverRequest,
  type KibanaHistogram,
  type KibanaInterval,
  type KibanaSuggestions,
  kibanaDataViewSchema,
  kibanaHistogramSchema,
  kibanaSuggestionsSchema,
} from '@shared/domain/kibana';
import type { Layout } from '@shared/domain/layout';
import type { MaskRule, MaskRuleFields } from '@shared/domain/mask';
import type { AppMode } from '@shared/domain/mode';
import type { OpRecord } from '@shared/domain/ops';
import type {
  ProofChanged,
  ProofDetail,
  ProofOpenResult,
  ProofSavePngResult,
  ProofSelect,
  ProofStepDetail,
  ProofSummary,
  ProofViewState,
  ProofViewStateEntry,
  ProofWindowTarget,
} from '@shared/domain/proof';
import {
  type ConsoleBody,
  type FilterBody,
  type FilterHistoryEntry,
  type SavedConsoleQuery,
  type SavedDiscoverQuery,
  type SavedFilterQuery,
  type SavedQuery,
  type SortSpec,
  savedDiscoverQuerySchema,
} from '@shared/domain/queries';
import type { ConnectionDdl } from '@shared/domain/schema';
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
import type { SecretStorageStatus } from '@shared/domain/secrets';
import type { ObjectMeta, RelationColumns, TreeNode } from '@shared/domain/tree';
import type { TreeVisibility } from '@shared/domain/treeFilter';
import {
  CHANNEL,
  type FilterHistoryChangedEvent,
  type MaskRulesChangedEvent,
  type SavedQueriesChangedEvent,
} from '@shared/protocol/events';
import { scriptsSnapshotOf } from '@workbench/automations/createCustomScriptsStore';
import { createCoreControl } from '@workbench/bridge/createCoreControl';
import { logWarn } from '@workbench/bridge/log';
import { createOpenExternal } from '@workbench/bridge/openExternal';
import { on, trust, unwrap, windowKey } from '@workbench/bridge/rpc';
import type { Settings, SettingsPatch } from '../state/settingsDomain';
import type { TabRecord } from '../state/tabDomain';
import { apiControl } from './apiControl';

// bridge/index.ts is the composition root: the only file that imports
// every part of the app's own bound-call surface — the methods moved into
// createCoreControl.ts (shared verbatim with Kira Space's own copy of this file), Studio's own
// remaining methods, defined right here, and the Api module's 40 (apiControl.ts) — and combines
// them into the one `control` object every other file in the app imports. None of the three
// depends on either of the others; all three ultimately depend only on rpc.ts's shared
// on/trust/unwrap/windowKey. control.ts itself is now a thin re-export of this module, so every
// existing `import { control } from '.../bridge/control'` call site (~200 of them) is unchanged.
const studioControl = {
  appInfo: (): Promise<WailsModels.AppInfo> => unwrap(AppService.Info()),
  onNewConnection: (cb: () => void): (() => void) => on(CHANNEL.newConnection, cb),
  onNewRequest: (cb: () => void): (() => void) => on(CHANNEL.newRequest, cb),
  onImportPostman: (cb: () => void): (() => void) => on(CHANNEL.importPostman, cb),
  onImportDataGrip: (cb: () => void): (() => void) => on(CHANNEL.importDataGrip, cb),
  onCommandPalette: (cb: () => void): (() => void) => on(CHANNEL.commandPalette, cb),
  onViewFind: (cb: () => void): (() => void) => on(CHANNEL.viewFind, cb),
  onViewRefresh: (cb: () => void): (() => void) => on(CHANNEL.viewRefresh, cb),
  onViewRun: (cb: () => void): (() => void) => on(CHANNEL.viewRun, cb),
  onViewRunAll: (cb: () => void): (() => void) => on(CHANNEL.viewRunAll, cb),
  onViewFormat: (cb: () => void): (() => void) => on(CHANNEL.viewFormat, cb),

  filesChooseSave: (defaultName: string): Promise<WailsModels.FilesChooseSaveResult> =>
    unwrap(FilesService.ChooseSave({ defaultName })),
  filesChooseOpen: (
    args?: WailsModels.FilesChooseOpenArgs,
  ): Promise<WailsModels.FilesChooseOpenResult> => unwrap(FilesService.ChooseOpen(args ?? {})),
  filesChooseOpenMany: (
    args?: WailsModels.FilesChooseOpenArgs,
  ): Promise<WailsModels.FilesChooseOpenManyResult> =>
    unwrap(FilesService.ChooseOpenMany(args ?? {})),

  connectionsList: (): Promise<ConnectionSummary[]> =>
    unwrap(ConnectionsService.List()).then((r) => trust<ConnectionSummary[]>(r ?? [])),
  connectionsReorder: (ids: string[]): Promise<ConnectionSummary[]> =>
    unwrap(ConnectionsService.Reorder({ ids })).then((r) => trust<ConnectionSummary[]>(r ?? [])),
  connectionsCreate: (input: ConnectionInput): Promise<ConnectionSummary> =>
    unwrap(ConnectionsService.Create(input)).then((r) => trust<ConnectionSummary>(r)),
  connectionsUpdate: (id: string, input: ConnectionInput): Promise<ConnectionSummary> =>
    unwrap(ConnectionsService.Update({ id, input })).then((r) => trust<ConnectionSummary>(r)),
  connectionsDuplicate: (id: string): Promise<ConnectionSummary> =>
    unwrap(ConnectionsService.Duplicate({ id })).then((r) => trust<ConnectionSummary>(r)),
  // Go-side name is Remove, not Delete (P57 §1.9) — read the .go file, do not assume lowerCamel.
  connectionsDelete: (id: string): Promise<void> => unwrap(ConnectionsService.Delete({ id })),
  // P14 D6: outcome is one of revealed | cancelled | confirmation-required | error — confirmed is
  // honoured by the backend only on the confirmation-required path (a Mac where LocalAuthentication
  // works ignores it entirely).
  connectionsReveal: (
    id: string,
    confirmed: boolean,
  ): Promise<{
    password: string | null;
    uri: string | null;
    error: string | null;
    outcome: string;
  }> => unwrap(ConnectionsService.Reveal({ id, confirmed })),
  // The generated TestResult's serverVersion/error are `string | null | undefined`; the
  // shape was `string | undefined` only (no null) — normalized here rather than pushed onto
  // ConnectionDialog.vue, which assigns straight into its own `?: string` reactive state.
  // id is the dialog's editingId (empty for a brand-new connection) — the backend fills in
  // the stored secret server-side when the draft carries none, so Test on an existing connection
  // whose password field was never revealed still probes with the real credential.
  connectionsTest: (
    input: ConnectionInput,
    id: string,
  ): Promise<{
    ok: boolean;
    serverVersion?: string;
    error?: string;
  }> =>
    unwrap<Awaited<ReturnType<typeof ConnectionsService.Test>>>(
      ConnectionsService.Test({ input, id }),
    ).then((r) => ({
      ok: r.ok,
      serverVersion: r.serverVersion ?? undefined,
      error: r.error ?? undefined,
    })),
  connectionsConnect: (id: string): Promise<ConnectionState> =>
    unwrap(ConnectionsService.Connect({ id })).then((r) => trust<ConnectionState>(r)),
  connectionsDisconnect: (id: string): Promise<ConnectionState> =>
    unwrap(ConnectionsService.Disconnect({ id })).then((r) => trust<ConnectionState>(r)),
  connectionsStates: (): Promise<ConnectionState[]> =>
    unwrap(ConnectionsService.States()).then((r) => trust<ConnectionState[]>(r ?? [])),
  connectionsSecretsStatus: (): Promise<SecretStorageStatus> =>
    unwrap(ConnectionsService.SecretsStatus()).then((r) => trust<SecretStorageStatus>(r)),

  // M5 §7.2/§9: the Privacy tab, the grid header menu, and the grid preview's own local tag
  // computation all share these — see state/maskRules.ts for the one store that wraps them.
  maskRulesList: (connectionId: string): Promise<MaskRule[]> =>
    unwrap(MaskRulesService.List({ connectionId })).then((r) => trust<MaskRule[]>(r ?? [])),
  // The generated MaskRuleFields.kind is a TS `enum` (wails3's binding generator turns Go's named
  // MaskKind consts into one, unlike this app's other string-const-typed fields, which stay plain
  // `string`) — a plain string-literal object is not structurally assignable to it, so this widens
  // at the call boundary the same documented way `trust` narrows on the way out (rpc.ts's own
  // comment): the Go value is always one of the valid members regardless of which TS shape names it.
  maskRulesUpsert: (connectionId: string, fields: MaskRuleFields): Promise<MaskRule> =>
    unwrap(
      MaskRulesService.Upsert({
        connectionId,
        fields,
      } as unknown as Parameters<typeof MaskRulesService.Upsert>[0]),
    ).then((r) => trust<MaskRule>(r)),
  maskRulesDelete: (id: string): Promise<void> => unwrap(MaskRulesService.Delete({ id })),
  maskRulesRegenerateKey: (connectionId: string): Promise<void> =>
    unwrap(MaskRulesService.RegenerateKey({ connectionId })),
  maskRulesCounts: (): Promise<Record<string, number>> =>
    unwrap(MaskRulesService.Counts()).then((r) => trust<Record<string, number>>(r ?? {})),
  // §6.3: the raw correlation key, hex-encoded — the one seam that sends it to the renderer, for
  // the grid preview's own local tag computation. "" means the connection needs no key.
  maskRulesCorrelationKey: (connectionId: string): Promise<string> =>
    unwrap(MaskRulesService.CorrelationKey({ connectionId })),
  // P108 Part 12 F18: schemaChanged's own shape — state/maskRules.ts's initMaskRulesSync is the
  // one subscriber, so a rule changed in one window's Privacy tab/header menu reaches every other
  // window's own query cache instead of staying stale until something else happens to refetch it.
  onMaskRulesChanged: (cb: (event: MaskRulesChangedEvent) => void): (() => void) =>
    on(CHANNEL.maskRulesChanged, cb),
  onSavedQueriesChanged: (cb: (event: SavedQueriesChangedEvent) => void): (() => void) =>
    on(CHANNEL.savedQueriesChanged, cb),
  onFilterHistoryChanged: (cb: (event: FilterHistoryChangedEvent) => void): (() => void) =>
    on(CHANNEL.filterHistoryChanged, cb),
  onConnectionState: (cb: (state: ConnectionState) => void): (() => void) =>
    on(CHANNEL.connectionState, cb),
  onConnectionMetadataInvalidated: (cb: (connectionId: string) => void): (() => void) =>
    on(CHANNEL.connectionMetadataInvalidated, cb),
  onConnectionsChanged: (cb: (records: ConnectionSummary[]) => void): (() => void) =>
    on(CHANNEL.connectionsChanged, cb),

  // P25 D9: only the project path crosses the bridge either way — Scan never touches a
  // credential store (file reads only), Import fetches and decrypts a password only inside Go,
  // and neither ever returns one to the renderer.
  datagripScan: (path: string): Promise<DataGripPreview> =>
    unwrap<DataGripModels.Preview>(DataGripService.Scan({ path })).then((r) =>
      trust<DataGripPreview>({
        ...r,
        rows: (r.rows ?? []).map((row) => ({ ...row, warnings: row.warnings ?? [] })),
      }),
    ),
  datagripImport: (path: string, selectedUuids: string[]): Promise<DataGripReport> =>
    unwrap<DataGripModels.Report>(DataGripService.Import({ path, selectedUuids })).then((r) =>
      trust<DataGripReport>({ ...r, rows: r.rows ?? [] }),
    ),

  treeChildren: (
    connectionId: string,
    path: string,
    refresh?: boolean,
  ): Promise<{ nodes: TreeNode[]; source: 'cache' | 'server'; truncated: boolean }> =>
    unwrap<Awaited<ReturnType<typeof TreeService.Children>>>(
      TreeService.Children({ connectionId, path, refresh: refresh ?? false }),
    ).then((r) =>
      trust<{ nodes: TreeNode[]; source: 'cache' | 'server'; truncated: boolean }>({
        ...r,
        nodes: r.nodes ?? [],
      }),
    ),
  // P63 §4.3: a batch of paths, answered with each one's engine-level type, in order — Redis
  // only (Caps().keyTypes gates every call site; every other adapter would just throw).
  treeKeyTypes: (connectionId: string, paths: string[]): Promise<string[]> =>
    unwrap<Awaited<ReturnType<typeof TreeService.KeyTypes>>>(
      TreeService.KeyTypes({ connectionId, paths }),
    ).then((r) => trust<string[]>(r ?? [])),
  treeDescribe: (
    connectionId: string,
    path: string,
    refresh?: boolean,
    tabId?: string,
  ): Promise<{ meta: ObjectMeta; source: 'cache' | 'server' }> =>
    unwrap(
      TreeService.Describe({
        connectionId,
        path,
        refresh: refresh ?? false,
        tabId: tabId ?? null,
      }),
    ).then((r) => trust<{ meta: ObjectMeta; source: 'cache' | 'server' }>(r)),
  treeDefinition: (
    connectionId: string,
    path: string,
    refresh?: boolean,
    tabId?: string,
  ): Promise<{ definition: ObjectDefinition; source: 'cache' | 'server' }> =>
    unwrap(
      TreeService.Definition({
        connectionId,
        path,
        refresh: refresh ?? false,
        tabId: tabId ?? null,
      }),
    ).then((r) => trust<{ definition: ObjectDefinition; source: 'cache' | 'server' }>(r)),
  // P22c D3: SchemaColumns' own bridge call — args reuse Describe/Definition's own four-field
  // shape (tabId unused: a schema-wide fetch is not tagged to one tab's op-log row).
  treeSchemaColumns: (
    connectionId: string,
    path: string,
    refresh?: boolean,
  ): Promise<{ relations: RelationColumns[]; source: 'cache' | 'server' }> =>
    unwrap<Awaited<ReturnType<typeof TreeService.SchemaColumns>>>(
      TreeService.SchemaColumns({
        connectionId,
        path,
        refresh: refresh ?? false,
        tabId: null,
      }),
    ).then((r) =>
      trust<{ relations: RelationColumns[]; source: 'cache' | 'server' }>({
        ...r,
        relations: r.relations ?? [],
      }),
    ),
  treeInvalidate: (connectionId: string, path?: string): Promise<void> =>
    unwrap(TreeService.Invalidate({ connectionId, path: path ?? null })),

  filtersList: (connectionId: string): Promise<TreeVisibility> =>
    unwrap(FiltersService.List({ connectionId })).then((r) => trust<TreeVisibility>(r)),
  filtersReplace: (connectionId: string, visibility: TreeVisibility): Promise<TreeVisibility> =>
    unwrap(FiltersService.Replace({ connectionId, visibility })).then((r) =>
      trust<TreeVisibility>(r),
    ),

  dbMcpStatus: (): Promise<DbMcpStatus> =>
    unwrap(DbMcpService.Status()).then((r) => trust<DbMcpStatus>(r)),
  dbMcpSetEnabled: (enabled: boolean): Promise<DbMcpStatus> =>
    unwrap(DbMcpService.SetEnabled({ enabled })).then((r) => trust<DbMcpStatus>(r)),
  dbMcpRegenerate: (): Promise<DbMcpStatus> =>
    unwrap(DbMcpService.Regenerate()).then((r) => trust<DbMcpStatus>(r)),
  dbMcpInstallAgent: (): Promise<DbMcpInstallResult> =>
    unwrap(DbMcpService.InstallAgent()).then((r) => trust<DbMcpInstallResult>(r)),
  dbMcpPendingApprovals: (): Promise<DbMcpApprovalSnapshot> =>
    unwrap(DbMcpService.PendingApprovals()).then((r) => trust<DbMcpApprovalSnapshot>(r)),
  dbMcpPendingResponse: (requestId: string): Promise<DbMcpPendingResponse> =>
    unwrap(DbMcpService.PendingResponse({ requestId })).then((r) => trust<DbMcpPendingResponse>(r)),
  dbMcpApproveQuery: (requestId: string): Promise<DbMcpApprovalSnapshot> =>
    unwrap(DbMcpService.ApproveQuery({ requestId })).then((r) => trust<DbMcpApprovalSnapshot>(r)),
  dbMcpDenyQuery: (requestId: string): Promise<DbMcpApprovalSnapshot> =>
    unwrap(DbMcpService.DenyQuery({ requestId })).then((r) => trust<DbMcpApprovalSnapshot>(r)),
  onDbMcpApprovalChanged: (cb: (snap: DbMcpApprovalSnapshot) => void): (() => void) =>
    on(CHANNEL.dbMcpApproval, cb),

  proofSessions: (): Promise<ProofSummary[]> =>
    unwrap(ProofService.Sessions()).then((r) => trust<ProofSummary[]>(r ?? [])),
  proofSession: (sessionId: string): Promise<ProofDetail> =>
    unwrap(ProofService.Session({ sessionId })).then((r) => trust<ProofDetail>(r)),
  proofStep: (sessionId: string, rev: number, ordinal: number): Promise<ProofStepDetail> =>
    unwrap(ProofService.Step({ sessionId, rev, ordinal })).then((r) => trust<ProofStepDetail>(r)),
  proofViewState: (sessionId: string, rev: number): Promise<ProofViewStateEntry[]> =>
    unwrap(ProofService.ViewState({ sessionId, rev })).then((r) =>
      trust<ProofViewStateEntry[]>(r ?? []),
    ),
  proofSetViewState: (
    sessionId: string,
    rev: number,
    ordinal: number,
    state: ProofViewState,
  ): Promise<void> => unwrap(ProofService.SetViewState({ sessionId, rev, ordinal, state })),
  proofAcceptEdit: (sessionId: string): Promise<void> =>
    unwrap(ProofService.AcceptEdit({ sessionId })),
  proofRejectEdit: (sessionId: string): Promise<void> =>
    unwrap(ProofService.RejectEdit({ sessionId })),
  proofDelete: (sessionId: string): Promise<void> => unwrap(ProofService.Delete({ sessionId })),
  proofMarkSeen: (sessionId: string): Promise<void> => unwrap(ProofService.MarkSeen({ sessionId })),
  proofOpenWindow: (sessionId?: string): Promise<ProofOpenResult> =>
    unwrap(ProofService.OpenWindow({ sessionId })).then((r) => trust<ProofOpenResult>(r)),
  proofWindowTarget: (): Promise<ProofWindowTarget> =>
    unwrap(ProofService.WindowTarget({ key: windowKey })).then((r) => trust<ProofWindowTarget>(r)),
  proofSavePng: (filename: string, pngBase64: string): Promise<ProofSavePngResult> =>
    unwrap(ProofService.SavePng({ filename, pngBase64 })).then((r) => trust<ProofSavePngResult>(r)),
  onProofChanged: (cb: (event: ProofChanged) => void): (() => void) => on(CHANNEL.proofChanged, cb),
  onProofSelect: (cb: (event: ProofSelect) => void): (() => void) => on(CHANNEL.proofSelect, cb),

  opsRecent: (limit: number): Promise<OpRecord[]> =>
    unwrap(OpsService.Recent({ limit })).then((r) => trust<OpRecord[]>(r ?? [])),
  opsCancel: (opId: string): Promise<void> => unwrap(OpsService.Cancel({ opId })),
  onOpUpdate: (cb: (record: OpRecord) => void): (() => void) => on(CHANNEL.opUpdate, cb),

  // Go's SavedQuery is one flat struct with `kind: string` and `body: json.RawMessage` (typed
  // `any` in the bindings) rather than the domain's real discriminated union — Go has no sum
  // types, so the polymorphic body is opaque JSON on the wire and the discriminant is a plain
  // string. `trust` narrows both at once, on the same "the server always writes a valid shape"
  // assumption the Electron build made implicitly.
  queriesList: (connectionId: string, path: string): Promise<SavedFilterQuery[]> =>
    unwrap(QueriesService.List({ connectionId, path })).then((r) =>
      trust<SavedFilterQuery[]>(r ?? []),
    ),
  queriesSave: (args: {
    connectionId: string;
    path: string;
    name: string;
    body: FilterBody;
    pinned: boolean;
  }): Promise<SavedFilterQuery> =>
    unwrap(QueriesService.Save(args)).then((r) => trust<SavedFilterQuery>(r)),
  queriesListConsole: (connectionId: string, path: string): Promise<SavedConsoleQuery[]> =>
    unwrap(QueriesService.ListConsole({ connectionId, path })).then((r) =>
      trust<SavedConsoleQuery[]>(r ?? []),
    ),
  queriesSaveConsole: (args: {
    connectionId: string;
    path: string;
    name: string;
    body: ConsoleBody;
    pinned: boolean;
  }): Promise<SavedConsoleQuery> =>
    unwrap(QueriesService.SaveConsole(args)).then((r) => trust<SavedConsoleQuery>(r)),
  queriesListDiscover: (connectionId: string): Promise<SavedDiscoverQuery[]> =>
    unwrap(QueriesService.ListDiscover({ connectionId })).then((rows) =>
      trust<unknown[]>(rows ?? []).flatMap((row) => {
        const parsed = savedDiscoverQuerySchema.safeParse(row);
        if (parsed.success) return [parsed.data];
        logWarn('queries', 'dropping saved Discover query that does not parse', parsed.error);
        return [];
      }),
    ),
  queriesSaveDiscover: (args: {
    connectionId: string;
    name: string;
    body: KibanaDiscoverBody;
    pinned: boolean;
  }): Promise<SavedDiscoverQuery> =>
    unwrap(QueriesService.SaveDiscover(args)).then((r) => savedDiscoverQuerySchema.parse(r)),
  queriesUpdateDiscover: (id: string, body: KibanaDiscoverBody): Promise<SavedDiscoverQuery> =>
    unwrap(QueriesService.UpdateDiscover({ id, body })).then((r) =>
      savedDiscoverQuerySchema.parse(r),
    ),
  queriesUpdate: (id: string, patch: { name?: string; pinned?: boolean }): Promise<SavedQuery> =>
    unwrap(
      QueriesService.Update({ id, name: patch.name ?? null, pinned: patch.pinned ?? null }),
    ).then((r) => trust<SavedQuery>(r)),
  queriesDelete: (id: string): Promise<void> => unwrap(QueriesService.Delete({ id })),
  queriesTouch: (id: string): Promise<void> => unwrap(QueriesService.Touch({ id })),
  queriesHistoryList: (
    connectionId: string,
    path: string,
    limit: number,
  ): Promise<FilterHistoryEntry[]> =>
    unwrap(QueriesService.HistoryList({ connectionId, path, limit })).then((r) =>
      trust<FilterHistoryEntry[]>(r ?? []),
    ),
  queriesHistoryRecord: (
    connectionId: string,
    path: string,
    where: string | null,
    orderBy: SortSpec | null,
  ): Promise<void> => unwrap(QueriesService.HistoryRecord({ connectionId, path, where, orderBy })),

  schemaGet: (connectionId: string): Promise<ConnectionDdl> =>
    unwrap(SchemaService.Get({ connectionId })).then((r) => trust<ConnectionDdl>(r)),
  schemaSet: (connectionId: string, ddl: string): Promise<ConnectionDdl> =>
    unwrap(SchemaService.Set({ connectionId, ddl })).then((r) => trust<ConnectionDdl>(r)),
  onSchemaChanged: (cb: (ddl: ConnectionDdl) => void): (() => void) =>
    on(CHANNEL.schemaChanged, cb),

  // P424: Discover reads beyond execute(). The wire shapes are zod-parsed, not trusted.
  kibanaDataViews: (connectionId: string, opId: string): Promise<KibanaDataView[]> =>
    unwrap(KibanaService.DataViews({ connectionId, opId })).then((r) =>
      kibanaDataViewSchema.array().parse(r ?? []),
    ),
  kibanaDataView: (connectionId: string, opId: string, id: string): Promise<KibanaDataView> =>
    unwrap(KibanaService.DataView({ connectionId, opId, id })).then((r) =>
      kibanaDataViewSchema.parse(r),
    ),
  kibanaHistogram: (args: {
    connectionId: string;
    opId: string;
    tabId: string;
    request: KibanaDiscoverRequest;
    interval: KibanaInterval;
  }): Promise<KibanaHistogram> =>
    unwrap(KibanaService.Histogram(args)).then((r) => kibanaHistogramSchema.parse(r)),
  kibanaSuggest: (args: {
    connectionId: string;
    opId: string;
    tabId: string;
    request: KibanaDiscoverRequest;
    field: string;
    prefix: string;
  }): Promise<KibanaSuggestions> =>
    unwrap(KibanaService.Suggest(args)).then((r) => kibanaSuggestionsSchema.parse(r)),

  // P85 §9.3: the Scripts settings section and the tab strip's own dropdown both go through
  // state/customScripts.ts, the one store that wraps these.
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
};

// One exported object, composed from the methods shared with Kira Space
// (createCoreControl.ts), Studio's own 94 remaining methods (studioControl, above) and the module's
// own 40 (apiControl.ts) — every one of the ~200 `control.xxx()` call sites in the app is
// unchanged, and mockRuntime.ts's channel map is unchanged, since neither the method names nor
// their bound-call FQNs moved. Spread order matters not at all here (the three objects' key sets
// are disjoint by construction).
export const control = {
  ...createCoreControl<Settings, Layout, TabRecord, SettingsPatch, AppMode>({
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
  openExternal: createOpenExternal((url) => unwrap(LinkService.OpenExternal({ url }))),
  ...apiControl,
  ...studioControl,
};
