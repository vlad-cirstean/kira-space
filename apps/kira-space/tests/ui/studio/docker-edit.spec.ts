import type { Locator, Page } from '@playwright/test';
import { installDockerMocks } from '../../../../../packages/docker-ui/src/testing/ui/dockerMock';
import { expect, test } from './fixtures';
import { modeTab } from './support/apiMode';
import { contract } from './support/contract';
import { containerDetailBare } from './support/dockerFixture';

const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge';
const GB = 1024 ** 3;
const OLD_ID = 'c-edit-0001';
const NEW_ID = 'c-new-0002';

const OK_STATUS = {
  state: 'ok',
  endpoint: {
    context: 'default',
    host: 'unix:///var/run/docker.sock',
    source: 'default',
    secure: true,
    remote: false,
  },
  engine: {
    version: '29.0.0',
    apiVersion: '1.52',
    os: 'linux',
    arch: 'amd64',
    operatingSystem: 'Debian',
    kernelVersion: '6.1',
    cpus: 8,
    memTotal: 16 * GB,
    containers: 1,
    running: 1,
    paused: 0,
    stopped: 0,
    images: 1,
  },
};

// biome-ignore lint/suspicious/noExplicitAny: contract fixtures are untyped JSON
type Json = Record<string, any>;

function row(id: string, name: string, extra: Json = {}): Json {
  return {
    id,
    name,
    image: 'alpine:3.20',
    imageId: 'sha256:img1',
    state: 'running',
    status: 'Up 2 minutes',
    created: 1_700_000_000,
    ports: [],
    composeProject: '',
    composeService: '',
    origin: '',
    originName: '',
    networks: ['bridge'],
    ...extra,
  };
}

const specFixture = (tag = ''): Json => ({
  ...contract<Json>('docker-edit', `DockerService.ContainerEditSpec${tag}`),
  id: OLD_ID,
  baseHash: 'hash-1',
});

interface Setup {
  name?: string;
  spec?: Json;
  handlers?: Record<string, (args: unknown) => unknown>;
}

async function setup(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  opts: Setup = {},
) {
  const { window: page } = await relaunch({ control: [] });
  const state = {
    spec: opts.spec ?? specFixture(),
    containers: [row(OLD_ID, opts.name ?? 'kira-flow-edit')] as Json[],
  };
  const docker = await installDockerMocks(page, {
    bridgePkg: BRIDGE_PKG,
    handlers: {
      Status: () => OK_STATUS,
      Containers: () => state.containers,
      Images: () => [],
      Volumes: () => [
        {
          name: 'kira-flow-edit',
          driver: 'local',
          mountpoint: '/v',
          scope: 'local',
          created: '',
          labels: {},
          usedBy: [],
        },
      ],
      Networks: () =>
        ['bridge', 'kira-flow-edit', 'kira-flow-edit2'].map((name) => ({
          id: name,
          name,
          driver: 'bridge',
          scope: 'local',
          internal: false,
          subnets: [],
          containers: 0,
          builtin: name === 'bridge',
          usedBy: [],
        })),
      InspectContainer: (a) => {
        const c = state.containers.find((x) => x.id === (a as { id: string }).id);
        return c
          ? containerDetailBare(c)
          : { error: { code: 'E_NOT_FOUND', message: 'no such container' } };
      },
      ContainerEditSpec: () => state.spec,
      ...opts.handlers,
    },
  });
  return { page, docker, state };
}

async function openEdit(page: Page, name = 'kira-flow-edit'): Promise<void> {
  await modeTab(page, 'docker').click();
  await expect(page.locator('[data-testid="docker-panel"]')).toBeVisible();
  await page.locator(`[data-testid="docker-row"][data-name="${name}"]`).click();
  await page.locator('[data-testid="docker-tab-edit"]').click();
  await expect(page.locator('[data-testid="docker-edit-inplace"]')).toBeVisible();
}

const tid = (page: Page, id: string): Locator => page.locator(`[data-testid="${id}"]`);
const field = (page: Page, id: string): Locator => tid(page, `docker-edit-field-${id}`);

async function showTab(page: Page, tab: 'inplace' | 'recreate'): Promise<void> {
  await tid(page, `docker-edit-tab-${tab}`).click();
  await expect(tid(page, `docker-edit-section-${tab}`)).toBeVisible();
}

function withoutIdentity(args: unknown): unknown {
  const { id: _id, baseHash: _hash, ...rest } = args as Json;
  return rest;
}

