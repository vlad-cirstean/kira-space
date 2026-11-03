import type { Page } from '@playwright/test';
import { runtimeErrorBody } from '@workbench/testing/ui/mockRuntime';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, type test } from '../fixtures';
import { contract } from './contract';

export type KubeHandler = (args: Record<string, unknown>) => unknown;

const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge';
const KEY = (m: string) => `KubernetesService.${m}`;
export const k = <T = Record<string, unknown>>(key: string): T => contract<T>('kubernetes', key);

export const kubeError = (code: string, message: string) => ({ error: { code, message } });
const isError = (v: unknown): v is ReturnType<typeof kubeError> =>
  typeof v === 'object' && v !== null && 'error' in v;

export const clusters = () =>
  k<{ clusters: { id: string; context: string }[] }>(KEY('Clusters')).clusters;
export const idOf = (context: string): string => {
  const c = clusters().find((x) => x.context === context);
  if (!c) throw new Error(`no cluster ${context}`);
  return c.id;
};

/** The Clusters result with its clusters in `ids` order (Reorder's answer for other orders). */
export const clustersIn = (ids: readonly string[]) => {
  const all = k<{ clusters: { id: string }[] }>(KEY('Clusters'));
  return { ...all, clusters: ids.map((id) => all.clusters.find((c) => c.id === id)) };
};

const SECRET_LIST = {
  columns: [
    { name: 'Name', type: 'string' },
    { name: 'Type', type: 'string' },
  ],
  rows: [
    {
      key: 'uid-secret-db-creds',
      name: 'db-creds',
      namespace: 'default',
      cells: ['db-creds', 'Opaque'],
    },
  ],
  truncated: false,
};

function defaults(): Record<string, KubeHandler> {
  return {
    Clusters: () => k(KEY('Clusters')),
    Reorder: () => k(KEY('Reorder')),
    Status: (a) => {
      if (a.clusterId === idOf('bad-token')) return k(KEY('Status:unauthorized'));
      if (a.clusterId === idOf('plugin-ctx')) return k(KEY('Status:plugin'));
      return k(KEY('Status'));
    },
    SetMcp: () => contract('dbmcp-kube', KEY('SetMcp')),
    Namespaces: () => k(KEY('Namespaces')),
    Kinds: () => k(KEY('Kinds')),
    List: (a) => {
      if (a.labelSelector === '!!') return kubeError('E_BAD_REQUEST', 'invalid label selector');
      if (a.kind === 'v1/secrets') return SECRET_LIST;
      return k(KEY(a.namespace === '' ? 'List:allNamespaces' : 'List'));
    },
    Describe: (a) => ({
      manifest: '',
      ...k<Record<string, unknown>>(KEY(a.name === 'db-creds' ? 'Describe:secret' : 'Describe')),
    }),
    LogsOpen: () => undefined,
    LogsClose: () => undefined,
  };
}

export interface KubeMock {
  calls(method?: string): { method: string; args: Record<string, unknown> }[];
}

async function install(page: Page, handlers: Record<string, KubeHandler>): Promise<KubeMock> {
  const prefix = `${BRIDGE_PKG}.KubernetesService.`;
  const log: { method: string; args: Record<string, unknown> }[] = [];
  await page.route('**/wails/runtime', async (route) => {
    const request = route.request();
    if (request.method() !== 'POST') return route.fallback();
    const body = JSON.parse(request.postData() ?? '{}') as {
      args?: { methodName?: string; args?: unknown[] };
    };
    const name = body.args?.methodName;
    if (!name?.startsWith(prefix)) return route.fallback();
    const method = name.slice(prefix.length);
    const args = (body.args?.args?.[0] ?? {}) as Record<string, unknown>;
    log.push({ method, args });
    const handler = handlers[method];
    if (!handler) {
      return route.fulfill({
        status: 422,
        contentType: 'application/json',
        body: runtimeErrorBody('E_FIXTURE_MISS', `no kubernetes mock handler for ${method}`),
      });
    }
    const result = handler(args);
    if (isError(result)) {
      return route.fulfill({
        status: 422,
        contentType: 'application/json',
        body: runtimeErrorBody(result.error.code, result.error.message),
      });
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: result === undefined ? 'null' : JSON.stringify(result),
    });
  });
  return { calls: (m) => (m ? log.filter((c) => c.method === m) : log) };
}

export async function setupKubernetes(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  overrides: Record<string, KubeHandler> = {},
  controlRoutes: readonly ControlSnapshot[] = [],
) {
  const { window: page, control } = await relaunch({ control: controlRoutes });
  const kube = await install(page, { ...defaults(), ...overrides });
  return { page, kube, control };
}

export const loc = (page: Page, id: string) => page.locator(`[data-testid="${id}"]`);

export async function openKubernetes(page: Page): Promise<void> {
  await page.locator('[data-testid="mode-tab"][data-mode="kubernetes"]').click();
  await expect(loc(page, 'kubernetes-panel')).toBeVisible();
}

export async function openCluster(page: Page, context: string): Promise<void> {
  const row = page.locator(`[data-testid="kube-cluster-row"][data-context="${context}"]`);
  await row.click();
  await expect(row).toHaveAttribute('data-state', 'ok');
}

/** Opens the cluster, then shows `key`: Pods is the default chip, any other kind comes from the picker. */
export async function openKind(page: Page, context: string, key: string): Promise<void> {
  await openCluster(page, context);
  if (key === 'v1/pods') {
    await expect(
      page.locator('[data-testid="kube-kind-chip-v1_pods"][data-state="on"]'),
    ).toBeVisible();
    return;
  }
  await loc(page, 'kube-kind-more').click();
  await loc(page, `kube-kind-more-option-${key.replaceAll('/', '_')}`).click();
}

export const activePanel = (page: Page) =>
  page.locator('[data-testid="kube-detail"][data-active="true"]');

export const tabChip = (page: Page, name: string) =>
  loc(page, 'kube-tabs').locator('[data-testid="tab"]').filter({ hasText: name });
