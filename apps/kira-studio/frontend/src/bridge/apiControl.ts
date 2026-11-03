import * as ApiConfigService from '@bindings/apiconfigservice.js';
import * as CollectionsService from '@bindings/collectionsservice.js';
import * as GrpcHistoryService from '@bindings/grpchistoryservice.js';
import * as GrpcService from '@bindings/grpcservice.js';
import * as HttpService from '@bindings/httpservice.js';
import type * as WailsModels from '@bindings/models.js';
import * as ResponseHistoryService from '@bindings/responsehistoryservice.js';
import * as VariablesService from '@bindings/variablesservice.js';
// P4: model.SavedRequest is a storage-package type, not a bridge one — CollectionsService's own
// args/results reference it rather than restating it, so the renderer types against it through
// vite.config.ts's existing @bindings-internal alias.
import type * as WailsStorageModels from '@bindings-internal/storage/model/models.js';
import type { ApiConfigView } from '@shared/domain/apiConfig';
import type { ImportResult } from '@shared/domain/collections';
import type { PaletteColor } from '@shared/domain/color';
import type {
  GrpcCallEvent,
  GrpcCallResultWire,
  GrpcMetaPairWire,
  GrpcSchemaWire,
} from '@shared/domain/grpc';
import type { GrpcCallHistoryEntry, GrpcCallSnapshot } from '@shared/domain/grpcHistory';
import type {
  HttpBodyWire,
  HttpCookieWire,
  HttpHeaderWire,
  HttpRequestSettingsWire,
  HttpResponseWire,
} from '@shared/domain/http';
import type { ResponseHistoryEntry, ResponseHistorySnapshot } from '@shared/domain/responseHistory';
import type {
  ApiEnvironment,
  ApiVariable,
  ApiVariableHistoryEntry,
  RevealResult,
  VariableScope,
} from '@shared/domain/variables';
import type { ApiDataChangedEvent } from '@shared/protocol/events';
import { CHANNEL } from '@shared/protocol/events';
import { on, trust, unwrap, windowKey } from '@workbench/bridge/rpc';

// The module's own binding surface — the 39 of 106 methods whose prefix is httpSend, grpc*,
// onGrpcCall, collections*, variables*, history* or grpcHistory*. Every call site is unchanged
// (bridge/index.ts composes this into the one exported `control` object — that composition, and
// this file's own on/trust/unwrap/windowKey import, moved out of control.ts and into
// rpc.ts/index.ts so this file never imports control.ts, which composes it back in), and
// mockRuntime.ts's channel map is unchanged — this is a source-file split, not a binding-surface
// change.

// model.VariableScope is a named Go string type (unlike, say, model.CollectionItem.Kind, which is
// a plain `string`), so the generator emits a real TS `enum` for it — a plain 'collection' |
// 'environment' literal is not structurally assignable to that nominal type. The same documented
// widen-then-narrow `trust` already applies to every bound *result*; this is its one *argument*-
// side counterpart, needed only because this is the one bound call whose input carries a Go named
// enum type at all.
function scopeArg(scope: VariableScope): WailsStorageModels.VariableScope {
  return scope as unknown as WailsStorageModels.VariableScope;
}

