import { fromSavedGrpcRequest, fromSavedRequest } from '@kira/api-core';
import type { GrpcSavedRequest, HttpSavedRequest } from '@shared/domain/collections';
import {
  defaultGrpcRequestTabState,
  type GrpcRequestTabState,
  grpcRequestTabStateSchema,
} from '@shared/domain/grpc';
import {
  defaultHttpRequestTabState,
  type HttpRequestTabState,
  httpRequestTabStateSchema,
} from '@shared/domain/http';
import {
  asGrpcRequestTab,
  asHttpRequestTab,
  type GrpcRequestTabRecord,
  type HttpRequestTabRecord,
} from '../../state/tabDomain';
import { type OpenTabResult, useTabsStore } from '../../state/tabs';
import { useVariablesMatrixStore } from './state/variablesMatrix';

// P12 D9: the module's own tab helpers, moved out of state/tabs.ts (§1.4) — these are the
// module's own code that happened to be written in a shell file, so this is a cut-and-paste onto
// the generic openTab/patchTabState primitives the shell keeps, not a new abstraction.

// Always a fresh tab — an HTTP request has no target to reuse by (its own id is its
// identity), the same "always new" shape openConsoleTab already has for the same reason.
// `connectionId` is null and `path` is the literal constant 'request' (non-empty, carrying no
// false uniqueness, safe through pathTail).
export function openApiRequestTab(): string {
  return useTabsStore().openTab(
    'http-request',
    null,
    'request',
    () => defaultHttpRequestTabState(),
    {
      reuse: false,
    },
  ).id;
}

// P107 I2-16: openCollectionRequestTab/openCollectionGrpcRequestTab shared this exact
// find-by-itemId/activate/schema.parse({...defaults, ...fromSaved, itemId, name})/openTab shape,
// differing only in kind and which schema/defaults/converter each uses.
function openSavedRequestTab<S extends { itemId: string | null }>(opts: {
  kind: 'http-request' | 'grpc-request';
  itemId: string;
  name: string;
  schema: { parse(v: unknown): S };
  defaults: S;
  fromSaved: Partial<S>;
}): OpenTabResult {
  const tabsStore = useTabsStore();
  const existing = tabsStore.tabs.find(
    (t) => t.kind === opts.kind && (t as unknown as { state: S }).state.itemId === opts.itemId,
  );
  if (existing) {
    tabsStore.activateTab(existing.id);
    return { id: existing.id, reused: true };
  }
  // D4: the one boundary where a stored saved request becomes tab state, and so the one place it
  // is Zod-parsed — reusing TabKindDef.parseState's own mechanism rather than adding a second
  // trust boundary. fromSavedRequest/fromSavedGrpcRequest carry F4's method coercion.
  const state = opts.schema.parse({
    ...opts.defaults,
    ...opts.fromSaved,
    itemId: opts.itemId,
    name: opts.name,
  });
  return tabsStore.openTab(opts.kind, null, 'request', () => state, { reuse: false });
}

// A saved request opens the **existing** 'http-request' tab kind — the same view, with its state
// sourced from a collection row instead of defaultHttpRequestTabState().
// No new tab kind, so tabKindSchema, RENDERABLE_TAB_KINDS, TAB_KIND_MODE, tabRecordSchema and Go's
// model.RenderableTabKinds are all byte-identical.
//
// `path` stays the literal constant 'request'. A real `collection:<id>/request:<id>` path is
// declined: duplicateTab copies `path` verbatim while duplicateState clears `itemId`, so a
// duplicated tab would carry the saved request's path identity with a state saying it is unsaved —
// and openTab's reuse lookup (keyed on kind + connectionId + path) would then activate the
// *duplicate* when the user opened the
// original. Keeping identity in exactly one place and doing the lookup explicitly is four lines
// and has no such failure mode.
export function openCollectionRequestTab(
  itemId: string,
  name: string,
  saved: HttpSavedRequest,
): OpenTabResult {
  return openSavedRequestTab({
    kind: 'http-request',
    itemId,
    name,
    schema: httpRequestTabStateSchema,
    defaults: defaultHttpRequestTabState(),
    fromSaved: fromSavedRequest(saved),
  });
}

/** Brings every bound request tab's name in line with the tree (a rename in another window only
 *  refreshes the tree there). A tab whose row is gone (orphan) keeps its last known name. */
