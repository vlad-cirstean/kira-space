import type { Page } from '@playwright/test';
import { scriptRunSchema } from '@shared/domain/scriptRuns';
import { customScriptSchema } from '@shared/domain/scripts';
import { replaceCodeEditorText } from '@workbench/testing/ui/codeEditor';
import { installDockerMocks } from '../../../../packages/docker-ui/src/testing/ui/dockerMock';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { modeTab } from './support/apiMode';
import { contract } from './support/contract';
import { containerDetail } from './support/dockerFixture';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// P267 C9: behaviour the UI leftovers pass unified. Docker logs and rows, the script dialog and run
// log, and the tab strip's keyboard move.

const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge';

const STATUS = {
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
    running: 2,
    paused: 0,
    stopped: 0,
    images: 1,
  },
};

function container(id: string, name: string) {
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
  };
}

const CONTAINERS = [container('c-a', 'alpha'), container('c-b', 'bravo')];

async function setupDocker(relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch']) {
  const { window: page } = await relaunch({ control: [] });
  const docker = await installDockerMocks(page, {
    bridgePkg: BRIDGE_PKG,
    handlers: {
      Status: () => STATUS,
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
      Inspect: () => ({ raw: '{"Id":"x"}' }),
    },
  });
  await modeTab(page, 'docker').click();
  await expect(page.locator('[data-testid="docker-panel"]')).toBeVisible();
  return { page, docker };
}

const row = (page: Page, name: string) =>
  page.locator(`[data-testid="docker-row"][data-name="${name}"]`);

async function openLogs(page: Page, docker: Awaited<ReturnType<typeof setupDocker>>['docker']) {
  await row(page, 'alpha').dblclick();
  await expect(page.locator('[data-testid="docker-logs"]')).toBeVisible();
  await expect.poll(() => docker.calls('LogsOpen').length).toBeGreaterThan(0);
  return (docker.calls('LogsOpen')[0].args as { streamId: string }).streamId;
}

test('docker logs: ANSI colour renders as styled spans', async ({ relaunch }) => {
  const { page, docker } = await setupDocker(relaunch);
  const streamId = await openLogs(page, docker);
  await emitWailsEvent(page, 'kira:docker:logs', {
    streamId,
    lines: [{ stream: 'stdout', text: '\u001b[31mred\u001b[0m plain' }],
    ended: false,
  });
  const line = page.locator('[data-testid="docker-log-line"]').first();
  await expect(line).toContainText('red plain');
  await expect(line.locator('span', { hasText: 'red' }).first()).toHaveAttribute('style', /color/);
});

test('docker logs: Find marks every match and prev/next walk them', async ({ relaunch }) => {
  const { page, docker } = await setupDocker(relaunch);
  const streamId = await openLogs(page, docker);
  await emitWailsEvent(page, 'kira:docker:logs', {
    streamId,
    lines: [
      { stream: 'stdout', text: 'boom one' },
      { stream: 'stdout', text: 'quiet' },
      { stream: 'stdout', text: 'boom two' },
    ],
    ended: false,
  });
  await page.locator('[data-testid="docker-logs-find"]').click();
  await page.locator('[data-testid="docker-logs-find-input"]').fill('boom');
  const marks = page.locator('[data-testid="docker-log-line"] mark');
  await expect(marks).toHaveCount(2);
  const count = page.locator('[data-testid="docker-logs-find-count"]');
  await expect(count).toHaveText('Match 1 of 2');
  await page.locator('[data-testid="docker-logs-find-next"]').click();
  await expect(count).toHaveText('Match 2 of 2');
  await page.locator('[data-testid="docker-logs-find-prev"]').click();
  await expect(count).toHaveText('Match 1 of 2');
});

test('docker rows: Enter opens the Logs tab, right-click selects the container', async ({
  relaunch,
}) => {
  const { page } = await setupDocker(relaunch);

  await row(page, 'bravo').click({ button: 'right' });
  await expect(row(page, 'bravo')).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Escape');

  await row(page, 'alpha').focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('[data-testid="docker-container-detail"]')).toBeVisible();
  await expect(page.locator('[data-testid="docker-logs"]')).toBeVisible();
});

