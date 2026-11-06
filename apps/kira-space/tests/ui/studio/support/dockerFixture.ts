import type { Page } from '@playwright/test';
import { installDockerMocks } from '../../../../../../packages/docker-ui/src/testing/ui/dockerMock';
import { expect, type studioTest as test } from '../../fixtures';
import { contract } from '../../support/contract';
import { modeTab } from './apiMode';

// biome-ignore lint/suspicious/noExplicitAny: contract fixtures are untyped JSON
type Json = Record<string, any>;

export function container(
  id: string,
  name: string,
  state: string,
  extra: Record<string, unknown> = {},
) {
  return {
    id,
    name,
    image: 'alpine:3.20',
    imageId: 'sha256:img1',
    state,
    status: state === 'running' ? 'Up 2 minutes' : 'Exited (0) 1 hour ago',
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

export function seedContainers() {
  return [
    container('c-web', 'shop-web-1', 'running', {
      composeProject: 'shop',
      composeService: 'web',
      origin: 'compose',
      originName: 'shop',
    }),
    container('c-db', 'shop-db-1', 'running', {
      composeProject: 'shop',
      composeService: 'db',
      origin: 'compose',
      originName: 'shop',
    }),
    container('c-solo', 'solo', 'running'),
    container('c-old', 'old-job', 'exited'),
  ];
}

export async function openDocker(page: Page): Promise<void> {
  await modeTab(page, 'docker').click();
  await expect(page.locator('[data-testid="docker-panel"]')).toBeVisible();
}

export async function openContainer(page: Page, name: string, tab?: string): Promise<void> {
  await page.locator(`[data-testid="docker-row"][data-name="${name}"]`).click();
  await expect(page.locator('[data-testid="docker-container-detail"]')).toBeVisible();
  if (tab) await page.locator(`[data-testid="docker-tab-${tab}"]`).click();
}

export const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge';

export const ENDPOINT = {
  context: 'default',
  host: 'unix:///var/run/docker.sock',
  source: 'default',
  secure: true,
  remote: false,
};

export const OK_STATUS = {
  state: 'ok',
  endpoint: ENDPOINT,
  engine: {
    version: '29.0.0',
    apiVersion: '1.52',
    os: 'linux',
    arch: 'amd64',
    operatingSystem: 'Debian',
    kernelVersion: '6.1',
    cpus: 8,
    memTotal: 16 * 1024 ** 3,
    containers: 4,
    running: 3,
    paused: 0,
    stopped: 1,
    images: 2,
  },
};

export const IMAGES = [
  {
    id: 'sha256:img1',
    tags: ['alpine:3.20'],
    size: 8_000_000,
    created: 1_700_000_000,
    containers: 2,
    dangling: false,
  },
  {
    id: 'sha256:img2',
    tags: ['local/app:dev'],
    size: 1_000_000,
    created: 1_700_000_000,
    containers: 0,
    dangling: false,
  },
];

export const VOLUMES = [
  {
    name: 'pgdata',
    driver: 'local',
    mountpoint: '/var/lib/docker/volumes/pgdata',
    scope: 'local',
    created: '',
    labels: {},
    usedBy: ['c-db'],
  },
];

export const NETWORKS = [
  {
    id: 'net1',
    name: 'shop_default',
    driver: 'bridge',
    scope: 'local',
    internal: false,
    subnets: ['172.18.0.0/16'],
    containers: 2,
    builtin: false,
    usedBy: ['c-web', 'c-db'],
  },
];

export function containerDetailWithSecret(c: ReturnType<typeof container>) {
  return {
    container: c,
    command: ['sh'],
    entrypoint: [],
    env: ['SECRET=hunter2'],
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

export async function setupDocker(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  opts: { status?: unknown; handlers?: Record<string, (args: unknown) => unknown> } = {},
) {
  const { window: page, control } = await relaunch({ control: [] });
  const state = {
    status: opts.status ?? OK_STATUS,
    containers: seedContainers(),
    restore: {
      state: 'none',
      engineId: '',
      context: '',
      detectedAt: 0,
      bootAt: 0,
      items: [],
    } as unknown,
  };
  const docker = await installDockerMocks(page, {
    bridgePkg: BRIDGE_PKG,
    handlers: {
      Status: () => state.status,
      Containers: () => state.containers,
      Images: () => IMAGES,
      Volumes: () => VOLUMES,
      Networks: () => NETWORKS,
      InspectContainer: (a) => {
        const c = state.containers.find((x) => x.id === (a as { id: string }).id);
        return c
          ? containerDetailWithSecret(c)
          : { error: { code: 'E_NOT_FOUND', message: 'no such container' } };
      },
      ImageRegistry: () => ({ url: '', unconfirmed: false }),
      ConfirmImageRegistry: () => ({ url: '', unconfirmed: false }),
      Inspect: () => ({ raw: '{"Id":"x"}' }),
      Stop: (a) => {
        const c = state.containers.find((x) => x.id === (a as { id: string }).id);
        if (c) c.state = 'exited';
      },
      RestoreState: () => state.restore,
      RestoreContainers: () => ({ items: [] }),
      RestoreDismiss: () => undefined,
      ExecOpen: () => contract('docker-exec', 'DockerService.ExecOpen'),
      ...opts.handlers,
    },
  });
  return { page, docker, state, control };
}

export function containerDetail(c: ReturnType<typeof container>) {
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

export function containerDetailBare(c: Json): Json {
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
