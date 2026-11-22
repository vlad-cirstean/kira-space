import { CAPTURE_NOW } from '@workbench/testing/screenshots/capture';
import { type KubeHandler, k } from '../../../ui/studio/support/kubernetesFixture';

// Kubernetes module fixture of the story: two clusters, the pods of `orders`. Answer shapes come from
// the `kubernetes` contract; the names and times are the story's.

export const STAGING = 'acme-staging';
export const PROD = 'acme-prod';
export const NAMESPACE = 'orders';
export const POD = 'orders-api-7d9f';

const iso = (agoMs: number) => new Date(CAPTURE_NOW - agoMs).toISOString();
const MIN = 60_000;
const HOUR = 60 * MIN;

type Rec = Record<string, unknown>;

const cluster = (
  id: string,
  context: string,
  server: string,
  current: boolean,
  namespace = '',
) => ({
  cluster: context,
  context,
  current,
  id,
  mcp: { approval: 'prompt', description: '', enabled: false, review: 'inherit' },
  namespace,
  server,
  source: '~/.kube/config',
  user: 'ana',
});

const CLUSTERS = {
  clusters: [
    cluster('kc-staging', STAGING, 'https://staging.k8s.acme.example:6443', true, NAMESPACE),
    cluster('kc-prod', PROD, 'https://prod.k8s.acme.example:6443', false),
  ],
  config: { message: '', paths: ['~/.kube/config'], state: 'ok' },
};

const PODS = [
  { name: POD, ready: '1/1', status: 'Running', restarts: '0', age: '2d' },
  { name: 'orders-worker-5c2a', ready: '1/1', status: 'Running', restarts: '1', age: '2d' },
  { name: 'export-csv-1', ready: '0/1', status: 'Completed', restarts: '0', age: '38m' },
  { name: 'postgres-0', ready: '1/1', status: 'Running', restarts: '0', age: '19d' },
];

const LIST = {
  columns: [
    { name: 'Name', type: 'string' },
    { name: 'Ready', type: 'string' },
    { name: 'Status', type: 'string' },
    { name: 'Restarts', type: 'integer' },
    { name: 'Age', type: 'string' },
  ],
  rows: PODS.map((p) => ({
    cells: [p.name, p.ready, p.status, p.restarts, p.age],
    key: `uid-pod-${NAMESPACE}-${p.name}`,
    name: p.name,
    namespace: NAMESPACE,
  })),
  truncated: false,
};

const describe = () => ({
  manifest: '',
  ...k<Rec>('KubernetesService.Describe'),
  name: POD,
  namespace: NAMESPACE,
  uid: `uid-pod-${NAMESPACE}-${POD}`,
  labels: { app: 'orders-api', tier: 'backend' },
  owners: [{ kind: 'ReplicaSet', name: 'orders-api-7d9f8c6b5' }],
  created: iso(2 * 24 * HOUR),
  containers: [
    { init: true, name: 'migrate' },
    { init: false, name: 'api' },
  ],
  conditions: [
    { type: 'Ready', status: 'True', reason: '', message: '', lastTransition: iso(2 * 24 * HOUR) },
    {
      type: 'PodScheduled',
      status: 'True',
      reason: 'Scheduled',
      message: 'assigned to node-2',
      lastTransition: iso(2 * 24 * HOUR),
    },
  ],
  events: [
    {
      type: 'Normal',
      reason: 'Pulled',
      message: 'image acme/orders-api:1.14.2 pulled',
      source: 'kubelet',
      count: 1,
      first: iso(2 * 24 * HOUR),
      last: iso(2 * 24 * HOUR),
    },
    {
      type: 'Normal',
      reason: 'Started',
      message: 'container api started',
      source: 'kubelet',
      count: 1,
      first: iso(2 * 24 * HOUR),
      last: iso(2 * 24 * HOUR),
    },
  ],
});

const kinds = () => {
  const all = k<{ kinds: { category: string }[] }>('KubernetesService.Kinds');
  return { ...all, kinds: all.kinds.filter((x) => x.category !== 'custom') };
};

/** Handlers for `setupKubernetes`: replace the contract's clusters and pods with the story's. */
export const ACME_KUBE: Record<string, KubeHandler> = {
  Clusters: () => CLUSTERS,
  Status: () => ({ message: '', reason: '', serverVersion: 'v1.31.4', state: 'ok' }),
  Namespaces: () => ({
    fallback: NAMESPACE,
    namespaces: ['default', 'kube-system', NAMESPACE, 'payments'],
    restricted: false,
  }),
  Kinds: kinds,
  List: () => LIST,
  Describe: describe,
};

/** Twenty fixed log lines of the orders API. */
export const POD_LOG = [
  'INFO  server listening on :8080',
  'INFO  GET /v1/orders 200 14ms',
  'INFO  GET /v1/orders/4217 200 9ms',
  'INFO  POST /v1/orders 201 31ms',
  'WARN  slow query on orders (240ms)',
  'INFO  GET /v1/customers/88 200 11ms',
  'INFO  GET /v1/orders?status=paid 200 22ms',
  'INFO  POST /v1/exports 202 18ms',
  'INFO  export exp-2026-09 started',
  'INFO  export exp-2026-09 wrote 1240 rows',
  'INFO  GET /v1/exports/exp-2026-09 200 6ms',
  'INFO  GET /v1/orders/4218 200 8ms',
  'WARN  retrying payments call (1/3)',
  'INFO  payments call ok after 1 retry',
  'INFO  PUT /v1/orders/4218 200 27ms',
  'INFO  GET /healthz 200 1ms',
  'INFO  GET /v1/orders 200 13ms',
  'ERROR order 4199 not found',
  'INFO  GET /v1/orders/4201 200 7ms',
  'INFO  GET /healthz 200 1ms',
] as const;