test('contract: edit tab splits in-place and recreate into tabs without mode badges', async ({
  relaunch,
}) => {
  const { page } = await setup(relaunch);
  await openEdit(page);

  await expect(tid(page, 'docker-edit-tab-inplace')).toHaveAttribute('data-state', 'on');
  await expect(tid(page, 'docker-edit-tab-recreate')).toHaveAttribute('data-state', 'off');
  await expect(tid(page, 'docker-edit-badge')).toHaveCount(0);
  await expect(tid(page, 'docker-edit-name')).toHaveValue('kira-flow-edit');
  await expect(tid(page, 'docker-edit-pending')).toHaveCount(0);
  await expect(tid(page, 'docker-edit-tab-dirty')).toHaveCount(0);
  await expect(tid(page, 'docker-edit-apply-now')).toBeDisabled();

  await tid(page, 'docker-edit-name').fill('kira-flow-other');
  await expect(
    tid(page, 'docker-edit-tab-inplace').locator('[data-testid="docker-edit-tab-dirty"]'),
  ).toBeVisible();
  await expect(
    tid(page, 'docker-edit-tab-recreate').locator('[data-testid="docker-edit-tab-dirty"]'),
  ).toHaveCount(0);

  await showTab(page, 'recreate');
  await expect(tid(page, 'docker-edit-badge')).toHaveCount(0);
  await expect(tid(page, 'docker-edit-apply-recreate')).toBeDisabled();
  await expect(tid(page, 'docker-edit-apply-now')).toHaveCount(0);
});

test('contract: in-place apply sends the recorded args and keeps recreate edits', async ({
  relaunch,
}) => {
  const sent = contract<Json>('docker-edit', 'args:DockerService.UpdateContainer');
  const result = contract<Json>('docker-edit', 'DockerService.UpdateContainer');
  const calls: unknown[] = [];
  const ctx: { state?: { spec: Json } } = {};
  const { page, docker, state } = await setup(relaunch, {
    handlers: {
      UpdateContainer: (a) => {
        calls.push(a);
        if (ctx.state)
          ctx.state.spec = { ...ctx.state.spec, baseHash: 'hash-2', inPlace: (a as Json).spec };
        return result;
      },
    },
  });
  ctx.state = state;
  await openEdit(page);

  await tid(page, 'docker-edit-name').fill(sent.spec.name);
  await tid(page, 'docker-edit-memory').fill('96');
  await tid(page, 'docker-edit-swap').fill('192');
  await tid(page, 'docker-edit-reservation').fill('32');
  await tid(page, 'docker-edit-cpus').fill('0.5');
  await tid(page, 'docker-edit-cpu-shares').fill('512');
  await tid(page, 'docker-edit-pids').fill('64');
  await tid(page, 'docker-edit-restart').selectOption('unless-stopped');
  const aliases = page.locator(
    '[data-testid="docker-edit-network"][data-name="kira-flow-edit"] [data-testid="docker-edit-network-aliases"]',
  );
  await aliases.fill('svc, api');
  await aliases.blur();
  await tid(page, 'docker-edit-network-select').selectOption('kira-flow-edit2');
  await tid(page, 'docker-edit-network-add').click();
  await showTab(page, 'recreate');
  await page.locator('[data-testid="docker-edit-env"] [data-testid="docker-edit-row-add"]').click();
  await page.locator('[data-testid="docker-edit-env-key"]').last().fill('B');
  await page.locator('[data-testid="docker-edit-env-value"]').last().fill('2');
  await showTab(page, 'inplace');

  await expect(
    page.locator('[data-testid="docker-edit-pending"][data-mode="now"]'),
  ).not.toHaveCount(0);
  await expect(
    page.locator('[data-testid="docker-edit-pending"][data-mode="recreate"]'),
  ).toHaveCount(1);
  await expect(
    page.locator('[data-testid="docker-edit-pending"][data-mode="now"]', {
      hasText: /Memory(?! reservation)/,
    }),
  ).toHaveCount(1);

  await tid(page, 'docker-edit-apply-now').click();
  await expect.poll(() => docker.calls('UpdateContainer').length).toBe(1);
  expect(withoutIdentity(calls[0])).toEqual(withoutIdentity(sent));
  expect((calls[0] as Json).baseHash).toBe('hash-1');

  await expect(page.locator('[data-testid="docker-edit-pending"][data-mode="now"]')).toHaveCount(0);
  await expect(
    page.locator('[data-testid="docker-edit-pending"][data-mode="recreate"]'),
  ).toHaveCount(1);
  await expect(tid(page, 'docker-edit-stale')).toHaveCount(0);
});

