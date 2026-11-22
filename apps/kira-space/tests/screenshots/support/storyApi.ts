import type { ApprovalItem } from '@shared/domain/approvals';
import type { KeptList } from '@shared/domain/kept';
import { CAPTURE_NOW } from '@workbench/testing/screenshots/capture';
import { liveItem } from '@workbench/testing/ui/approvals';
import { contract } from '../../ui/support/contract';
import { STORY } from './buildStory';

// Agent API request fixtures of the story: the contract items give the shape, the story gives the
// words. Highlight sites are recomputed for the story text, never copied from the contract.

type Rec = Record<string, unknown>;

export const API_HOST = 'api.acme.example';
export const API_ENV = { color: 'green', id: 'env-staging', name: 'Staging' } as const;
const ORIGIN = `https://${API_HOST}`;
const EXPORT_URL = `${ORIGIN}/v1/exports/{{plain.exportId}}`;
const TRANSFORM =
  'plain.rowCount=response.json.rowCount;return {id:response.json.id,file:response.json.file}';
const HASH = '4c1d7a9e03b85f62a1d0c7e94b3a58f6d2e1907bc4a8356f01e9d27ab63c5d80';

/** `{{plain.x}}` and `{{secret.x}}` spans of a text, UTF-16 offsets, as internal/keptvars RefSpans. */
function refSite(text: string, token: string): { from: number; to: number } {
  const from = text.indexOf(token);
  if (from < 0) throw new Error(`${token} not in ${text}`);
  return { from, to: from + token.length };
}

const target = {
  color: 'green',
  kind: 'http',
  location: API_ENV.name,
  name: API_HOST,
  readOnly: false,
};
const origin = {
  client: 'Claude Code 2.1.318',
  cwd: '',
  source: '',
  step: 'Implement',
  task: STORY.taskTitle,
};
const keptSource = (request: string, kind: 'script' | 'user') => ({
  client: 'Claude Code 2.1.318',
  kind,
  mcp: 'api',
  request,
  tool: 'api_request',
});
const iso = (offsetMs: number) => new Date(CAPTURE_NOW + offsetMs).toISOString();

function first(file: string, key: string): ApprovalItem {
  const [item] = contract<ApprovalItem[]>(file, key);
  if (!item) throw new Error(`contract ${file} has no ${key}`);
  return item;
}

const variables = [
  { name: 'base_url', secret: false, value: ORIGIN },
  { name: 'token', secret: true, value: '••••' },
];

/** GET with a masked bearer token, a saved `exportId` in the URL and a transform that saves `rowCount`. */
export function storyApiRequest(): ApprovalItem {
  const base = first('approvals-api', 'ApprovalsService.List');
  const payload = base.payload as Rec;
  const transform = {
    hash: HASH,
    mayKeep: [
      {
        name: 'rowCount',
        secret: false,
        sites: [
          {
            field: 'script',
            index: 0,
            part: 'transform',
            ...refSite(TRANSFORM, 'plain.rowCount'),
          },
        ],
      },
    ],
    source: TRANSFORM,
  };
  return liveItem(
    {
      ...base,
      id: 'api.request:story-export',
      target,
      origin,
      payload: {
        ...payload,
        environment: API_ENV,
        headers: [{ masked: true, name: 'Authorization', value: 'Bearer {{token}}' }],
        url: EXPORT_URL,
        query: [],
        variables,
        credentials: [{ hosts: [API_HOST], name: 'token' }],
        reviewResult: true,
        kept: [
          {
            bytes: 14,
            createdAt: iso(-12 * 60_000),
            expiresAt: iso(48 * 60_000),
            id: 'kv_exportId',
            items: 0,
            name: 'exportId',
            pending: false,
            preview: '"exp-2026-09"',
            previewTruncated: false,
            secret: false,
            sites: [
              { field: 'url', index: 0, part: '', ...refSite(EXPORT_URL, '{{plain.exportId}}') },
            ],
            source: keptSource(`POST {{base_url}}/v1/exports`, 'script'),
            type: 'string',
          },
        ],
        script: { prepare: null, transform },
      },
    } as ApprovalItem,
    1800,
    CAPTURE_NOW,
  );
}