const SCRIPT_CONTROL: ControlSnapshot[] = [
  {
    channel: IPC.customScriptsList,
    response: {
      collections: [],
      scripts: [
        customScriptSchema.parse({
          id: 'draft',
          name: 'draft',
          kind: 'script',
          command: 'true',
          params: [],
          smart: null,
          schedule: null,
          dirMode: 'kira',
          workingDir: '',
          color: 'blue',
          collection: '',
          useAdeDir: false,
          sortOrder: 0,
          createdAt: '2026-01-01T00:00:00Z',
          updatedAt: '2026-01-01T00:00:00Z',
          savedSecrets: [],
        }),
      ],
    },
  },
  {
    channel: IPC.customScriptsFile,
    response: {
      file: 'draft.yaml',
      id: 'draft',
      path: '/kira/runs/draft.yaml',
      text: 'name: draft\n',
      hash: 'h1',
      problem: null,
      lastGood: false,
      editable: true,
    },
  },
  { channel: IPC.scriptRunsNextFires, response: [1_900_000_000_000] },
];

test('run file dialog: Escape on unsaved edits asks to discard first', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: SCRIPT_CONTROL });
  await modeTab(page, 'automations').click();
  await page.locator('[data-testid="script-draft"]').dblclick();
  const dialog = page.locator('[data-testid="run-file-dialog"]');
  await expect(dialog).toBeVisible();
  await dialog.locator('[data-testid="run-file-tab-yaml"]').click();

  await replaceCodeEditorText(
    page,
    dialog.locator('[data-testid="run-file-text"]'),
    'name: changed\n',
  );
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="confirm-dialog"]')).toBeVisible();
  await page.locator('[data-testid="confirm-dialog-cancel"]').click();
  await expect(dialog).toBeVisible();

  await page.keyboard.press('Escape');
  await page.locator('[data-testid="confirm-dialog-confirm"]').click();
  await expect(dialog).toHaveCount(0);
});

test('contract: the run log keeps ANSI colour', async ({ relaunch }) => {
  const script = contract('schedule-confirm', 'CustomScriptsService.Create', {
    schema: customScriptSchema,
  });
  const preview = contract<{ dir: { path: string } }>(
    'schedule-confirm',
    'ScriptRunsService.SchedulePreview',
  );
  const done = contract('schedule-confirm', 'ScriptRunsService.Get#done', {
    schema: scriptRunSchema,
  });
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [script] } },
      { channel: IPC.scriptRunsResolveDir, response: preview.dir },
      { channel: IPC.scriptRunsNextFires, response: [1_900_000_000_000] },
      { channel: IPC.scriptRunsSchedulePreview, response: preview },
      {
        channel: IPC.scriptRunsReadLog,
        response: { chunks: [{ text: '\u001b[32mgreen\u001b[0m tail' }], truncated: false },
      },
    ],
  });
  await modeTab(page, 'automations').click();
  await emitWailsEvent(page, IPC.scriptRunsChanged, {
    ...done,
    createdAt: 2_000,
    startedAt: 2_000,
    finishedAt: 3_000,
  });
  await page.locator('[data-testid="toggle-operations-panel"]').click();
  await page.locator(`[data-op-id="run:${done.id}"]`).click({ button: 'right' });
  await page.locator('[data-testid="menu-item-open-run"]').click();
  const log = page.locator('[data-testid="script-run-view"] [data-testid="run-log"]');
  await expect(log).toContainText('green tail');
  await expect(log.locator('span', { hasText: 'green' }).first()).toHaveAttribute('style', /color/);
});

test('tab strip: Alt+ArrowRight moves the focused tab one place', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } }],
  });
  await modeTab(page, 'automations').click();
  for (let i = 0; i < 2; i++) {
    await page.locator('[data-testid="tab-strip-new"]').click();
    await page.locator('[data-testid="menu-item-new-terminal"]').click();
  }
  const chips = page.locator('[data-testid="tab-strip-row"] [data-testid="tab"]');
  await expect(chips).toHaveCount(2);
  const order = () => chips.evaluateAll((els) => els.map((el) => el.getAttribute('data-tab-id')));
  const [first, second] = (await order()) as string[];

  await chips.first().locator('button').first().focus();
  await page.keyboard.press('Alt+ArrowRight');
  await expect.poll(order).toEqual([second, first]);
});