test('clearing a memory limit moves it to recreate', async ({ relaunch }) => {
  const { page } = await setup(relaunch);
  await openEdit(page);

  await tid(page, 'docker-edit-memory').fill('');
  await expect(
    field(page, 'memory').locator('[data-testid="docker-edit-field-recreate-hint"]'),
  ).toBeVisible();
  await expect(tid(page, 'docker-edit-badge')).toHaveCount(0);
  const line = page.locator('[data-testid="docker-edit-pending"][data-mode="recreate"]', {
    hasText: /Memory(?! reservation)/,
  });
  await expect(line).toHaveCount(1);
  await expect(tid(page, 'docker-edit-apply-now')).toBeDisabled();
});

test('contract: recreate confirms losses, then selects the new container', async ({ relaunch }) => {
  const sent = contract<Json>('docker-edit', 'args:DockerService.RecreateContainer');
  const result = contract<Json>('docker-edit', 'DockerService.RecreateContainer');
  const spec = { ...specFixture(), origin: 'compose', originName: 'proj' };
  const calls: unknown[] = [];
  const ctx: { state?: { spec: Json; containers: Json[] } } = {};
  const { page, docker, state } = await setup(relaunch, {
    spec,
    handlers: {
      RecreateContainer: (a) => {
        calls.push(a);
        if (ctx.state) {
          ctx.state.containers = [row(NEW_ID, 'kira-flow-edit')];
          ctx.state.spec = {
            ...spec,
            id: NEW_ID,
            baseHash: 'hash-9',
            recreate: (a as Json).recreate,
          };
        }
        return { ...result, id: NEW_ID, oldId: OLD_ID, name: 'kira-flow-edit' };
      },
    },
  });
  ctx.state = state;
  await openEdit(page);
  await showTab(page, 'recreate');

  await tid(page, 'docker-edit-image').fill(sent.recreate.image);
  await page.locator('[data-testid="docker-edit-env"] [data-testid="docker-edit-row-add"]').click();
  await page.locator('[data-testid="docker-edit-env-key"]').last().fill('B');
  await page.locator('[data-testid="docker-edit-env-value"]').last().fill('2');
  await page
    .locator('[data-testid="docker-edit-labels"] [data-testid="docker-edit-row-add"]')
    .click();
  await page.locator('[data-testid="docker-edit-label-key"]').last().fill('edit2');
  await page.locator('[data-testid="docker-edit-label-value"]').last().fill('yes');
  await page
    .locator('[data-testid="docker-edit-ports"] [data-testid="docker-edit-row-remove"]')
    .click();
  await page
    .locator('[data-testid="docker-edit-ports"] [data-testid="docker-edit-row-add"]')
    .click();
  await tid(page, 'docker-edit-port-container').fill('8081');
  await tid(page, 'docker-edit-cmd').fill(sent.recreate.cmd.join('\n'));

  await tid(page, 'docker-edit-apply-recreate').click();
  const dialog = tid(page, 'docker-edit-dialog');
  await expect(dialog).toBeVisible();
  await expect(tid(page, 'docker-edit-lost-layer')).toBeVisible();
  await expect(tid(page, 'docker-edit-lost-volume')).toContainText('anon-volume');
  await expect(tid(page, 'docker-edit-compose')).toContainText('proj');

  await tid(page, 'docker-edit-cancel').click();
  await expect(dialog).toHaveCount(0);
  expect(docker.calls('RecreateContainer')).toHaveLength(0);

  await tid(page, 'docker-edit-apply-recreate').click();
  await tid(page, 'docker-edit-confirm').click();
  await expect.poll(() => docker.calls('RecreateContainer').length).toBe(1);
  expect(withoutIdentity(calls[0])).toEqual(withoutIdentity(sent));

  await expect(tid(page, 'docker-detail-id')).toContainText(NEW_ID.slice(0, 12));
  await expect(tid(page, 'docker-edit')).toBeVisible();
  await expect(tid(page, 'docker-edit-pending')).toHaveCount(0);
});

test('contract: failures keep the draft and explain the state', async ({ relaunch }) => {
  const rollback = contract<{ code: string; details: Json; message: string }>(
    'docker-edit',
    'DockerService.RecreateContainer#rollback',
  );
  const stale = contract<{ code: string; message: string }>(
    'docker-edit',
    'DockerService.UpdateContainer#stale',
  );
  const { page, docker } = await setup(relaunch, {
    handlers: {
      UpdateContainer: () => ({ error: { code: stale.code, message: stale.message } }),
      RecreateContainer: () => ({
        error: { code: rollback.code, message: 'start: no such file', details: rollback.details },
      }),
    },
  });
  await openEdit(page);

  await tid(page, 'docker-edit-name').fill('kira-flow-other');
  await tid(page, 'docker-edit-apply-now').click();
  await expect(tid(page, 'docker-edit-stale')).toBeVisible();
  await expect(tid(page, 'docker-edit-error')).toContainText('Refresh the editor');
  await tid(page, 'docker-edit-reload').click();
  await expect(tid(page, 'docker-edit-stale')).toHaveCount(0);
  await expect(tid(page, 'docker-edit-name')).toHaveValue('kira-flow-edit');
  await expect(tid(page, 'docker-edit-pending')).toHaveCount(0);

  await showTab(page, 'recreate');
  await tid(page, 'docker-edit-entrypoint').fill('/nonexistent');
  await tid(page, 'docker-edit-apply-recreate').click();
  await tid(page, 'docker-edit-confirm').click();
  await expect.poll(() => docker.calls('RecreateContainer').length).toBe(1);
  await expect(tid(page, 'docker-edit-error')).toContainText('original container was restored');
  await expect(tid(page, 'docker-detail-id')).toContainText(OLD_ID.slice(0, 12));
  await showTab(page, 'recreate');
  await expect(tid(page, 'docker-edit-entrypoint')).toHaveValue('/nonexistent');
});

