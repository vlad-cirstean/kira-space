import { hasRequestBody, httpRequestTitle } from '@kira/api-core';
import type { ConnectionColor } from '@shared/domain/connection';
import {
  defaultGrpcRequestTabState,
  type GrpcRequestTabState,
  grpcRequestTabStateSchema,
  grpcRequestTitle,
} from '@shared/domain/grpc';
import {
  defaultHttpRequestTabState,
  type HttpRequestTabState,
  httpRequestTabStateSchema,
} from '@shared/domain/http';
import { tabTitle } from '@shared/domain/tabs';
import { pathTail } from '@shared/domain/tree';
import { t } from '@theme/i18n';
import { scriptRunState } from '@workbench/automations/runs/runStates';
import type { MenuItem } from '@workbench/state/contextMenu';
import { scriptRunTabKind } from '@workbench/tabs/scriptRunTabKind';
import { terminalTabKind } from '@workbench/tabs/terminalTabKind';
import { parseStateWith, type TabKindRegistry } from '@workbench/tabs/types';
import { useTreeStore } from '../project/state/tree';
import { discoverDirty } from '../views/console/kibana/savedDiscover';
import { dropForTab as dropConsoleResultPagesForTab } from '../views/console/resultPages';
import { drop as dropDocumentPagesForTab } from '../views/documents/page';
import { drop as dropGridPagesForTab } from '../views/grid/page';
import { usePendingChangesStore } from '../views/grid/pendingChanges';
import { isEditDirty } from '../views/shared/editDirty';
import { drop as dropKeyValuePagesForTab } from '../views/shared/keyvalue/page';
import { drop as dropStreamPagesForTab } from '../views/stream/page';
import { useConnectionsStore } from './connections';
import { useLayoutStore } from './layout';
import { useSettingsStore } from './settings';
import {
  type BrowseTabRecord,
  type BrowseTabState,
  browseTabStateSchema,
  type ConsoleTabRecord,
  type ConsoleTabState,
  consoleTabStateSchema,
  type DataTabRecord,
  type DataTabState,
  type DefinitionTabRecord,
  type DefinitionTabState,
  type DocumentTabRecord,
  type DocumentTabState,
  dataTabStateSchema,
  defaultBrowseTabState,
  defaultConsoleTabState,
  defaultDataTabState,
  defaultDefinitionTabState,
  defaultDocumentTabState,
  defaultKeyValueTabState,
  defaultStreamTabState,
  definitionTabStateSchema,
  documentTabStateSchema,
  type EnvironmentsTabState,
  environmentsTabStateSchema,
  type GrpcRequestTabRecord,
  type HttpRequestTabRecord,
  type KeyValueTabRecord,
  type KeyValueTabState,
  keyValueTabStateSchema,
  STUDIO_TAB_KIND_MODE,
  type StreamTabRecord,
  type StreamTabState,
  type StudioTabKind,
  streamTabStateSchema,
  type TabRecord,
  type VariableSetTabRecord,
  type VariableSetTabState,
  variableSetTabStateSchema,
} from './tabDomain';
import { useTabIncognitoStore } from './tabIncognito';
import { useTabsStore } from './tabs';
import { useTerminalsStore } from './terminals';

// The tab-kind registry, split from workbench/tabViews.ts by the lint rules —
// this half is component-free (title/icon/railColor/dropResources/menuExtras/state constructors
// only), so it can live in state/ without creating a state/ -> workbench/ edge biome.json forbids.
// Every entry below carries Studio's existing per-kind behaviour verbatim: TabStrip.vue's
// old iconFor body, tabTitle, connectionRecord(tab.connectionId)?.color, and the "Reveal in
// project panel" menu item — nothing here changes what Studio does, only where it lives.
// TabIcon stays a plain string alias (not inlined as `string` directly) since TabStrip.vue's own
// `icon(tab)` call sites read through this type name.
// `TabKindDef` itself is now `@workbench/tabs/types`'s generic contract,
// instantiated once below (via `TabKindRegistry`) with this app's own icon/rail-colour/menu-item
// types.
type TabIcon = string;

const KIND_ICON: Record<string, string> = {
  table: 'table',
  view: 'eye',
  matview: 'symbol-structure',
};

function railColor(tab: TabRecord): ConnectionColor | undefined {
  return useConnectionsStore().connectionRecord(tab.connectionId)?.color;
}

