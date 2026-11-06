import { installDockerMocks } from '../../../../../packages/docker-ui/src/testing/ui/dockerMock';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { openContainer, openDocker } from './support/dockerFixture';

const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge';
const GB = 1024 ** 3;

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
    containers: 2,
    running: 2,
    paused: 0,
    stopped: 0,
    images: 1,
  },
};

function container(id: string, name: string, extra: Record<string, unknown> = {}) {
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

function detail(c: ReturnType<typeof container>) {
  return {
    container: c,
    command: ['sh'],
    entrypoint: [],
    env: [],
    workingDir: '/',
    user: '',
    restartPolicy: 'no',
    health: '',
    startedAt: '2026-01-01T00:00:00Z',
    finishedAt: '0001-01-01T00:00:00Z',
    exitCode: 0,
    tty: false,
    mounts: [],
    labels: {},
    networkAttachments: [],
    raw: '{"Id":"x"}',
  };
}

async function setup(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  handlers: Record<string, (args: unknown) => unknown> = {},
  containers = [container('c-a', 'alpha'), container('c-b', 'beta')],
) {
  const { window: page } = await relaunch({ control: [] });
  const docker = await installDockerMocks(page, {
    bridgePkg: BRIDGE_PKG,
    handlers: {
      Status: () => OK_STATUS,
      Containers: () => containers,
      Images: () => [],
      Volumes: () => [],
      Networks: () => [],
      InspectContainer: (a) => {
        const c = containers.find((x) => x.id === (a as { id: string }).id);
        return c ? detail(c) : { error: { code: 'E_NOT_FOUND', message: 'no such container' } };
      },
      ...handlers,
    },
  });
  return { page, docker };
}

test('contract: engine disk usage is measured only on demand and persists its last result', async ({
  relaunch,
}) => {
  const df = {
    ...contract<Record<string, unknown>>('docker-disk', 'DockerService.DiskUsage'),
    takenAt: new Date().toISOString(),
    total: 6.5 * GB,
    images: { count: 3, active: 2, size: 3 * GB, reclaimable: GB },
    containers: { count: 2, active: 2, size: 0.5 * GB, reclaimable: 0 },
    volumes: { count: 2, active: 1, size: 2 * GB, reclaimable: 0 },
    buildCache: { count: 0, active: 0, size: GB, reclaimable: GB },
    volumeSizes: [
      { name: 'pgdata', size: 2 * GB, refCount: 1 },
      { name: 'scratch', size: -1, refCount: 0 },
    ],
  };
  const state = { fail: false };
  const { page, docker } = await setup(relaunch, {
    DiskUsage: () => (state.fail ? { error: { code: 'E_INTERNAL', message: 'df failed' } } : df),
  });
  await openDocker(page);

  const measure = page.locator('[data-testid="docker-disk-measure"]');
  await expect(measure).toBeVisible();
  await page.locator('[data-testid="docker-refresh"]').click();
  await page.locator('[data-testid="docker-overview-refresh"]').click();
  await expect(measure).toBeVisible();
  expect(docker.calls('DiskUsage')).toHaveLength(0);

  await measure.click();
  await expect(page.locator('[data-testid="docker-disk-total"]')).toHaveText('6.5 GB');
  await expect(page.locator('[data-testid="docker-disk-images"]')).toHaveText('3.0 GB');
  await expect(page.locator('[data-testid="docker-disk-build-cache"]')).toHaveText('1.0 GB');
  const volumes = page.locator('[data-testid="docker-disk-volume"]');
  await expect(volumes).toHaveCount(2);
  await expect(volumes.nth(0)).toContainText('2.0 GB');
  await expect(volumes.nth(1)).toContainText('scratch');
  await expect(page.locator('[data-testid="docker-disk-taken"]')).toBeVisible();
  expect(docker.calls('DiskUsage')).toHaveLength(1);

  await page.reload();
  await openDocker(page);
  await expect(page.locator('[data-testid="docker-disk-total"]')).toHaveText('6.5 GB');
  await expect(page.locator('[data-testid="docker-disk-taken"]')).toBeVisible();
  expect(docker.calls('DiskUsage')).toHaveLength(1);

  await page.locator('[data-testid="docker-disk-refresh"]').click();
  await expect.poll(() => docker.calls('DiskUsage').length).toBe(2);

  state.fail = true;
  await page.locator('[data-testid="docker-disk-refresh"]').click();
  await expect(page.locator('[data-testid="docker-disk-error"]')).toContainText('df failed');
  await expect(page.locator('[data-testid="docker-disk-total"]')).toHaveText('6.5 GB');
});

test('contract: container size is measured on demand and cached for the session', async ({
  relaunch,
}) => {
  const size = {
    ...contract<Record<string, unknown>>('docker-disk', 'DockerService.ContainerSize'),
    takenAt: new Date().toISOString(),
    sizeRw: 512 * 1024,
    sizeRootFs: 8 * 1024 ** 2,
  };
  const { page, docker } = await setup(relaunch, { ContainerSize: () => size });
  await openDocker(page);
  await openContainer(page, 'alpha', 'stats');

  const measure = page.locator('[data-testid="docker-size-measure"]');
  await expect(measure).toBeVisible();
  expect(docker.calls('ContainerSize')).toHaveLength(0);

  await measure.click();
  await expect(page.locator('[data-testid="docker-size-rw"]')).toHaveText('512 KB');
  await expect(page.locator('[data-testid="docker-size-rootfs"]')).toHaveText('8.0 MB');
  expect(docker.calls('ContainerSize')).toHaveLength(1);
  expect(docker.calls('ContainerSize')[0].args).toEqual({ id: 'c-a' });

  await openContainer(page, 'beta', 'stats');
  await expect(page.locator('[data-testid="docker-size-measure"]')).toBeVisible();
  await openContainer(page, 'alpha', 'stats');
  await expect(page.locator('[data-testid="docker-size-rw"]')).toHaveText('512 KB');
  expect(docker.calls('ContainerSize')).toHaveLength(1);

  await page.locator('[data-testid="docker-size-refresh"]').click();
  await expect.poll(() => docker.calls('ContainerSize').length).toBe(2);

  await page.reload();
  await openDocker(page);
  await openContainer(page, 'alpha', 'stats');
  await expect(page.locator('[data-testid="docker-size-measure"]')).toBeVisible();
});

test('container size and the stopped notice show in Stats of a stopped container', async ({
  relaunch,
}) => {
  const { page } = await setup(relaunch, {}, [
    container('c-a', 'alpha'),
    container('c-s', 'sleepy', { state: 'exited', status: 'Exited (0) 1 hour ago' }),
  ]);
  await openDocker(page);
  await openContainer(page, 'sleepy', 'stats');

  await expect(page.locator('[data-testid="docker-stats-stopped"]')).toBeVisible();
  await expect(page.locator('[data-testid="docker-size-measure"]')).toBeVisible();
  await page.locator('[data-testid="docker-tab-overview"]').click();
  await expect(page.locator('[data-testid="docker-size-measure"]')).toHaveCount(0);
});

test('contract: left-bar origin icons', async ({ relaunch }) => {
  const rows = contract<
    Array<{
      name: string;
      origin: string;
      originName: string;
      composeProject: string;
      composeService: string;
    }>
  >('docker-disk', 'DockerService.Containers#origins');
  const containers = rows.map((r, i) => container(`c-${i}`, r.name, r));
  const { page } = await setup(relaunch, {}, containers);
  await openDocker(page);

  const groups = page.locator('[data-testid="docker-group"]');
  await expect(groups).toHaveCount(2);
  for (const g of await groups.all()) {
    await expect(g.locator('[data-testid="docker-group-icon"][data-origin="compose"]')).toHaveCount(
      1,
    );
  }

  for (const r of rows) {
    const icon = page.locator(
      `[data-testid="docker-row"][data-name="${r.name}"] [data-testid="docker-origin-icon"]`,
    );
    if (r.origin === '' || (r.origin === 'compose' && r.composeProject !== '')) {
      await expect(icon).toHaveCount(0);
    } else {
      await expect(icon).toHaveAttribute('data-origin', r.origin);
    }
  }
  expect(rows.some((r) => r.origin === 'devcontainer' && r.composeProject !== '')).toBe(true);
});