export const apiControl = {
  // Runs through the same op scheduler/op log the DB adapters use (opId is
  // renderer-minted, exactly like every data-plane op's own beginOp) — never the webview's own
  // fetch (docs/ARCHITECTURE.md's "Go owns the network"). The generated Send drops the injected
  // ctx parameter from its TS signature — Wails still passes it through server-side, so a
  // window closing mid-request still aborts it. collectionId/environmentId name the scope
  // stage 2 (Go) resolves secrets against — both '' is valid (a scratch tab, no environment).
  // itemId travels alongside so Go can record a response-history entry under the right
  // scope — '' for a scratch tab, exactly like collectionId/environmentId's own "possibly empty"
  // shape above. Optional here: a missing field decodes as Go's zero value ("") on the wire.
  // incognito mirrors itemId's own optional-on-the-TS-side shape — a missing field
  // decodes as Go's zero value (false) on the wire.
  // options carries the per-request overrides of the seven api settings leaves — state.ts's
  // send() builds it from tab.state.settings with every null leaf dropped (buildSettingsWire), so
  // Go's Options only ever sees the leaves this request overrides.
  httpSend: (args: {
    opId: string;
    tabId: string;
    method: string;
    url: string;
    headers: HttpHeaderWire[];
    body: HttpBodyWire;
    collectionId: string;
    environmentId: string;
    itemId?: string;
    incognito?: boolean;
    options?: HttpRequestSettingsWire;
  }): Promise<HttpResponseWire> =>
    unwrap(
      HttpService.Send({
        ...args,
        itemId: args.itemId ?? '',
        incognito: args.incognito ?? false,
        options: args.options ?? {},
      }),
    ).then((r) => trust<HttpResponseWire>(r)),

  // P90 item 2: the Cookies tab's three bound calls — none of these go through the op scheduler
  // (HttpService.Cookies/DeleteCookie/ClearCookies, bridge/http.go), so no opId/tabId.
  httpCookies: (url: string): Promise<HttpCookieWire[]> =>
    unwrap(HttpService.Cookies({ url })).then((r) => trust<HttpCookieWire[]>(r ?? [])),
  httpDeleteCookie: (
    url: string,
    cookie: { name: string; domain: string; path: string },
  ): Promise<HttpCookieWire[]> =>
    unwrap(HttpService.DeleteCookie({ url, ...cookie })).then((r) =>
      trust<HttpCookieWire[]>(r ?? []),
    ),
  httpClearCookies: (): Promise<void> => unwrap(HttpService.ClearCookies()),

  // P11 D3/D4: resolves a target's (or a .proto's) services and methods — reflection.Register's
  // own cache lives in Go (grpcclient's descriptorCache), never here; `reload` bypasses it (the
  // schema pane's own Reload button).
  grpcDescribe: (args: {
    descriptorMode: 'reflection' | 'proto';
    target: string;
    tls: { enabled: boolean; caFile: string; serverName: string };
    metadata: GrpcMetaPairWire[];
    protoPath: string;
    importPaths: string[];
    collectionId: string;
    environmentId: string;
    reload: boolean;
    /** Names the describe as a cancellable op (`opsCancel`); never persisted. */
    opId: string;
  }): Promise<GrpcSchemaWire> =>
    unwrap(GrpcService.Describe(args)).then((r) => trust<GrpcSchemaWire>(r)),

  // Runs through the same op scheduler/op log HttpService.Send already does (opId
  // renderer-minted, tabId/windowKey addressing exactly like httpSend/TabsService's own shape).
  // `streaming` tells Go which of Unary/ServerStream to run — the renderer already knows this
  // from the schema it resolved via grpcDescribe.
  grpcCall: (args: {
    opId: string;
    tabId: string;
    streaming: boolean;
    descriptorMode: 'reflection' | 'proto';
    target: string;
    tls: { enabled: boolean; caFile: string; serverName: string };
    protoPath: string;
    importPaths: string[];
    service: string;
    method: string;
    messageJson: string;
    metadata: GrpcMetaPairWire[];
    collectionId: string;
    environmentId: string;
    itemId?: string;
    incognito?: boolean;
  }): Promise<GrpcCallResultWire> =>
    unwrap(
      GrpcService.Call({
        ...args,
        windowKey,
        itemId: args.itemId ?? '',
        incognito: args.incognito ?? false,
      }),
    ).then((r) => trust<GrpcCallResultWire>(r)),

  // P11 D8: one server-streaming call's coalesced message batches — EmitTo'd to this window only,
  // so a stream in one window never wakes another.
  onGrpcCall: (cb: (event: GrpcCallEvent) => void): (() => void) => on(CHANNEL.grpcCall, cb),

  // P112: every API-client mutation broadcasts the scopes it touched — the renderer-side
  // apiQueries.ts sync layer is this channel's one subscriber.
  onApiDataChanged: (cb: (event: ApiDataChangedEvent) => void): (() => void) =>
    on(CHANNEL.apiDataChanged, cb),

  // Wrappers over CollectionsService. These stay typed against the generated models
  // rather than `trust<T>()`-ing a domain type, because the one place a saved request's shape
  // genuinely has to be trusted is where it becomes tab state — `openCollectionRequestTab` Zod-
  // parses it there, which is the app's single trust boundary for this document rather than
  // a second one here. Only a *path* ever crosses this bridge for import/export, never a file's
  // bytes: Go reads and Go writes.
  collectionsList: (): Promise<{
    collections: WailsModels.CollectionSummary[];
    items: WailsModels.ItemSummary[];
  }> =>
    // The explicit type argument is `connectionsTest`'s own precedent (control.ts): tests/unit's
    // tsconfig resolves the generated $CancellablePromise loosely, so a `.then` that reads members
    // off the result (rather than handing it straight to `trust`) needs the awaited type named.
    unwrap<WailsModels.CollectionsTree>(CollectionsService.List()).then((r) => ({
      collections: r.collections ?? [],
      items: r.items ?? [],
    })),
  collectionsGetRequest: (itemId: string): Promise<WailsStorageModels.SavedRequest> =>
    unwrap(CollectionsService.GetRequest({ itemId })),
  collectionsSaveRequest: (
    itemId: string,
    name: string,
    request: WailsStorageModels.SavedRequest,
  ): Promise<WailsModels.ItemSummary> =>
    unwrap(CollectionsService.SaveRequest({ itemId, name, request })),
  // P11 D12: GetRequest/SaveRequest's own gRPC siblings.
  collectionsGetGrpcRequest: (itemId: string): Promise<WailsStorageModels.SavedGrpcRequest> =>
    unwrap(CollectionsService.GetGrpcRequest({ itemId })),
  collectionsSaveGrpcRequest: (
    itemId: string,
    name: string,
    request: WailsStorageModels.SavedGrpcRequest,
  ): Promise<WailsModels.ItemSummary> =>
    unwrap(CollectionsService.SaveGrpcRequest({ itemId, name, request })),
  collectionsCreateCollection: (name: string): Promise<WailsModels.CollectionSummary> =>
    unwrap(CollectionsService.CreateCollection({ name })),
  collectionsCreateItem: (args: {
    collectionId: string;
    parentId: string | null;
    kind: 'folder' | 'request';
    request?: WailsStorageModels.SavedRequest | null;
    name: string;
  }): Promise<WailsModels.ItemSummary> =>
    unwrap(CollectionsService.CreateItem({ ...args, request: args.request ?? null })),
  // P11 D12: CreateItem's own gRPC sibling — always a request, never a folder.
  collectionsCreateGrpcItem: (args: {
    collectionId: string;
    parentId: string | null;
    request?: WailsStorageModels.SavedGrpcRequest | null;
    name: string;
  }): Promise<WailsModels.ItemSummary> =>
    unwrap(CollectionsService.CreateGrpcItem({ ...args, request: args.request ?? null })),
  collectionsImport: (paths: string[]): Promise<ImportResult> =>
    unwrap(CollectionsService.Import({ paths })).then((r) => trust<ImportResult>(r)),
  // P5 D16: ExportReport.secretCount is what lets the panel say "N secret values were not
  // written" once, rather than that being a fact only discoverable by opening the file.
  collectionsExport: (collectionId: string, path: string): Promise<WailsModels.ExportReport> =>
    unwrap(CollectionsService.Export({ collectionId, path })),

  // The config files' status and the secret names; SetSecret is write-only, no call returns a value.
  apiConfigView: (): Promise<ApiConfigView> =>
    unwrap(ApiConfigService.View()).then((r) => trust<ApiConfigView>(r)),
  apiConfigSetSecret: (name: string, value: string): Promise<ApiConfigView> =>
    unwrap(ApiConfigService.SetSecret({ name, value })).then((r) => trust<ApiConfigView>(r)),
  apiConfigDeleteSecret: (name: string): Promise<ApiConfigView> =>
    unwrap(ApiConfigService.DeleteSecret({ name })).then((r) => trust<ApiConfigView>(r)),

  // Wrappers over VariablesService — typed against the shared domain mirrors
  // rather than the generated models directly, the one difference from collectionsList's
  // own precedent above (nothing here becomes tab state, so there is no equivalent reason to stay
  // close to the wire shape).
  variablesListEnvironments: (): Promise<ApiEnvironment[]> =>
    unwrap(VariablesService.ListEnvironments()).then((r) => trust<ApiEnvironment[]>(r ?? [])),
  variablesCreateEnvironment: (
    name: string,
    description = '',
    color: PaletteColor = 'none',
  ): Promise<ApiEnvironment> =>
    unwrap(VariablesService.CreateEnvironment({ name, description, color })).then((r) =>
      trust<ApiEnvironment>(r),
    ),
  // id: '' selects "No environment" (D3).
  variablesSetActiveEnvironment: (id: string): Promise<void> =>
    unwrap(VariablesService.SetActiveEnvironment({ id })),
  variablesList: (scope: VariableScope, ownerId: string): Promise<ApiVariable[]> =>
    unwrap(VariablesService.List({ scope: scopeArg(scope), ownerId })).then((r) =>
      trust<ApiVariable[]>(r ?? []),
    ),
  variablesHistory: (variableId: string): Promise<ApiVariableHistoryEntry[]> =>
    unwrap(VariablesService.History({ variableId })).then((r) =>
      trust<ApiVariableHistoryEntry[]>(r ?? []),
    ),

  // D8/D9: neither reveal call ever rejects — the outcome names what happened (revealed |
  // cancelled | confirmation-required | error), the same contract connectionsReveal already has.
  variablesReveal: (variableId: string, confirmed: boolean): Promise<RevealResult> =>
    unwrap(VariablesService.Reveal({ variableId, confirmed })).then((r) => trust<RevealResult>(r)),
  variablesRevealHistory: (historyId: string, confirmed: boolean): Promise<RevealResult> =>
    unwrap(VariablesService.RevealHistory({ historyId, confirmed })).then((r) =>
      trust<RevealResult>(r),
    ),

  // P8 D8: five wrappers over ResponseHistoryService. List/Clear take both ids — '' for whichever
  // doesn't apply — and the service computes the scope key the same way the generated SQLite
  // column does, so the two sides can never disagree about what a scope is.
  historyList: (itemId: string, tabId: string): Promise<ResponseHistoryEntry[]> =>
    unwrap(ResponseHistoryService.List({ itemId, tabId })).then((r) =>
      trust<ResponseHistoryEntry[]>(r ?? []),
    ),
  historyGet: (id: string): Promise<ResponseHistorySnapshot> =>
    unwrap(ResponseHistoryService.Get({ id })).then((r) => trust<ResponseHistorySnapshot>(r)),
  historyDelete: (id: string): Promise<void> => unwrap(ResponseHistoryService.Delete({ id })),
  historyClear: (itemId: string, tabId: string): Promise<void> =>
    unwrap(ResponseHistoryService.Clear({ itemId, tabId })),
  // D14: called from http/state/collections.ts's Save-as path, immediately after the CreateItem
  // that produced itemId.
  historyAdopt: (tabId: string, itemId: string): Promise<number> =>
    // Explicit type argument: tests/unit's tsconfig resolves the generated $CancellablePromise
    // loosely, same as collectionsList above — a `.then` that reads a member off the result needs
    // the awaited type named rather than handed straight to `trust`.
    unwrap<WailsModels.ResponseHistoryAdoptResult>(
      ResponseHistoryService.Adopt({ tabId, itemId }),
    ).then((r) => r.adopted),

  // P11 D11: GrpcHistoryService's own five wrappers — ResponseHistoryService's exact shape,
  // reused verbatim for the second protocol's own history table.
  grpcHistoryList: (itemId: string, tabId: string): Promise<GrpcCallHistoryEntry[]> =>
    unwrap(GrpcHistoryService.List({ itemId, tabId })).then((r) =>
      trust<GrpcCallHistoryEntry[]>(r ?? []),
    ),
  grpcHistoryGet: (id: string): Promise<GrpcCallSnapshot> =>
    unwrap(GrpcHistoryService.Get({ id })).then((r) => trust<GrpcCallSnapshot>(r)),
  grpcHistoryDelete: (id: string): Promise<void> => unwrap(GrpcHistoryService.Delete({ id })),
  grpcHistoryClear: (itemId: string, tabId: string): Promise<void> =>
    unwrap(GrpcHistoryService.Clear({ itemId, tabId })),
  grpcHistoryAdopt: (tabId: string, itemId: string): Promise<number> =>
    unwrap<WailsModels.GrpcHistoryAdoptResult>(GrpcHistoryService.Adopt({ tabId, itemId })).then(
      (r) => r.adopted,
    ),
};