const RAW = JSON.stringify({
  status: 200,
  statusText: 'OK',
  headers: [
    { name: 'Content-Type', value: 'application/json' },
    { name: 'Content-Length', value: '78' },
  ],
  contentType: 'application/json',
  body: '{"id":"exp-2026-09","status":"ready","rowCount":1240,"file":"orders-2026-09.csv"}',
  bodyBytes: 78,
  truncated: false,
  elapsedMs: 212,
  finalUrl: `${ORIGIN}/v1/exports/exp-2026-09`,
  redirects: [],
});

/** The held response of `storyApiRequest()` and what the agent gets once it is sent. */
export function storyApiReview(): { item: ApprovalItem; held: Rec } {
  const base = first('e2e-script-vars', 'ApprovalsService.List#review');
  const requestId = 'story-export';
  const item = liveItem(
    {
      ...base,
      id: `api.response:${requestId}`,
      target,
      origin,
      payload: {
        ...(base.payload as Rec),
        environment: API_ENV,
        host: API_HOST,
        url: `${ORIGIN}/v1/exports/exp-2026-09`,
        request: {
          body: null,
          headers: [{ masked: true, name: 'Authorization', value: 'Bearer {{token}}' }],
          variables: [],
        },
        bytes: 63,
        rawBytes: RAW.length,
      },
    } as ApprovalItem,
    1800,
    CAPTURE_NOW,
  );
  const held = {
    isError: false,
    keep: [
      {
        bytes: 4,
        deleted: false,
        items: 0,
        name: 'rowCount',
        preview: '1240',
        previewTruncated: false,
        secret: false,
        type: 'number',
      },
    ],
    raw: RAW,
    requestId,
    script: { hash: HASH, source: TRANSFORM },
    text: '{"result":{"id":"exp-2026-09","file":"orders-2026-09.csv"}}',
  };
  return { item, held };
}

/** One value still to type (`otp`) and one already saved (`apiKey`). */
export function storyApiValues(): ApprovalItem {
  const base = first('e2e-user-value', 'ApprovalsService.List#stored');
  const payload = base.payload as Rec;
  const url = `${ORIGIN}/v1/exports?key={{secret.apiKey}}&code={{secret.otp}}`;
  const header = '{{secret.apiKey}}';
  const query = '{{secret.apiKey}}';
  const otp = '{{secret.otp}}';
  const [stored] = payload.kept as Rec[];
  const [input] = payload.inputs as Rec[];
  return liveItem(
    {
      ...base,
      id: 'api.request:story-values',
      target,
      origin,
      payload: {
        ...payload,
        environment: API_ENV,
        url,
        headers: [{ masked: false, name: 'X-Api-Key', value: header }],
        query: [
          { name: 'key', value: query },
          { name: 'code', value: otp },
        ],
        variables: [{ name: 'base_url', secret: false, value: ORIGIN }],
        inputs: [
          {
            name: 'otp',
            label: 'One-time code',
            state: 'needed',
            origin: ORIGIN,
            keepMinutes: 60,
          },
          { ...input, name: 'apiKey', origin: ORIGIN },
        ],
        kept: [
          {
            ...stored,
            name: 'apiKey',
            createdAt: iso(-60_000),
            expiresAt: iso(59 * 60_000),
            sites: [
              { field: 'url', index: 0, part: '', ...refSite(url, '{{secret.apiKey}}') },
              { field: 'header', index: 0, part: 'value', from: 0, to: header.length },
              { field: 'query', index: 0, part: 'value', from: 0, to: query.length },
            ],
            source: keptSource('input apiKey', 'user'),
          },
        ],
      },
    } as ApprovalItem,
    1800,
    CAPTURE_NOW,
  );
}