// P71 §5.2: both request kinds' own tab context-menu entry — `setIncognito`'s own listener
// (state/tabs.ts) flushes the tab's existing row immediately on the on-transition.
function incognitoMenuExtras(tab: TabRecord): MenuItem[] {
  const tabIncognitoStore = useTabIncognitoStore();
  return [
    {
      type: 'item',
      id: 'incognito',
      label: tabIncognitoStore.isIncognito(tab.id)
        ? t('studio.tabs.incognitoOff.label')
        : t('studio.tabs.incognito.label'),
      icon: 'eye-closed',
      run: () => tabIncognitoStore.setIncognito(tab.id, !tabIncognitoStore.isIncognito(tab.id)),
    },
  ];
}

// Every Studio kind (F11): a tab addresses a tree node, so "Reveal in project panel" makes sense
// for all seven — an Api tab kind (P2+) supplies its own menuExtras instead, or none at all.
function revealInProjectPanel(tab: TabRecord): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'reveal-in-project-panel',
      label: t('studio.tabs.revealInProject.label'),
      icon: 'target',
      run: () => {
        if (!tab.connectionId) return;
        // A hidden panel has no mounted tree to scroll, so the reveal would silently do nothing.
        const layoutStore = useLayoutStore();
        if (!layoutStore.panel.project.visible) layoutStore.toggleProjectPanel();
        void useTreeStore().revealPath(tab.connectionId, tab.path);
      },
    },
  ];
}

function noDrop(): void {
  // definition/browse have no page store of their own (F12) — nothing to free.
}

export const TAB_KINDS: TabKindRegistry<
  StudioTabKind,
  TabRecord,
  TabIcon,
  ConnectionColor,
  MenuItem
