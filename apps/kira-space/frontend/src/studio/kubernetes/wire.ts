// Field-for-field mirrors of internal/kubernetes' wire types.
import type { McpPermissionMode } from '@shared/domain/connection';

export const KUBE_CHANNEL = {
  logs: 'kira:kubernetes:logs',
  clusters: 'kira:kubernetes:clusters',
} as const;

type ConfigState = 'ok' | 'missing' | 'invalid';

interface KubeConfigInfo {
  state: ConfigState;
  paths: string[];
  message: string;
}

export interface KubeCluster {
  id: string;
  context: string;
  cluster: string;
  server: string;
  user: string;
  namespace: string;
  current: boolean;
  source: string;
  mcp: KubeMcpSettings;
}

/** internal/kubernetes McpSettings: the AI description and one permission per operation group. */
export interface KubeMcpSettings {
  description: string;
  discover: McpPermissionMode;
  list: McpPermissionMode;
  describe: McpPermissionMode;
  logs: McpPermissionMode;
}

export type KubeMcpGroup = 'discover' | 'list' | 'describe' | 'logs';

export interface KubeClusters {
  config: KubeConfigInfo;
  clusters: KubeCluster[];
}

export type UnavailableReason =
  | 'unreachable'
  | 'unauthorized'
  | 'forbidden'
  | 'plugin'
  | 'config'
  | 'timeout';

export interface KubeStatus {
  state: 'ok' | 'unavailable';
  reason: UnavailableReason | '';
  message: string;
  serverVersion: string;
}

export interface KubeNamespaces {
  namespaces: string[];
  restricted: boolean;
  fallback: string;
}

export type KindCategory =
  | 'workloads'
  | 'network'
  | 'config'
  | 'storage'
  | 'access'
  | 'cluster'
  | 'custom';

export interface KubeKind {
  key: string;
  group: string;
  version: string;
  resource: string;
  kind: string;
  namespaced: boolean;
  category: KindCategory;
  logs: boolean;
}

export interface KubeKinds {
  kinds: KubeKind[];
  partial: boolean;
}

interface KubeColumn {
  name: string;
  type: string;
}

export interface KubeRow {
  key: string;
  name: string;
  namespace: string;
  cells: string[];
}

export interface KubeList {
  columns: KubeColumn[];
  rows: KubeRow[];
  truncated: boolean;
}

interface KubeCondition {
  type: string;
  status: string;
  reason: string;
  message: string;
  lastTransition: string;
}

interface KubeEvent {
  type: string;
  reason: string;
  message: string;
  count: number;
  first: string;
  last: string;
  source: string;
}

export interface KubeContainerRef {
  name: string;
  init: boolean;
}

export interface KubeDescription {
  kind: string;
  apiVersion: string;
  name: string;
  namespace: string;
  uid: string;
  created: string;
  labels: Record<string, string>;
  annotations: Record<string, string>;
  owners: { kind: string; name: string }[];
  conditions: KubeCondition[];
  containers: KubeContainerRef[];
  events: KubeEvent[];
  manifest: string;
}

export interface KubeLogLine {
  ts?: string;
  text: string;
}

export interface KubeLogsEvent {
  streamId: string;
  lines: KubeLogLine[];
  ended: boolean;
  error?: string;
}

export interface KubeLogsOpen {
  streamId: string;
  clusterId: string;
  namespace: string;
  pod: string;
  container: string;
  previous: boolean;
  tail: number;
  timestamps: boolean;
  follow: boolean;
}