/** Temporary variables as Settings lists them, with live times. */
export function storyKeptList(): KeptList {
  const list = contract<KeptList>('kept-vars', 'KeptVarsService.List');
  const [secret, plain] = list.entries;
  if (!secret || !plain) throw new Error('contract kept-vars lacks entries');
  const script = (request: string) => keptSource(request, 'script');
  const exports = 'GET {{base_url}}/v1/exports/{{plain.exportId}}';
  return {
    retentionMinutes: 60,
    entries: [
      {
        ...plain,
        id: 'kv_exportId',
        name: 'exportId',
        bytes: 14,
        preview: '"exp-2026-09"',
        scopeLabel: 'External clients',
        source: script('POST {{base_url}}/v1/exports'),
        createdAt: iso(-12 * 60_000),
        expiresAt: iso(48 * 60_000),
      },
      {
        ...plain,
        id: 'kv_rowCount',
        name: 'rowCount',
        type: 'number',
        bytes: 4,
        preview: '1240',
        scopeLabel: 'External clients',
        source: script(exports),
        createdAt: iso(-3 * 60_000),
        expiresAt: iso(57 * 60_000),
      },
      {
        ...secret,
        id: 'kv_sessionToken',
        name: 'sessionToken',
        bytes: 36,
        scopeLabel: 'External clients',
        source: script('POST {{base_url}}/v1/sessions'),
        createdAt: iso(-25 * 60_000),
        expiresAt: iso(35 * 60_000),
      },
    ],
  };
}

const KUBE_SCRIPT =
  'plain.podCount=result.rowCount;return {pods:result.rows.map(r=>r.name),count:result.rowCount}';
const kubeTarget = {
  color: '',
  kind: 'kubernetes',
  location: 'staging.k8s.acme.example:6443 · orders',
  name: 'acme-staging',
  readOnly: false,
};
const kubeRequest = {
  container: '',
  kind: 'v1/pods',
  labelSelector: '',
  maxRows: 200,
  name: '',
  namespace: 'orders',
  pod: '',
  previous: false,
  tail: 0,
  timestamps: false,
};

/** A pod list read of `acme-staging` with a transform script that saves `podCount`. */
export function storyKubeRead(): ApprovalItem {
  const base = first('e2e-kube-script', 'ApprovalsService.List#list');
  const hash = 'b7e24d90c1a36f58e02d4a9137c85be6f01a2d73c94e58b6a0d1f37e92c4b85a';
  return liveItem(
    {
      ...base,
      id: 'kube.read:story-pods',
      target: kubeTarget,
      origin,
      payload: {
        ...(base.payload as Rec),
        context: 'acme-staging',
        namespace: 'orders',
        request: kubeRequest,
        statement: 'kubectl get pods -n orders --context acme-staging',
        server: 'https://staging.k8s.acme.example:6443',
        script: {
          prepare: null,
          transform: {
            hash,
            mayKeep: [
              {
                name: 'podCount',
                secret: false,
                sites: [
                  {
                    field: 'script',
                    index: 0,
                    part: 'transform',
                    ...refSite(KUBE_SCRIPT, 'plain.podCount'),
                  },
                ],
              },
            ],
            source: KUBE_SCRIPT,
          },
        },
      },
    } as ApprovalItem,
    1800,
    CAPTURE_NOW,
  );
}

const PODS_TEXT = JSON.stringify({
  columns: ['Name', 'Ready', 'Status', 'Restarts', 'Age'],
  rowCount: 4,
  rows: [
    ['orders-api-7d9f', '1/1', 'Running', '0', '2d'],
    ['orders-worker-5c2a', '1/1', 'Running', '1', '2d'],
    ['export-csv-1', '0/1', 'Completed', '0', '38m'],
    ['postgres-0', '1/1', 'Running', '0', '19d'],
  ].map((cells) => ({ name: cells[0], namespace: 'orders', cells })),
  truncated: false,
});

/** The held pod list of `acme-staging` and what the agent gets once it is sent. */
export function storyKubeReview(): { item: ApprovalItem; held: Rec } {
  const base = first('kube-review', 'ApprovalsService.List#list');
  const requestId = 'story-pods-review';
  const item = liveItem(
    {
      ...base,
      id: `kube.response:${requestId}`,
      target: kubeTarget,
      origin,
      payload: {
        ...(base.payload as Rec),
        context: 'acme-staging',
        namespace: 'orders',
        request: kubeRequest,
        statement: 'kubectl get pods -n orders --context acme-staging',
        server: 'https://staging.k8s.acme.example:6443',
        bytes: PODS_TEXT.length,
      },
    } as ApprovalItem,
    1800,
    CAPTURE_NOW,
  );
  return {
    item,
    held: { isError: false, keep: [], raw: '', requestId, script: null, text: PODS_TEXT },
  };
}
