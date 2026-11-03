import { installDockerMocks } from '../../../../packages/docker-ui/src/testing/ui/dockerMock';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { modeTab } from './support/apiMode';
import { container, containerDetail, openDocker } from './support/dockerFixture';
import { IPC } from './support/ipcChannels';

// P266 stream C. Backend half: internal/flows/{termflow,dockerflow}/errors_test.go return the
// same texts asserted here.

const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge';

const ENDPOINT = {
  context: 'default',
  host: 'unix:///var/run/docker.sock',
  source: 'default',
  secure: true,
  remote: false,
};
const OK_STATUS = {
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
    containers: 2,
    running: 1,
    paused: 0,
    stopped: 1,
    images: 1,
  },
};
const DOWN_STATUS = {
  state: 'unavailable',
  reason: 'daemon-down',
  message: 'connection refused',
  endpoint: ENDPOINT,
};

const CONTAINERS = [
  container('c-web', 'shop-web-1', 'running', {
    composeProject: 'shop',
    composeService: 'web',
    origin: 'compose',
    originName: 'shop',
  }),
  container('c-old', 'old-job', 'exited'),
];

async function setupDocker(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  handlers: Record<string, (args: unknown) => unknown> = {},
  status: unknown = OK_STATUS,
) {
  const { window: page } = await relaunch({ control: [] });
  await installDockerMocks(page, {
    bridgePkg: BRIDGE_PKG,
    handlers: {
      Status: () => status,
      Containers: () => CONTAINERS,
      Images: () => [],
      Volumes: () => [],
      Networks: () => [],
      InspectContainer: (a) => {
        const c = CONTAINERS.find((x) => x.id === (a as { id: string }).id);
        return c
          ? containerDetail(c)
          : { error: { code: 'E_NOT_FOUND', message: 'no such container' } };
      },
      ...handlers,
    },
  });
  return page;
}

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
  name: 'Dev server',
  command: 'npm run dev {topic}',
  workingDir: '/tmp/demo',
  dirMode: 'fixed',
  useAdeDir: false,
  color: 'green',
  collection: '',
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

const AUTOMATIONS_COMMON: ControlSnapshot[] = [
  {
    channel: IPC.scriptRunsResolveDir,
    response: {
      path: SCRIPT.workingDir,
      mode: 'fixed',
      base: '',
      blocker: '',
      branch: '',
      pending: false,
    },
  },
  { channel: IPC.scriptRunsNextFires, response: [] },
];

test('automations front door is a module start with one new-script action', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: [] });
  await modeTab(page, 'automations').click();

  const start = page.locator('[data-testid="automations-start"]');
  await expect(start).toBeVisible();
  await expect(start).toHaveAttribute('data-slot', 'module-start');
  await expect(start.locator('[data-testid="automations-start-new"]')).toBeVisible();
});

test('docker unavailable state is an empty state with a Retry button', async ({ relaunch }) => {
  const page = await setupDocker(relaunch, {}, DOWN_STATUS);
  await openDocker(page);

  const state = page.locator('[data-testid="docker-unavailable"]');
  await expect(state).toBeVisible();
  await expect(state).toHaveAttribute('data-slot', 'empty-state');
  await expect(state.locator('[data-testid="docker-retry"]')).toBeVisible();
});

test('docker origin icons are codicons, not lucide svgs', async ({ relaunch }) => {
  const page = await setupDocker(relaunch);
  await openDocker(page);

  const icon = page.locator('[data-testid="docker-group-icon"]').first();
  await expect(icon).toBeVisible();
  await expect(icon.locator('.codicon')).toBeVisible();
  // shadcn's checkbox draws its own check mark with lucide; every other glyph is a codicon.
  const stray = await page
    .locator('[data-testid="docker-panel"] svg.lucide')
    .evaluateAll((els) => els.filter((el) => !el.closest('[data-slot^="checkbox"]')).length);
  expect(stray).toBe(0);
});

test('a failed container start shows the catalog text from the backend', async ({ relaunch }) => {
  const page = await setupDocker(relaunch, {
    Start: () => ({
      error: { code: 'E_NOT_FOUND', message: 'That Docker item no longer exists.' },
    }),
  });
  await openDocker(page);
  await page.locator('[data-testid="docker-row"][data-name="old-job"]').click();
  await page.locator('[data-testid="docker-action-start"]').click();

  await expect(page.locator('[data-testid="docker-action-error"]')).toHaveText(
    'That Docker item no longer exists.',
  );
});

test('the run dialog shows the backend text when a task is refused in Studio', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      ...AUTOMATIONS_COMMON,
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [SCRIPT] } },
      {
        channel: IPC.scriptRunsPreview,
        error: { code: 'E_BAD_REQUEST', message: 'Tasks exist only in Kira Space.' },
      },
    ],
  });
  await modeTab(page, 'automations').click();
  await page.locator(`[data-testid="script-${SCRIPT.id}"] [data-testid="script-play"]`).click();

  await expect(page.locator('[data-testid="run-preview-error"]')).toHaveText(
    'Tasks exist only in Kira Space.',
  );
});