export function syncRequestTabNames(items: readonly { id: string; name: string }[]): void {
  const nameById = new Map(items.map((item) => [item.id, item.name]));
  for (const tab of useTabsStore().tabs) {
    if (tab.kind === 'http-request') {
      const { itemId, name } = (tab as HttpRequestTabRecord).state;
      const next = itemId ? nameById.get(itemId) : undefined;
      if (next !== undefined && next !== name) patchHttpRequestTabState(tab.id, { name: next });
    } else if (tab.kind === 'grpc-request') {
      const { itemId, name } = (tab as GrpcRequestTabRecord).state;
      const next = itemId ? nameById.get(itemId) : undefined;
      if (next !== undefined && next !== name) patchGrpcRequestTabState(tab.id, { name: next });
    }
  }
}

// P11 D2: 'grpc-request''s own sibling of the four openApiRequestTab-family functions above —
// identical reasoning throughout (always fresh, no target to reuse by; a saved request opens the
// existing kind with state sourced from the collection row instead of the default).
export function openGrpcRequestTab(): string {
  return useTabsStore().openTab(
    'grpc-request',
    null,
    'request',
    () => defaultGrpcRequestTabState(),
    {
      reuse: false,
    },
  ).id;
}

export function openCollectionGrpcRequestTab(
  itemId: string,
  name: string,
  saved: GrpcSavedRequest,
): OpenTabResult {
  return openSavedRequestTab({
    kind: 'grpc-request',
    itemId,
    name,
    schema: grpcRequestTabStateSchema,
    defaults: defaultGrpcRequestTabState(),
    fromSaved: fromSavedGrpcRequest(saved),
  });
}

// P2: no skipUnchanged guard — a Params-table edit rewriting the URL to the value it already had
// (D9) is rare enough that the extra write is not worth the comparison every other patcher above
// already accepts skipping for a hotter path (scroll offsets, page index).
export function patchHttpRequestTabState(id: string, patch: Partial<HttpRequestTabState>): void {
  useTabsStore().patchTabState(id, 'http-request', patch, { skipUnchanged: false });
}

export function patchGrpcRequestTabState(id: string, patch: Partial<GrpcRequestTabState>): void {
  useTabsStore().patchTabState(id, 'grpc-request', patch, { skipUnchanged: false });
}

/** True when any open request tab (HTTP or gRPC) is bound to one of `itemIds`. */
export function hasOpenRequestTab(itemIds: ReadonlySet<string>): boolean {
  return useTabsStore().tabs.some((tab) => {
    if (tab.kind !== 'http-request' && tab.kind !== 'grpc-request') return false;
    const { itemId } = (tab as HttpRequestTabRecord | GrpcRequestTabRecord).state;
    return itemId !== null && itemIds.has(itemId);
  });
}

/** A move into another collection file makes new rows: every tab bound to a moved row follows it. */
export function remapRequestTabItems(moved: Readonly<Record<string, string>>): void {
  for (const tab of useTabsStore().tabs) {
    if (tab.kind === 'http-request') {
      const { itemId } = (tab as HttpRequestTabRecord).state;
      const next = itemId ? moved[itemId] : undefined;
      if (next && next !== itemId) patchHttpRequestTabState(tab.id, { itemId: next });
    } else if (tab.kind === 'grpc-request') {
      const { itemId } = (tab as GrpcRequestTabRecord).state;
      const next = itemId ? moved[itemId] : undefined;
      if (next && next !== itemId) patchGrpcRequestTabState(tab.id, { itemId: next });
    }
  }
}

export function findHttpRequestTab(id: string): HttpRequestTabRecord | null {
  return asHttpRequestTab(useTabsStore().tabs.find((t) => t.id === id));
}

export function findGrpcRequestTab(id: string): GrpcRequestTabRecord | null {
  return asGrpcRequestTab(useTabsStore().tabs.find((t) => t.id === id));
}

/** The Variables tab (kind `environments`): one tab over a fixed path, so the action focuses the
 *  existing one. A collection id points its default column at that collection. */
export function openVariablesTab(collectionId?: string): OpenTabResult {
  if (collectionId) useVariablesMatrixStore().focusCollection(collectionId);
  return useTabsStore().openTab('environments', null, 'environments', () => ({}), { reuse: true });
}
