import ScriptRunView from '@workbench/automations/runs/ScriptRunView.vue';
import type { TabViewMap } from '@workbench/tabs/types';
import TerminalTabView from '@workbench/terminal/TerminalTabView.vue';
import EnvironmentsTabView from '../api/EnvironmentsView.vue';
import VariableSetTabView from '../api/VariableSetView.vue';
import type { StudioTabKind } from '../state/tabDomain';
import BrowseTabView from '../views/browse/BrowseView.vue';
import ConsoleTabView from '../views/console/ConsoleTabView.vue';
import DefinitionTabView from '../views/definition/DefinitionView.vue';
import DocumentTabView from '../views/documents/DocumentView.vue';
import DataTabView from '../views/grid/DataView.vue';
import GrpcRequestTabView from '../views/grpcrequest/GrpcRequestView.vue';
import HttpRequestTabView from '../views/httprequest/HttpRequestView.vue';
import KeyValueTabView from '../views/keyvalue/KeyValueView.vue';
import StreamTabView from '../views/stream/StreamView.vue';

// The component half of the tab-kind registry — split from state/tabKinds.ts because
// state/ -> workbench/ is a lint-forbidden edge, while workbench/ -> views/ is not. STATIC
// imports, deliberately: this is a registry lookup, not a lazy-load boundary, so the bundle keeps
// exactly the two dynamic chunks docs/ARCHITECTURE.md's Stack section records (sql-formatter,
// @faker-js/faker).
// `StudioTabKind` covers this app's own real vocabulary only, so this is a
// total function over it with no unreachable member to keep total against a wider union.
export const TAB_VIEWS: TabViewMap<StudioTabKind> = {
  data: DataTabView,
  definition: DefinitionTabView,
  console: ConsoleTabView,
  document: DocumentTabView,
  keyvalue: KeyValueTabView,
  stream: StreamTabView,
  browse: BrowseTabView,
  'http-request': HttpRequestTabView,
  'grpc-request': GrpcRequestTabView,
  'variable-set': VariableSetTabView,
  environments: EnvironmentsTabView,
  // The embedded terminal — a static entry like every kind above; @xterm/xterm itself
  // stays behind terminalRenderer.ts's own dynamic import(). The tab view itself is now
  // one shared component (packages/workbench/src/terminal/TerminalTabView.vue), rendering either a
  // Terminal-module tab or a repo terminal — the two differ only in workspaceId/codeRepoId, set by
  // whichever opener built the tab.
  terminal: TerminalTabView,
  'script-run': ScriptRunView,
};
