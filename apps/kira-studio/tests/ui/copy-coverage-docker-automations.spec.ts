import type { Locator, Page } from '@playwright/test';
import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { installDockerMocks } from '../../../../packages/docker-ui/src/testing/ui/dockerMock';
import {
  expectCatalogText,
  setPseudoLocale,
} from '../../../../packages/workbench/src/testing/ui/copyCoverage';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { modeTab } from './support/apiMode';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P265 Stream C: every chrome string on the Docker and Automations surfaces comes from the catalog.
// The pseudo-locale wraps catalog messages; fixture data is allowed by pattern.

const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge';
const OLD_ID = 'c-cover-0001';

// biome-ignore lint/suspicious/noExplicitAny: contract fixtures are untyped JSON
type Json = Record<string, any>;

const tid = (page: Page, id: string): Locator => page.locator(`[data-testid="${id}"]`);

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
    containers: 2,
    running: 1,
    paused: 0,
    stopped: 1,
    images: 1,
  },
};

function container(id: string, name: string, state: string): Json {
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

const CONTAINERS = [container(OLD_ID, 'solo', 'running'), container('c-old', 'old-job', 'exited')];

const IMAGES = [
  {
    id: 'sha256:img1',
    tags: ['alpine:3.20'],
    size: 8_000_000,
    created: 1_700_000_000,
    containers: 1,
    dangling: false,
  },
];
const VOLUMES = [
  {
    name: 'pgdata',
    driver: 'local',
    mountpoint: '/var/lib/docker/volumes/pgdata',
    scope: 'local',
    created: '',
    labels: {},
    usedBy: [OLD_ID],
  },
];
const NETWORKS = [
  {
    id: 'net1',
    name: 'shop_default',
    driver: 'bridge',
    scope: 'local',
    internal: false,
    subnets: ['172.18.0.0/16'],
    containers: 1,
    builtin: false,
    usedBy: [OLD_ID],
  },
];

function detail(c: Json): Json {
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

// Values that are data, not chrome: container, image, volume and network names, engine facts, sizes,
// timestamps and the status text the engine reports.
const DOCKER_DATA = [
  /solo|old-job|alpine|sha256|pgdata|shop_default|bridge|unix:|docker\.sock|hub\.docker/,
  /^(Up|Exited)\b|^(Debian|linux|amd64|local|default)$/,
  /^(running|exited|paused|restarting|created|dead|no|always|on-failure|unless-stopped|tcp|udp|sctp|volume|bind)$/,
  /^workdir\b/,
  /^\s*[\d.,]+\s*(B|KB|MB|GB|kB|%|ms|s)?\s*$/,
  /\b\d+(\.\d+)?\s?(B|KB|MB|GB|kB)\b/,
  /\bago\b|\bUTC\b|\d{4}-\d{2}-\d{2}/,
];

async function setupDocker(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
): Promise<Page> {
  const { window: page } = await relaunch({ control: [] });
  await installDockerMocks(page, {
    bridgePkg: BRIDGE_PKG,
    handlers: {
      Status: () => OK_STATUS,
      Containers: () => CONTAINERS,
      Images: () => IMAGES,
      Volumes: () => VOLUMES,
      Networks: () => NETWORKS,
      InspectContainer: (a) => {
        const c = CONTAINERS.find((x) => x.id === (a as { id: string }).id);
        return c ? detail(c) : { error: { code: 'E_NOT_FOUND', message: 'no such container' } };
      },
      Inspect: () => ({ raw: '{"Id":"x"}' }),
      ContainerEditSpec: () => ({
        ...contract<Json>('docker-edit', 'DockerService.ContainerEditSpec'),
        id: OLD_ID,
        baseHash: 'hash-1',
      }),
    },
  });
  await setPseudoLocale(page);
  await modeTab(page, 'docker').click();
  await expect(tid(page, 'docker-panel')).toBeVisible();
  return page;
}

test('Docker panel lists use catalog text', async ({ relaunch }) => {
  const page = await setupDocker(relaunch);
  const panel = tid(page, 'docker-panel');

  await expect(tid(page, 'docker-row').first()).toBeVisible();
  await expectCatalogText(panel, { allow: DOCKER_DATA });

  await tid(page, 'toggle-search').click();
  await tid(page, 'tree-search').fill('no-such-container');
  await expect(tid(page, 'docker-list-empty')).toBeVisible();
  await expectCatalogText(panel, { allow: DOCKER_DATA });
  await tid(page, 'toggle-search').click();

  for (const section of ['images', 'volumes', 'networks']) {
    await tid(page, `docker-section-${section}`).click();
    await expect(tid(page, 'docker-row').first()).toBeVisible();
    await expectCatalogText(panel, { allow: DOCKER_DATA });
  }
});

test('container detail tabs and the container menu use catalog text', async ({ relaunch }) => {
  const page = await setupDocker(relaunch);

  await tid(page, 'docker-row').first().click({ button: 'right' });
  await expect(tid(page, 'context-menu')).toBeVisible();
  await expectCatalogText(tid(page, 'context-menu'));
  await page.keyboard.press('Escape');

  await tid(page, 'docker-row').first().click();
  const view = tid(page, 'docker-container-detail');
  await expect(view).toBeVisible();
  for (const tab of ['overview', 'logs', 'stats', 'inspect']) {
    await tid(page, `docker-tab-${tab}`).click();
    await expectCatalogText(view, { allow: DOCKER_DATA });
  }
});

test('container edit view and the recreate confirm use catalog text', async ({ relaunch }) => {
  const page = await setupDocker(relaunch);

  await page.locator('[data-testid="docker-row"][data-name="solo"]').click();
  await tid(page, 'docker-tab-edit').click();
  const edit = tid(page, 'docker-edit');
  await expect(tid(page, 'docker-edit-inplace')).toBeVisible();
  await expectCatalogText(edit, { allow: DOCKER_DATA });

  await tid(page, 'docker-edit-tab-recreate').click();
  await expect(tid(page, 'docker-edit-section-recreate')).toBeVisible();
  await expectCatalogText(edit, { allow: DOCKER_DATA });

  await tid(page, 'docker-edit-image').fill('alpine:3.21');
  await tid(page, 'docker-edit-apply-recreate').click();
  await expect(tid(page, 'docker-edit-dialog')).toBeVisible();
  await expectCatalogText(tid(page, 'docker-edit-dialog'), { allow: DOCKER_DATA });
});

const SCHEDULE = {
  cron: '0 9 * * 1-5',
  timezone: 'UTC',
  enabled: true,
  confirm: true,
  timeout: '30m',
  params: {},
  taskId: '',
  branchId: '',
};

const SCRIPT = {
  id: 'rec-1',
  kind: 'script',
  name: 'Nightly build',
  command: 'make build {topic}',
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
  schedule: SCHEDULE,
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
  path: '/kira/automations/rec-1',
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
  timeout: '30m',
  tools: [],
  allowedTools: [],
  mcpServers: [],
  hash: 'h1',
  needs: { tasks: [], branches: [] },
  ade: null,
};

function run(id: string, state: string, createdAt: number): Json {
  const ended = state !== 'running';
  return {
    id,
    scriptId: SCRIPT.id,
    scriptName: SCRIPT.name,
    color: 'blue',
    kind: 'script',
    trigger: 'terminal',
    state,
    terminalId: `t-${id}`,
    cwd: DIR.path,
    command: 'make build',
    outcome: ended ? { status: state, reason: 'finished', source: 'exit', reported: false } : null,
    createdAt,
    startedAt: createdAt,
    finishedAt: ended ? createdAt + 1_000 : null,
    model: '',
    sessionId: '',
    prompt: '',
    params: [],
    tools: { tools: [], allowedTools: [], mcpServers: [] },
    taskId: '',
    taskTitle: '',
    branchId: '',
    branchLabel: '',
  };
}

const AUTOMATIONS: ControlSnapshot[] = [
  { channel: IPC.customScriptsList, response: { collections: [], scripts: [SCRIPT] } },
  {
    channel: IPC.customScriptsFile,
    response: {
      file: `${SCRIPT.id}.yaml`,
      id: SCRIPT.id,
      path: `/kira/runs/${SCRIPT.id}.yaml`,
      text: 'name: x\n',
      hash: 'h1',
      problem: null,
      lastGood: false,
      editable: true,
    },
  },
  { channel: IPC.scriptRunsResolveDir, response: DIR },
  { channel: IPC.scriptRunsNextFires, response: [1_900_000_000_000, 1_900_086_400_000] },
  { channel: IPC.scriptRunsSchedulePreview, response: PREVIEW },
  { channel: IPC.scriptRunsPreview, response: PREVIEW },
  {
    channel: IPC.scriptRunsList,
    response: [
      run('r-c', 'running', 4_000),
      run('r-a', 'done', 3_000),
      run('r-b', 'failed', 2_000),
    ],
  },
  { channel: IPC.scriptRunsReadLog, response: { chunks: [], truncated: false } },
  { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
];

// Script, run and folder names and the times the schedule derives from the clock.
const AUTOMATIONS_DATA = [
  /Nightly build|make build|\/kira\/|\/home\/test|^\s*UTC\s*$|0 9 \* \* 1-5|^Topic$|^(red|amber|green|cyan|blue|magenta|grey)$|^\/automations\/$/,
  /\d{1,2}:\d{2}|\d{4}-\d{2}-\d{2}|\b\d+\s?(ms|s|m|h)\b/,
];

async function openAutomations(page: Page): Promise<void> {
  await setPseudoLocale(page);
  await modeTab(page, 'automations').click();
  await expect(tid(page, 'automations-panel')).toBeVisible();
}

test('Automations panel, row menu and runs section use catalog text', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: AUTOMATIONS });
  await openAutomations(page);

  await expectCatalogText(tid(page, 'automations-panel'), { allow: AUTOMATIONS_DATA });

  await expect(tid(page, 'run-row')).toHaveCount(1);
  await expectCatalogText(tid(page, 'runs-section'), { allow: AUTOMATIONS_DATA });

  await tid(page, 'toggle-operations-panel').click();
  await expect(page.locator('[data-testid="op-row"][data-op-id="run:r-b"]')).toBeVisible();
  await page.locator('[data-testid="op-row"][data-op-id="run:r-b"]').click();
  await expectCatalogText(tid(page, 'operations-panel'), { allow: AUTOMATIONS_DATA });
  await page.locator('[data-testid="op-row"][data-op-id="run:r-b"]').click({ button: 'right' });
  await expectCatalogText(tid(page, 'context-menu'));
  await page.keyboard.press('Escape');
  await tid(page, 'toggle-operations-panel').click();

  await tid(page, `script-${SCRIPT.id}`).click({ button: 'right' });
  await expect(tid(page, 'context-menu')).toBeVisible();
  await expectCatalogText(tid(page, 'context-menu'));
});

