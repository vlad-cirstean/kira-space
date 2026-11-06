import * as AppService from '@bindings/appservice.js';
import * as ConnectionConfigService from '@bindings/connectionconfigservice.js';
import * as ConnectionsService from '@bindings/connectionsservice.js';
import * as DataGripService from '@bindings/datagripservice.js';
import * as DbMcpService from '@bindings/dbmcpservice.js';
import * as FilesService from '@bindings/filesservice.js';
import * as FiltersService from '@bindings/filtersservice.js';
import * as KibanaService from '@bindings/kibanaservice.js';
import * as MaskRulesService from '@bindings/maskrulesservice.js';
import type * as WailsModels from '@bindings/models.js';
import * as ProofService from '@bindings/proofservice.js';
import * as QueriesService from '@bindings/queriesservice.js';
import * as SchemaService from '@bindings/schemaservice.js';
import * as TreeService from '@bindings/treeservice.js';
import type * as DataGripModels from '@bindings-internal/datagrip/models.js';
import type {
  ConnectionInput,
  ConnectionState,
  ConnectionSummary,
} from '@shared/domain/connection';
import type {
  ConnectionConfigView,
  ConnectionFileDetail,
  ConnectionFormResult,
  ConnectionSecretKind,
} from '@shared/domain/connectionConfig';
import type { DataGripPreview, DataGripReport } from '@shared/domain/datagrip';
import type {
  DbMcpInstallResult,
  DbMcpPendingResponse,
  DbMcpStatus,
  DbMcpTablesListing,
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
import type { MaskRule, MaskRuleFields } from '@shared/domain/mask';
import type { MaskReviewListing, MaskReviewPending } from '@shared/domain/maskReview';
import type {
  ProofChanged,
  ProofDetail,
  ProofOpenResult,
  ProofSavePngResult,
  ProofSavePngToResult,
  ProofSelect,
  ProofShotFolderResult,
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
import type { SecretStorageStatus } from '@shared/domain/secrets';
import type { ObjectMeta, RelationColumns, TreeNode } from '@shared/domain/tree';
import type { TreeVisibility } from '@shared/domain/treeFilter';
import {
  CHANNEL,
  type FilterHistoryChangedEvent,
  type MaskRulesChangedEvent,
  type SavedQueriesChangedEvent,
} from '@shared/protocol/events';
import { logWarn } from '@workbench/bridge/log';
import { on, trust, unwrap, windowKey } from '@workbench/bridge/rpc';
import type { FormEdit } from '@workbench/config/configForm';
import { control as hubControl } from '../../bridge';
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
  // Writes the username into the connection's file and seals the password; a nil field is kept.
  // Go reconnects a live connection.
  connectionsUpdateCredentials: (
    id: string,
    credentials: { username?: string; password?: string },
  ): Promise<void> =>
    unwrap(
      ConnectionsService.UpdateCredentials({
        id,
        username: credentials.username ?? null,
        password: credentials.password ?? null,
      }),
    ),
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
  // Tests the saved file. The generated TestResult's serverVersion/error are
  // `string | null | undefined`; normalized here to `string | undefined`.
  connectionsTest: (
    id: string,
  ): Promise<{
    ok: boolean;
    serverVersion?: string;
    error?: string;
  }> =>
    unwrap<Awaited<ReturnType<typeof ConnectionsService.Test>>>(
      ConnectionsService.Test({ id }),
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

  // The connection files' status and text, and the secret names they reference; SetSecret is
  // write-only, no call returns a value.
  connectionConfigView: (): Promise<ConnectionConfigView> =>
    unwrap(ConnectionConfigService.View()).then((r) => trust<ConnectionConfigView>(r)),
  connectionConfigFile: (file: string): Promise<ConnectionFileDetail> =>
    unwrap(ConnectionConfigService.File({ file })).then((r) => trust<ConnectionFileDetail>(r)),
  connectionConfigSaveFile: (
    file: string,
    text: string,
    baseHash: string,
  ): Promise<ConnectionFileDetail> =>
    unwrap(ConnectionConfigService.SaveFile({ file, text, baseHash })).then((r) =>
      trust<ConnectionFileDetail>(r),
    ),
  connectionConfigForm: (
    file: string,
    text: string,
    edits: FormEdit[],
  ): Promise<ConnectionFormResult> =>
    unwrap(
      ConnectionConfigService.Form({
        file,
        text,
        edits: edits.map((e) => ({ path: e.path, value: e.value ?? null, unset: e.unset })),
      }),
    ).then((r) => trust<ConnectionFormResult>(r)),
  connectionConfigDeleteFile: (file: string): Promise<void> =>
    unwrap(ConnectionConfigService.DeleteFile({ file })),
  connectionConfigSetSecret: (
    kind: ConnectionSecretKind,
    name: string,
    value: string,
  ): Promise<ConnectionConfigView> =>
    unwrap(ConnectionConfigService.SetSecret({ kind, name, value })).then((r) =>
      trust<ConnectionConfigView>(r),
    ),
  connectionConfigDeleteSecret: (
    kind: ConnectionSecretKind,
    name: string,
  ): Promise<ConnectionConfigView> =>
    unwrap(ConnectionConfigService.DeleteSecret({ kind, name })).then((r) =>
      trust<ConnectionConfigView>(r),
    ),
  onConnectionConfigChanged: (cb: () => void): (() => void) =>
    on(CHANNEL.connectionConfigChanged, cb),
  onConnectionFiltersChanged: (
    cb: (event: { connectionId: string; visibility: TreeVisibility }) => void,
  ): (() => void) => on(CHANNEL.connectionFiltersChanged, cb),

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
  dbMcpTables: (id: string): Promise<DbMcpTablesListing> =>
    unwrap(DbMcpService.Tables({ id })).then((r) => trust<DbMcpTablesListing>(r)),
  maskReviewColumns: (id: string, refresh: boolean): Promise<MaskReviewListing> =>
    unwrap(MaskRulesService.ReviewColumns({ id, refresh })).then((r) =>
      trust<MaskReviewListing>(r),
    ),
  maskReviewPending: (id: string): Promise<MaskReviewPending> =>
    unwrap(MaskRulesService.ReviewPending({ id })).then((r) => trust<MaskReviewPending>(r)),
  dbMcpPendingResponse: (requestId: string): Promise<DbMcpPendingResponse> =>
    unwrap(DbMcpService.PendingResponse({ requestId })).then((r) => trust<DbMcpPendingResponse>(r)),

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
  proofChooseShotFolder: (sessionName: string): Promise<ProofShotFolderResult> =>
    unwrap(ProofService.ChooseShotFolder({ sessionName })).then((r) =>
      trust<ProofShotFolderResult>(r),
    ),
  proofSavePngTo: (
    folderId: string,
    filename: string,
    pngBase64: string,
  ): Promise<ProofSavePngToResult> =>
    unwrap(ProofService.SavePngTo({ folderId, filename, pngBase64 })).then((r) =>
      trust<ProofSavePngToResult>(r),
    ),
  proofForgetShotFolder: (folderId: string): Promise<void> =>
    unwrap(ProofService.ForgetShotFolder({ folderId })),
  onProofChanged: (cb: (event: ProofChanged) => void): (() => void) => on(CHANNEL.proofChanged, cb),
  onProofSelect: (cb: (event: ProofSelect) => void): (() => void) => on(CHANNEL.proofSelect, cb),

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
  kibanaDataViews: (connectionId: string): Promise<KibanaDataView[]> =>
    unwrap(KibanaService.DataViews({ connectionId, opId: '' })).then((r) =>
      kibanaDataViewSchema.array().parse(r ?? []),
    ),
  kibanaDataView: (connectionId: string, id: string): Promise<KibanaDataView> =>
    unwrap(KibanaService.DataView({ connectionId, opId: '', id })).then((r) =>
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
};

// Studio's control: the hub's shared and Space methods, with the Studio-bound ones layered on top.
export const control = {
  ...hubControl,
  ...apiControl,
  ...studioControl,
};
