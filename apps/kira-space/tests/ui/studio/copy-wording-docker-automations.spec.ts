import type { Locator, Page } from '@playwright/test';
import { installDockerMocks } from '../../../../../packages/docker-ui/src/testing/ui/dockerMock';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { modeTab } from './support/apiMode';
import { containerDetailBare } from './support/dockerFixture';

// P265 Stream C: pins the unified wording of the Docker and Automations surfaces.

const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge';
const OLD_ID = 'c-wording-0001';

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
    memTotal: 16 * 1024 ** 3,
    containers: 1,
    running: 1,
    paused: 0,
    stopped: 0,
    images: 1,
  },
};

// biome-ignore lint/suspicious/noExplicitAny: contract fixtures are untyped JSON
type Json = Record<string, any>;

function row(id: string, name: string, state: string): Json {
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
  };
}

async function setupDocker(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  containers: Json[],
  extra: Record<string, (args: unknown) => unknown> = {},
) {
  const { window: page } = await relaunch({ control: [] });
  await installDockerMocks(page, {
    bridgePkg: BRIDGE_PKG,
    handlers: {
      Status: () => OK_STATUS,
      Containers: () => containers,
      Images: () => [],
      Volumes: () => [],
      Networks: () => [],
      InspectContainer: (a) => {
        const c = containers.find((x) => x.id === (a as { id: string }).id);
        return c
          ? containerDetailBare(c)
          : { error: { code: 'E_NOT_FOUND', message: 'no such container' } };
      },
      ContainerEditSpec: () => ({
        ...contract<Json>('docker-edit', 'DockerService.ContainerEditSpec'),
        id: OLD_ID,
        baseHash: 'hash-1',
      }),
      ...extra,
    },
  });
  await modeTab(page, 'docker').click();
  await expect(page.locator('[data-testid="docker-panel"]')).toBeVisible();
  return page;
}

const tid = (page: Page, id: string): Locator => page.locator(`[data-testid="${id}"]`);

test('container context menu labels and order', async ({ relaunch }) => {
  const page = await setupDocker(relaunch, [row(OLD_ID, 'solo', 'running')]);

  await tid(page, 'docker-row').click({ button: 'right' });
  const menu = tid(page, 'context-menu');
  await expect(menu).toBeVisible();
  await expect(menu.locator('[data-testid^="menu-item-"]')).toHaveText([
    'Start',
    'Stop',
    'Restart',
    'Logs',
    'Open terminal',
    'Copy ID',
    'Copy name',
  ]);
});

test('empty container list says what to do next', async ({ relaunch }) => {
  const page = await setupDocker(relaunch, []);

  const empty = tid(page, 'docker-panel').locator('[data-testid="docker-list-empty"]');
  await expect(empty).toContainText('No containers yet');
  await expect(empty).toContainText('Start one with docker run or docker compose up.');
});

// Deviation: the planned "delete container" confirm has no flow in the app. The destructive
// container confirm is the recreate confirm, so that one is pinned instead.
test('recreate confirm: title, body and buttons', async ({ relaunch }) => {
  const page = await setupDocker(relaunch, [row(OLD_ID, 'solo', 'running')]);

  await tid(page, 'docker-row').click();
  await tid(page, 'docker-tab-edit').click();
  await tid(page, 'docker-edit-tab-recreate').click();
  await tid(page, 'docker-edit-image').fill('alpine:3.21');
  await tid(page, 'docker-edit-apply-recreate').click();

  const dialog = tid(page, 'docker-edit-dialog');
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole('heading', { name: 'Recreate container?' })).toBeVisible();
  await expect(dialog).toContainText(
    'The engine can’t change these settings on an existing container. A new container replaces this one.',
  );
  await expect(tid(page, 'docker-edit-cancel')).toHaveText('Cancel');
  await expect(tid(page, 'docker-edit-confirm')).toHaveText('Recreate now');
});

const SCRIPT = {
  id: 'script-1',
  kind: 'script',
  params: [
    {
      name: 'topic',
      type: 'text',
      options: [],
      default: [],
      required: false,
      secret: false,
    },
  ],
  smart: null,
  name: 'Build all',
  command: 'make build {topic}',
  workingDir: '',
  dirMode: 'kira',
  useAdeDir: false,
  color: 'blue',
  collection: '',
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

const DIR = {
  path: '/kira/automations/script-1',
  mode: 'kira',
  base: '/kira',
  blocker: '',
  branch: '',
  pending: false,
};

const PREVIEW = {
  kind: 'script',
  missing: [],
  blocker: '',
  dir: DIR,
  body: SCRIPT.command,
  prompt: [],
  suffix: '',
  env: [],
  command: 'make build',
  model: '',
  maxBudgetUsd: 0,
  timeout: '',
  tools: [],
  allowedTools: [],
  mcpServers: [],
  hash: 'h1',
  needs: { tasks: [], branches: [] },
  ade: null,
};

const AUTOMATIONS: ControlSnapshot[] = [
  { channel: IPC.customScriptsList, response: { collections: [], scripts: [SCRIPT] } },
  { channel: IPC.scriptRunsResolveDir, response: DIR },
  { channel: IPC.scriptRunsPreview, response: PREVIEW },
  { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
];

async function openAutomations(page: Page): Promise<void> {
  await modeTab(page, 'automations').click();
  await expect(tid(page, 'automations-panel')).toBeVisible();
}

test('remove script confirm: title, body and buttons', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: AUTOMATIONS });
  await openAutomations(page);

  await tid(page, `script-${SCRIPT.id}`).click({ button: 'right' });
  await tid(page, 'menu-item-remove').click();

  const dialog = tid(page, 'confirm-dialog');
  await expect(dialog.getByRole('heading', { name: 'Remove script?' })).toBeVisible();
  await expect(tid(page, 'confirm-dialog-message')).toHaveText(
    '“Build all” will no longer appear in Runs.',
  );
  await expect(tid(page, 'confirm-dialog-cancel')).toHaveText('Cancel');
  await expect(tid(page, 'confirm-dialog-confirm')).toHaveText('Remove');
});

test('run script dialog primary action is Run', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: AUTOMATIONS });
  await openAutomations(page);

  await tid(page, `script-${SCRIPT.id}`).locator('[data-testid="script-play"]').click();
  await expect(tid(page, 'run-dialog')).toBeVisible();
  await expect(tid(page, 'run-start')).toHaveText('Run');
  await expect(tid(page, 'run-cancel')).toHaveText('Cancel');
});

test('tab strip menu offers Close, Close others, Close to the right, Close all', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: AUTOMATIONS });
  await openAutomations(page);

  await tid(page, 'tab-strip-new').click();
  await tid(page, 'menu-item-new-terminal').click();
  const tab = page.locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"]');
  await expect(tab).toHaveCount(1);

  await tab.first().click({ button: 'right' });
  await expect(tid(page, 'menu-item-close')).toContainText('Close');
  await expect(tid(page, 'menu-item-close-others')).toHaveText('Close others');
  await expect(tid(page, 'menu-item-close-to-the-right')).toHaveText('Close to the right');
  await expect(tid(page, 'menu-item-close-all')).toHaveText('Close all');
});