test('run file dialog uses catalog text', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: AUTOMATIONS });
  await openAutomations(page);

  await tid(page, `script-${SCRIPT.id}`).click({ button: 'right' });
  await tid(page, 'menu-item-edit-file').click();
  const dialog = tid(page, 'run-file-dialog');
  await expect(dialog).toBeVisible();
  await expectCatalogText(dialog, { allow: AUTOMATIONS_DATA });
});

test('run script dialog uses catalog text', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: AUTOMATIONS });
  await openAutomations(page);

  await tid(page, `script-${SCRIPT.id}`).locator('[data-testid="script-play"]').click();
  const dialog = tid(page, 'run-dialog');
  await expect(dialog).toBeVisible();
  await expectCatalogText(dialog, { allow: AUTOMATIONS_DATA });
});

test('tab strip menu uses catalog text', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: AUTOMATIONS });
  await openAutomations(page);

  await tid(page, 'tab-strip-new').click();
  await expectCatalogText(tid(page, 'context-menu'));
  await tid(page, 'menu-item-new-terminal').click();
  const tab = page.locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"]');
  await expect(tab).toHaveCount(1);

  await tab.first().click({ button: 'right' });
  await expect(tid(page, 'menu-item-close-all')).toBeVisible();
  await expectCatalogText(tid(page, 'context-menu'));
});

test('update dialog and prompt host use catalog text', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.updateStatus,
        response: {
          updateAvailable: true,
          currentVersion: '1.2.0',
          latestVersion: '1.3.0',
          installLogPath: '/tmp/kira-studio-update.log',
        },
      },
    ],
  });
  await setPseudoLocale(page);
  await emitPrompts(page, [routed('update', '1.3.0')]);

  const dialog = tid(page, 'update-dialog');
  await expect(dialog).toBeVisible();
  await expectCatalogText(dialog, { allow: [/kira-studio-update\.log/] });
});
