import ScriptRunView from '@workbench/automations/runs/ScriptRunView.vue';
import TerminalTabView from '@workbench/terminal/TerminalTabView.vue';
import type { Component } from 'vue';
import type { TabKind } from '../state/tabDomain';
import EnvironmentsTabView from '../studio/api/EnvironmentsView.vue';
import VariableSetTabView from '../studio/api/VariableSetView.vue';
import BrowseTabView from '../studio/views/browse/BrowseView.vue';
import ConsoleTabView from '../studio/views/console/ConsoleTabView.vue';
import DefinitionTabView from '../studio/views/definition/DefinitionView.vue';
import DocumentTabView from '../studio/views/documents/DocumentView.vue';
import DataTabView from '../studio/views/grid/DataView.vue';
import GrpcRequestTabView from '../studio/views/grpcrequest/GrpcRequestView.vue';
import HttpRequestTabView from '../studio/views/httprequest/HttpRequestView.vue';
import KeyValueTabView from '../studio/views/keyvalue/KeyValueView.vue';
import StreamTabView from '../studio/views/stream/StreamView.vue';
import LocalFileTabView from '../views/repo/LocalFileView.vue';
import RepoDiffTabView from '../views/repo/RepoDiffView.vue';
import RepoFileTabView from '../views/repo/RepoFileView.vue';
import RepoGraphTabView from '../views/repo/RepoGraphView.vue';
import RepoMultiDiffTabView from '../views/repo/RepoMultiDiffView.vue';

// The component half of the tab-kind registry (state/tabKinds.ts), split off because
// state/ -> workbench/ is a lint-forbidden edge. Static imports: a registry lookup, not a
// lazy-load boundary.
export const TAB_VIEWS: Record<TabKind, Component> = {
  'repo-graph': RepoGraphTabView,
  'repo-file': RepoFileTabView,
  'local-file': LocalFileTabView,
  'repo-diff': RepoDiffTabView,
  'repo-multi-diff': RepoMultiDiffTabView,
  terminal: TerminalTabView,
  'script-run': ScriptRunView,
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
};