> = {
  data: {
    mode: STUDIO_TAB_KIND_MODE.data,
    title: tabTitle,
    icon: (tab) => {
      const tail = pathTail(tab.path);
      return (tail && KIND_ICON[tail.kind]) || 'table';
    },
    railColor,
    defaultState: () => defaultDataTabState(useSettingsStore().data.defaultPageSize),
    duplicateState: (tab: DataTabRecord): DataTabState => defaultDataTabState(tab.state.pageSize),
    isDirty: (tabId) => usePendingChangesStore().hasPending(tabId),
    dropResources: dropGridPagesForTab,
    menuExtras: revealInProjectPanel,
    parseState: parseStateWith(dataTabStateSchema),
  },
  definition: {
    mode: STUDIO_TAB_KIND_MODE.definition,
    title: tabTitle,
    icon: () => 'file-code',
    railColor,
    defaultState: () => defaultDefinitionTabState(),
    duplicateState: (_tab: DefinitionTabRecord): DefinitionTabState => defaultDefinitionTabState(),
    dropResources: noDrop,
    menuExtras: revealInProjectPanel,
    parseState: parseStateWith(definitionTabStateSchema),
  },
  console: {
    mode: STUDIO_TAB_KIND_MODE.console,
    title: (tab) =>
      tab.kind === 'console' && tab.state.kibana
        ? (tab.state.kibana.saved?.name ??
          tab.state.kibana.dataViewName ??
          t('studio.kibana.tab.title'))
        : tabTitle(tab),
    icon: (tab) => (tab.kind === 'console' && tab.state.kibana ? 'search' : 'terminal'),
    railColor,
    badge: (tab) =>
      tab.kind === 'console' && discoverDirty(tab.state)
        ? {
            icon: 'circle-filled',
            tooltip: t('studio.kibana.saved.dirty.tooltip', {
              name: tab.state.kibana?.saved?.name ?? '',
            }),
          }
        : null,
    defaultState: () => defaultConsoleTabState(),
    // A duplicate is an unsaved copy of the same query.
    duplicateState: (tab: ConsoleTabRecord): ConsoleTabState =>
      tab.state.kibana
        ? {
            ...defaultConsoleTabState(),
            text: tab.state.text,
            kibana: { ...tab.state.kibana, saved: null },
          }
        : defaultConsoleTabState(),
    isDirty: (tabId) => discoverDirty(useTabsStore().findConsoleTab(tabId)?.state),
    dropResources: dropConsoleResultPagesForTab,
    menuExtras: revealInProjectPanel,
    parseState: parseStateWith(consoleTabStateSchema),
  },
  document: {
    mode: STUDIO_TAB_KIND_MODE.document,
    title: tabTitle,
    icon: () => 'json',
    railColor,
    defaultState: () => defaultDocumentTabState(useSettingsStore().data.defaultPageSize),
    duplicateState: (tab: DocumentTabRecord): DocumentTabState =>
      defaultDocumentTabState(tab.state.pageSize),
    isDirty: isEditDirty,
    dropResources: dropDocumentPagesForTab,
    menuExtras: revealInProjectPanel,
    parseState: parseStateWith(documentTabStateSchema),
  },
  keyvalue: {
    mode: STUDIO_TAB_KIND_MODE.keyvalue,
    title: tabTitle,
    // P17: a 'keyvalue' tab is a redis key OR an s3 object — pathTail's own node kind tells
    // them apart with no extra state.
    icon: (tab) => (pathTail(tab.path)?.kind === 'object' ? 'file' : 'symbol-key'),
    railColor,
    defaultState: () => defaultKeyValueTabState(useSettingsStore().data.defaultPageSize),
    duplicateState: (tab: KeyValueTabRecord): KeyValueTabState =>
      defaultKeyValueTabState(tab.state.pageSize),
    isDirty: isEditDirty,
    dropResources: dropKeyValuePagesForTab,
    menuExtras: revealInProjectPanel,
    parseState: parseStateWith(keyValueTabStateSchema),
  },
  stream: {
    mode: STUDIO_TAB_KIND_MODE.stream,
    title: tabTitle,
    icon: () => 'broadcast',
    railColor,
    defaultState: () => defaultStreamTabState(useSettingsStore().data.defaultPageSize),
    duplicateState: (tab: StreamTabRecord): StreamTabState =>
      defaultStreamTabState(tab.state.pageSize),
    dropResources: dropStreamPagesForTab,
    menuExtras: revealInProjectPanel,
    parseState: parseStateWith(streamTabStateSchema),
  },
  browse: {
    mode: STUDIO_TAB_KIND_MODE.browse,
    title: tabTitle,
    icon: () => 'list-tree',
    railColor,
    defaultState: () => defaultBrowseTabState(),
    duplicateState: (_tab: BrowseTabRecord): BrowseTabState => defaultBrowseTabState(),
    // The split's preview pane keys its page under `${tabId}::preview`; dropResources runs on
    // close and on disconnect, so the preview bytes are freed on both.
    dropResources: (id) => dropKeyValuePagesForTab(`${id}::preview`),
    menuExtras: revealInProjectPanel,
    parseState: parseStateWith(browseTabStateSchema),
  },
  'http-request': {
    mode: STUDIO_TAB_KIND_MODE['http-request'],
    title: (tab) => httpRequestTitle((tab as HttpRequestTabRecord).state),
    icon: () => 'globe',
    // D2: no connection, so no rail — TabStrip's own rail already resolves undefined to
    // transparent (P1 F17).
    railColor: () => undefined,
    defaultState: () => defaultHttpRequestTabState(),
    // Deliberately breaks with every Studio kind's "same target, fresh default state" — an
    // HTTP request's state *is* the request, so duplicating it to try a variant is the only
    // reason anyone would. headers/urlEncoded/formData are deep-copied since each row is
    // an object in an array (a shallow spread would share row objects between the two tabs, so
    // editing one's fields would edit the other's); binaryFile is copied as a fresh object.
    // A duplicated tab is an **unsaved copy** — which is what duplicating a saved request
    // to try a variant means. Clearing both is also what keeps `path` honest: the duplicate
    // carries the original's path, so leaving itemId set would make openTab's reuse lookup
    // activate the copy when the user next opened the original from the tree.
    duplicateState: (tab: HttpRequestTabRecord): HttpRequestTabState => ({
      ...tab.state,
      itemId: null,
      name: '',
      headers: tab.state.headers.map((h) => ({ ...h })),
      urlEncoded: tab.state.urlEncoded.map((f) => ({ ...f })),
      formData: tab.state.formData.map((f) => ({ ...f })),
      binaryFile: tab.state.binaryFile ? { ...tab.state.binaryFile } : null,
    }),
    // D2: the response lives in the view's own runtime store (views/httprequest/state.ts),
    // freed by cleanupTabRuntime — there is no page store of this kind's own to drop.
    dropResources: noDrop,
    // D2: there is no project panel to reveal an HTTP request into. P71 §5.2: the incognito
    // toggle is the one thing this kind appends instead.
    menuExtras: incognitoMenuExtras,
    parseState: parseStateWith(httpRequestTabStateSchema),
    // P15 D8: answers a question about a tab you're not looking at — gRPC is deliberately left
    // out (a call always has a message body, so the mark would be on every tab always, which is
    // not information; §8 OQ-3).
    badge: (tab) =>
      hasRequestBody((tab as HttpRequestTabRecord).state)
        ? { icon: 'symbol-namespace', tooltip: t('studio.tabs.requestHasBody.tooltip') }
        : null,
  },
  'grpc-request': {
    mode: STUDIO_TAB_KIND_MODE['grpc-request'],
    title: (tab) => grpcRequestTitle((tab as GrpcRequestTabRecord).state),
    // D2: distinct from 'globe' at a glance in a strip holding both kinds.
    icon: () => 'symbol-interface',
    // D2: no connection, so no rail.
    railColor: () => undefined,
    defaultState: () => defaultGrpcRequestTabState(),
    // D2: a deliberate break with "same target, fresh default state" — a gRPC request's state
    // *is* the request, mirroring 'http-request''s own identical reasoning.
    duplicateState: (tab: GrpcRequestTabRecord): GrpcRequestTabState => ({
      ...tab.state,
      itemId: null,
      name: '',
      importPaths: [...tab.state.importPaths],
      metadata: tab.state.metadata.map((m) => ({ ...m })),
    }),
    // D2: the runtime (and a still-running call) lives in views/grpcrequest/state.ts, freed —
    // and cancelled — by registerTabRuntimeCleanup, not by this hook.
    dropResources: noDrop,
    // D2: there is no project panel to reveal a gRPC request into. P71 §5.2: 'http-request''s own
    // incognitoMenuExtras.
    menuExtras: incognitoMenuExtras,
    parseState: parseStateWith(grpcRequestTabStateSchema),
  },
  'variable-set': {
    mode: STUDIO_TAB_KIND_MODE['variable-set'],
    // D16: the owner's last-known name, falling back to a generic title before the list has
    // loaded after a restore (mirrors HttpRequestTabState's own `name` field's own convention).
    title: (tab) => (tab as VariableSetTabRecord).state.name || t('common.module.variables.label'),
    // D16: collection scope keeps the variable symbol; environment scope reads as a settings
    // surface — distinguishable at a glance in a strip holding both.
    icon: (tab) =>
      (tab as VariableSetTabRecord).state.scope === 'environment'
        ? 'settings-gear'
        : 'symbol-variable',
    // No connection, so no rail — same as the two request kinds.
    railColor: () => undefined,
    // Never reached through a generic "new tab of this kind" affordance — a variable-set tab is
    // always opened contextually (openVariableSetTab) with a real scope/ownerId. This placeholder
    // only satisfies TabKindDef's own required member.
    defaultState: (): VariableSetTabState => ({ scope: 'collection', ownerId: '', name: '' }),
    // A variable set has no variant-to-try story — "duplicate" returns the same target's own
    // state, which is exactly what makes openTab's `reuse: true` correct rather than a bug (F8).
    duplicateState: (tab: VariableSetTabRecord): VariableSetTabState => ({ ...tab.state }),
    // The rows/error runtime lives in api/state/variables.ts, freed by its own
    // registerTabRuntimeCleanup — not by this hook (mirrors http-request's own D2 reasoning).
    dropResources: noDrop,
    // No project-panel reveal for a variable set (mirrors grpc-request's own reasoning).
    menuExtras: () => [],
    parseState: parseStateWith(variableSetTabStateSchema),
  },
  // P28 D16(c): the environment list. No state of its own, one fixed `path`, so openTab's
  // `reuse: true` gives exactly one of these however many times the action is invoked.
  environments: {
    mode: STUDIO_TAB_KIND_MODE.environments,
    title: () => t('studio.api.common.environments.label'),
    icon: () => 'server-environment',
    railColor: () => undefined,
    defaultState: (): EnvironmentsTabState => ({}),
    // Nothing to vary, so a duplicate is the same tab — which is what makes `reuse: true` correct
    // rather than a bug, the same reasoning 'variable-set' above records.
    duplicateState: (): EnvironmentsTabState => ({}),
    // The environment list lives in api/state/variables.ts and is shared with every other Api
    // surface — this tab owns none of it, so closing it frees nothing.
    dropResources: noDrop,
    menuExtras: () => [],
    parseState: parseStateWith(environmentsTabStateSchema),
  },
  // `TAB_KINDS` is a total function over this app's own real vocabulary — no
  // unreachable member to keep total against a wider union.
  // An embedded shell at a working directory, rendered with @xterm/xterm.
  // A launch's own label (a script's name, or 'Claude Code') wins over the cwd's basename.
  // The descriptor itself moved to workbench's own terminalTabKind, shared with
  // kira-space's identical copy — only the mode constant and dropResources' own store differ.
  terminal: terminalTabKind<'terminal', TabRecord, TabIcon, ConnectionColor, MenuItem>(
    STUDIO_TAB_KIND_MODE.terminal,
    (tabId) => useTerminalsStore().closeTerminalSession(tabId),
    (tabId) => useTerminalsStore().terminalSession(tabId),
  ),
  'script-run': scriptRunTabKind<'script-run', TabRecord, TabIcon, ConnectionColor, MenuItem>(
    STUDIO_TAB_KIND_MODE['script-run'],
    scriptRunState,
  ),
};