test('contract: a managed container is read-only', async ({ relaunch }) => {
  const spec = specFixture('#managed');
  expect(spec.managed).toBe('kubernetes');
  const { page } = await setup(relaunch, { spec, name: 'kira-flow-managed' });
  await openEdit(page, 'kira-flow-managed');

  await expect(tid(page, 'docker-edit-managed')).toBeVisible();
  await expect(tid(page, 'docker-edit-name')).toBeDisabled();
  await expect(tid(page, 'docker-edit-apply-now')).toHaveCount(0);
  await showTab(page, 'recreate');
  await expect(tid(page, 'docker-edit-image')).toBeDisabled();
  await expect(tid(page, 'docker-edit-apply-recreate')).toHaveCount(0);
});

test('contract: a stopped container applies in place and recreate keeps it stopped', async ({
  relaunch,
}) => {
  const spec = specFixture('#stopped');
  expect(spec.state).toBe('exited');
  const sent = contract<Json>('docker-edit', 'args:DockerService.UpdateContainer#stopped');
  const result = contract<Json>('docker-edit', 'DockerService.UpdateContainer#stopped');
  const calls: unknown[] = [];
  const ctx: { state?: { spec: Json } } = {};
  const { page, docker, state } = await setup(relaunch, {
    spec,
    handlers: {
      UpdateContainer: (a) => {
        calls.push(a);
        if (ctx.state)
          ctx.state.spec = { ...ctx.state.spec, baseHash: 'hash-2', inPlace: (a as Json).spec };
        return result;
      },
    },
  });
  ctx.state = state;
  state.containers = [row(OLD_ID, 'kira-flow-edit', { state: 'exited', status: 'Exited (0)' })];
  await openEdit(page);

  await tid(page, 'docker-edit-memory').fill(String(sent.spec.resources.memory / 1024 ** 2));
  await tid(page, 'docker-edit-swap').fill(String(sent.spec.resources.memorySwap / 1024 ** 2));
  await tid(page, 'docker-edit-restart').selectOption(sent.spec.restart.name);
  await tid(page, 'docker-edit-apply-now').click();
  await expect.poll(() => docker.calls('UpdateContainer').length).toBe(1);
  expect(withoutIdentity(calls[0])).toEqual(withoutIdentity(sent));
  await expect(page.locator('[data-testid="docker-edit-pending"]')).toHaveCount(0);

  await showTab(page, 'recreate');
  await page.locator('[data-testid="docker-edit-env"] [data-testid="docker-edit-row-add"]').click();
  await page.locator('[data-testid="docker-edit-env-key"]').last().fill('B');
  await page.locator('[data-testid="docker-edit-env-value"]').last().fill('2');
  await tid(page, 'docker-edit-apply-recreate').click();
  await expect(tid(page, 'docker-edit-kept')).toContainText('The new container stays stopped.');
  await tid(page, 'docker-edit-cancel').click();
  expect(docker.calls('RecreateContainer')).toHaveLength(0);
});

test('contract: an auto-remove container is never recreated', async ({ relaunch }) => {
  const spec = specFixture('#auto-remove');
  expect(spec.autoRemove).toBe(true);
  const refusal = contract<{ code: string }>(
    'docker-edit',
    'DockerService.RecreateContainer#auto-remove',
  );
  expect(refusal.code).toBe('E_BAD_REQUEST');
  const { page, docker } = await setup(relaunch, { spec, name: 'kira-flow-auto' });
  await openEdit(page, 'kira-flow-auto');

  await expect(tid(page, 'docker-edit-autoremove')).toBeVisible();
  await showTab(page, 'recreate');
  await tid(page, 'docker-edit-image').fill('alpine:3.21');
  await expect(tid(page, 'docker-edit-apply-recreate')).toBeDisabled();
  expect(docker.calls('RecreateContainer')).toHaveLength(0);
});
