import * as KubernetesService from '@bindings/kubernetesservice.js';
import { on, trust, unwrap, windowKey } from '@workbench/bridge/rpc';
import {
  KUBE_CHANNEL,
  type KubeClusters,
  type KubeDescription,
  type KubeKinds,
  type KubeList,
  type KubeLogsEvent,
  type KubeLogsOpen,
  type KubeMcpSettings,
  type KubeNamespaces,
  type KubeStatus,
} from './wire';

export const kubeControl = {
  clusters: () => unwrap(KubernetesService.Clusters()).then((r) => trust<KubeClusters>(r)),
  reorder: (ids: string[]) =>
    unwrap(KubernetesService.Reorder({ ids })).then((r) => trust<KubeClusters>(r)),
  status: (clusterId: string, refresh: boolean, background: boolean) =>
    unwrap(KubernetesService.Status({ clusterId, refresh, background })).then((r) =>
      trust<KubeStatus>(r),
    ),
  setMcp: (clusterId: string, mcp: KubeMcpSettings) =>
    unwrap(KubernetesService.SetMcp({ clusterId, mcp })).then((r) => trust<KubeMcpSettings>(r)),
  namespaces: (clusterId: string, background: boolean) =>
    unwrap(KubernetesService.Namespaces({ clusterId, background })).then((r) =>
      trust<KubeNamespaces>(r),
    ),
  kinds: (clusterId: string, background: boolean) =>
    unwrap(KubernetesService.Kinds({ clusterId, background })).then((r) => trust<KubeKinds>(r)),
  list: (
    clusterId: string,
    kind: string,
    namespace: string,
    labelSelector: string,
    background: boolean,
  ) =>
    unwrap(KubernetesService.List({ clusterId, kind, namespace, labelSelector, background })).then(
      (r) => trust<KubeList>(r),
    ),
  describe: (
    clusterId: string,
    kind: string,
    namespace: string,
    name: string,
    background: boolean,
  ) =>
    unwrap(KubernetesService.Describe({ clusterId, kind, namespace, name, background })).then((r) =>
      trust<KubeDescription>(r),
    ),
  logsOpen: (a: KubeLogsOpen) => unwrap(KubernetesService.LogsOpen({ windowKey, ...a })),
  logsClose: (streamId: string) => unwrap(KubernetesService.LogsClose({ streamId })),
  onClusters: (cb: () => void) => on<unknown>(KUBE_CHANNEL.clusters, () => cb()),
  onLogs: (cb: (e: KubeLogsEvent) => void) => on<KubeLogsEvent>(KUBE_CHANNEL.logs, cb),
};
