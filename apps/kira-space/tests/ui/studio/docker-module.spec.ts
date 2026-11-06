import en from '../../../../../packages/theme/src/i18n/en.json';
import { expect, test } from './fixtures';
import { modeTab } from './support/apiMode';
import { contract } from './support/contract';
import {
  container,
  ENDPOINT,
  IMAGES,
  OK_STATUS,
  openContainer,
  openDocker,
  seedContainers,
  setupDocker,
} from './support/dockerFixture';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import { assertTooltipShows } from './support/tooltip';

const DOWN_STATUS = {
  state: 'unavailable',
  reason: 'daemon-down',
  message: 'connection refused',
  endpoint: ENDPOINT,
};

const execOpenArgs = () =>
  contract<Record<string, unknown>>('docker-exec', 'args:DockerService.ExecOpen');

test('daemon down shows the unavailable state; Retry refetches and renders the lists', async ({
  relaunch,
}) => {
  const { page, docker, state } = await setupDocker(relaunch, { status: DOWN_STATUS });
  await openDocker(page);

  await expect(page.locator('[data-testid="docker-unavailable"]')).toBeVisible();
  await expect(page.locator('[data-testid="docker-unavailable"]')).toContainText('isn’t running');

  state.status = OK_STATUS;
  await page.locator('[data-testid="docker-retry"]').click();

  await expect(page.locator('[data-testid="docker-container-list"]')).toBeVisible();
  expect(
    docker.calls('Status').some((c) => (c.args as { refresh?: boolean }).refresh === true),
  ).toBe(true);
});

test('containers group by Compose project; show-stopped and search filter the list', async ({
  relaunch,
}) => {
  const { page } = await setupDocker(relaunch);
  await openDocker(page);

  const rows = page.locator('[data-testid="docker-row"]');
  await expect(page.locator('[data-testid="docker-group"]')).toHaveCount(1);
  await expect(page.locator('[data-testid="docker-group"]')).toContainText('shop');
  await expect(rows).toHaveCount(4);

  await page.locator('[data-testid="docker-show-stopped"]').click();
  await expect(rows).toHaveCount(3);
  await expect(page.locator('[data-testid="docker-row"][data-name="old-job"]')).toHaveCount(0);

  await page.locator('[data-testid="toggle-search"]').click();
  await page.locator('[data-testid="tree-search"]').fill('db');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toHaveAttribute('data-name', 'shop-db-1');
});

test('Stop calls the bound method and the row updates after a changed event', async ({
  relaunch,
}) => {
  const { page, docker } = await setupDocker(relaunch);
  await openDocker(page);

  const row = page.locator('[data-testid="docker-row"][data-name="solo"]');
  await expect(row).toHaveAttribute('data-state', 'running');
  await row.hover();
  await row.locator('[data-testid="docker-row-stop"]').click();

  await expect.poll(() => docker.calls('Stop').length).toBe(1);
  expect(docker.calls('Stop')[0].args).toEqual({ id: 'c-solo' });

  await emitWailsEvent(page, 'kira:docker:changed', { kinds: ['container'] });
  await expect(row).toHaveAttribute('data-state', 'exited');
});

test('stats events reach the overview, not the side list; leaving the mode unsubscribes and unwatches', async ({
  relaunch,
}) => {
  const { page, docker } = await setupDocker(relaunch);
  await openDocker(page);

  await expect.poll(() => docker.calls('StatsSubscribe').length).toBeGreaterThan(0);
  await emitWailsEvent(page, 'kira:docker:stats', {
    samples: [
      {
        id: 'c-solo',
        cpuPercent: 12.5,
        memUsage: 1_048_576,
        memLimit: 8_388_608,
        memPercent: 12.5,
        netRx: 0,
        netTx: 0,
        blockRead: 0,
        blockWrite: 0,
        pids: 1,
        at: 1,
      },
    ],
  });
  await expect(page.locator('[data-testid="docker-engine-cpu"]')).toContainText('12.5%');
  for (const id of ['docker-row-cpu', 'docker-row-mem', 'docker-usage-bar']) {
    await expect(
      page.locator(`[data-testid="docker-container-list"] [data-testid="${id}"]`),
    ).toHaveCount(0);
  }

  await modeTab(page, 'studio').click();
  await expect.poll(() => docker.calls('StatsUnsubscribe').length).toBeGreaterThan(0);
  await expect.poll(() => docker.calls('Unwatch').length).toBeGreaterThan(0);
});

test('logs: stream args, appended lines, filter, stderr styling, timestamps reopen', async ({
  relaunch,
}) => {
  const { page, docker } = await setupDocker(relaunch);
  await openDocker(page);
  await openContainer(page, 'solo', 'logs');

  await expect.poll(() => docker.calls('LogsOpen').length).toBe(1);
  const first = docker.calls('LogsOpen')[0].args as {
    streamId: string;
    containerId: string;
    tail: number;
    timestamps: boolean;
    follow: boolean;
  };
  expect(first).toMatchObject({
    containerId: 'c-solo',
    tail: 1000,
    timestamps: false,
    follow: true,
  });

  await emitWailsEvent(page, 'kira:docker:logs', {
    streamId: first.streamId,
    lines: [
      { stream: 'stdout', text: 'hello world' },
      { stream: 'stderr', text: 'boom failure' },
    ],
    ended: false,
  });
  const lines = page.locator('[data-testid="docker-log-line"]');
  await expect(lines).toHaveCount(2);
  await expect(lines.nth(1)).toHaveAttribute('data-stream', 'stderr');
  await expect(lines.nth(1)).toHaveClass(/text-error/);

  await page.locator('[data-testid="docker-logs-find"]').click();
  await page.locator('[data-testid="docker-logs-find-input"]').fill('boom');
  await expect(lines).toHaveCount(2);
  await expect(page.locator('[data-testid="docker-logs-find-count"]')).toContainText('1');
  await expect(lines.nth(1).locator('mark')).toHaveText('boom');

  await page.locator('[data-testid="docker-logs-timestamps"]').click();
  await expect.poll(() => docker.calls('LogsOpen').length).toBe(2);
  expect((docker.calls('LogsOpen')[1].args as { timestamps: boolean }).timestamps).toBe(true);
  expect(docker.calls('LogsClose').length).toBeGreaterThan(0);
});

test('contract: terminal: exec opens for the container, input is written, closing the chip closes it', async ({
  relaunch,
}) => {
  const { page, docker } = await setupDocker(relaunch);
  await openDocker(page);
  await openContainer(page, 'solo', 'terminal');

  await page.locator('[data-testid="docker-exec-new"]').click();
  await expect.poll(() => docker.calls('ExecOpen').length).toBe(1);
  const sent = docker.calls('ExecOpen')[0].args as Record<string, unknown>;
  expect(sent).toMatchObject({ containerId: 'c-solo' });
  expect(Object.keys(sent).sort()).toEqual(Object.keys(execOpenArgs()).sort());
  await expect(page.locator('.xterm-rows')).toBeVisible();

  await page.locator('.xterm-helper-textarea').focus();
  await page.keyboard.type('ls');
  await expect.poll(() => docker.calls('ExecWrite').length).toBeGreaterThan(0);

  await page.locator('[data-testid="docker-exec-close"]').click();
  await expect.poll(() => docker.calls('ExecClose').length).toBe(1);
});

test('contract: terminal: no session opens until New session is clicked; sessions survive tab switches', async ({
  relaunch,
}) => {
  const { page, docker } = await setupDocker(relaunch);
  await openDocker(page);
  await openContainer(page, 'solo', 'terminal');

  await expect(page.locator('[data-testid="docker-exec-empty"]')).toBeVisible();
  await expect(page.locator('[data-testid="docker-exec-chip"]')).toHaveCount(0);
  expect(docker.calls('ExecOpen')).toHaveLength(0);
  await page.locator('[data-testid="docker-tab-logs"]').click();
  await page.locator('[data-testid="docker-tab-terminal"]').click();
  await expect(page.locator('[data-testid="docker-exec-empty"]')).toBeVisible();
  expect(docker.calls('ExecOpen')).toHaveLength(0);

  await page.locator('[data-testid="docker-exec-new"]').focus();
  await page.keyboard.press('Enter');
  await expect.poll(() => docker.calls('ExecOpen').length).toBe(1);
  expect(Object.keys(docker.calls('ExecOpen')[0].args as object).sort()).toEqual(
    Object.keys(execOpenArgs()).sort(),
  );
  await expect(page.locator('[data-testid="docker-exec-chip"]')).toHaveCount(1);
  await expect(page.locator('.xterm-rows')).toBeVisible();

  await page.locator('[data-testid="docker-tab-logs"]').click();
  await page.locator('[data-testid="docker-tab-terminal"]').click();
  await expect(page.locator('[data-testid="docker-exec-chip"]')).toHaveCount(1);
  expect(docker.calls('ExecOpen')).toHaveLength(1);

  await page.locator('[data-testid="docker-exec-close"]').click();
  await expect.poll(() => docker.calls('ExecClose').length).toBe(1);
  await expect(page.locator('[data-testid="docker-exec-empty"]')).toBeVisible();
  expect(docker.calls('ExecOpen')).toHaveLength(1);
});

test('engine dropdown items do not overlap', async ({ relaunch }) => {
  const { page } = await setupDocker(relaunch, {
    handlers: {
      Contexts: () => [
        { name: 'default', host: 'unix:///var/run/docker.sock', description: '', current: true },
        {
          name: 'colima',
          host: 'unix:///Users/me/.colima/default/docker.sock',
          description: '',
          current: false,
        },
        { name: 'remote', host: 'tcp://10.0.0.5:2376', description: '', current: false },
      ],
    },
  });
  await openDocker(page);
  await page.getByTestId('docker-panel').getByTestId('docker-endpoint-chip').click();
  await expect(page.locator('[data-testid="docker-context-option"]')).toHaveCount(3);

  const bad = await page
    .locator('[data-testid="docker-context-menu"] [role="menuitemradio"]')
    .evaluateAll((items) => {
      const problems: string[] = [];
      let prevBottom = Number.NEGATIVE_INFINITY;
      for (const item of items) {
        const r = item.getBoundingClientRect();
        if (r.top < prevBottom - 0.5) problems.push('items overlap');
        prevBottom = r.bottom;
        for (const span of item.querySelectorAll('span')) {
          const s = span.getBoundingClientRect();
          if (s.top < r.top - 0.5 || s.bottom > r.bottom + 0.5) problems.push('text spills');
        }
      }
      return problems;
    });
  expect(bad).toEqual([]);
});

test('images, volumes and networks list; "used by" selects the container', async ({ relaunch }) => {
  const { page } = await setupDocker(relaunch);
  await openDocker(page);

  await page.locator('[data-testid="docker-section-images"]').click();
  await expect(page.locator('[data-testid="docker-row"]')).toHaveCount(2);
  await page.locator('[data-testid="docker-section-networks"]').click();
  await expect(page.locator('[data-testid="docker-row"]')).toHaveCount(1);

  await page.locator('[data-testid="docker-section-volumes"]').click();
  await page.locator('[data-testid="docker-row"]').click();
  await expect(page.locator('[data-testid="docker-resource-detail"]')).toHaveAttribute(
    'data-kind',
    'volume',
  );
  await page.locator('[data-testid="docker-used-by"]').click();
  await expect(page.locator('[data-testid="docker-container-detail"]')).toBeVisible();
  await expect(page.locator('[data-testid="docker-detail-name"]')).toHaveText('shop-db-1');
});

test('the tab strip is hidden in Docker mode and back elsewhere', async ({ relaunch }) => {
  const { page } = await setupDocker(relaunch);
  await expect(page.locator('[data-testid="tab-strip"]')).toBeVisible();

  await openDocker(page);
  await expect(page.locator('[data-testid="tab-strip"]')).toHaveCount(0);

  await modeTab(page, 'studio').click();
  await expect(page.locator('[data-testid="tab-strip"]')).toBeVisible();
});

test('engine overview tabulates containers with humanized stats; section tabs share one row', async ({
  relaunch,
}) => {
  const { page } = await setupDocker(relaunch);
  await openDocker(page);

  const tabs = page.locator(
    '[data-testid="docker-section-containers"], [data-testid="docker-section-images"], [data-testid="docker-section-volumes"], [data-testid="docker-section-networks"]',
  );
  await expect(tabs).toHaveCount(4);
  const tops = await tabs.evaluateAll((els) =>
    els.map((el) => Math.round(el.getBoundingClientRect().top)),
  );
  expect(new Set(tops).size).toBe(1);

  await expect(page.locator('[data-testid="docker-engine-overview"]')).toBeVisible();
  await expect(page.locator('[data-testid="docker-engine-mem"]')).toHaveText('0 B');
  await expect(page.locator('[data-testid="docker-table-row"]')).toHaveCount(4);

  await emitWailsEvent(page, 'kira:docker:stats', {
    samples: [
      {
        id: 'c-db',
        cpuPercent: 3.2,
        memUsage: 1.5 * 1024 ** 3,
        memLimit: 16 * 1024 ** 3,
        memPercent: 9.4,
        netRx: 0,
        netTx: 0,
        blockRead: 0,
        blockWrite: 0,
        pids: 1,
        at: 1,
      },
    ],
  });
  await expect(page.locator('[data-testid="docker-engine-mem"]')).toHaveText('1.5 GB');
  await expect(
    page.locator('[data-testid="docker-table-row"][data-name="shop-db-1"]'),
  ).toContainText('1.5 GB');

  await page.locator('[data-testid="docker-table-row"][data-name="solo"]').click();
  await expect(page.locator('[data-testid="docker-detail-name"]')).toHaveText('solo');
  await page.locator('[data-testid="docker-overview-home"]').click();
  await expect(page.locator('[data-testid="docker-engine-overview"]')).toBeVisible();
});

const registryLink = (key: string) =>
  contract<{ url: string; unconfirmed: boolean }>('docker-registry', `DockerService.${key}`);

test('contract: image detail registry button asks Hub only on click, then opens the URL', async ({
  relaunch,
}) => {
  const candidate = registryLink('ImageRegistry#unconfirmed');
  const hub = registryLink('ConfirmImageRegistry#hub');
  let confirms = 0;
  const { page, control } = await setupDocker(relaunch, {
    handlers: {
      ImageRegistry: (a) =>
        (a as { id: string }).id === IMAGES[0].id ? candidate : { url: '', unconfirmed: false },
      ConfirmImageRegistry: () => {
        confirms += 1;
        return hub;
      },
    },
  });
  await openDocker(page);
  await page.locator('[data-testid="docker-section-images"]').click();

  const alpine = page.locator('[data-testid="docker-row"]', { hasText: IMAGES[0].tags[0] });
  const none = page.locator('[data-testid="docker-row"]', { hasText: IMAGES[1].tags[0] });
  await none.click();
  await expect(page.locator('[data-testid="docker-resource-title"]')).toHaveText(IMAGES[1].tags[0]);
  await expect(page.locator('[data-testid="docker-resource-registry"]')).toHaveCount(0);

  await alpine.click();
  await expect(page.locator('[data-testid="docker-resource-registry"]')).toBeVisible();
  expect(confirms).toBe(0);
  await page.locator('[data-testid="docker-resource-registry"]').click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.linkOpenExternal))
    .toHaveLength(1);
  expect(confirms).toBe(1);
  expect(control.log().find((e) => e.channel === IPC.linkOpenExternal)?.args).toMatchObject({
    url: hub.url,
  });
});

test('contract: registry button shows "No registry page found" when Hub has none', async ({
  relaunch,
}) => {
  const candidate = registryLink('ImageRegistry#unconfirmed');
  const none = registryLink('ConfirmImageRegistry#local');
  const { page, control } = await setupDocker(relaunch, {
    handlers: { ImageRegistry: () => candidate, ConfirmImageRegistry: () => none },
  });
  await openDocker(page);
  await openContainer(page, 'solo');

  await expect(page.locator('[data-testid="docker-registry-missing"]')).toHaveCount(0);
  await page.locator('[data-testid="docker-detail-registry"]').click();
  await expect(page.locator('[data-testid="docker-registry-missing"]')).toHaveText(
    'No registry page found',
  );
  expect(control.log().filter((e) => e.channel === IPC.linkOpenExternal)).toHaveLength(0);
});

test('registry missing label clears after a later successful click', async ({ relaunch }) => {
  const candidate = registryLink('ImageRegistry#unconfirmed');
  const none = registryLink('ConfirmImageRegistry#local');
  const hub = registryLink('ConfirmImageRegistry#hub');
  let confirms = 0;
  const { page, control } = await setupDocker(relaunch, {
    handlers: {
      ImageRegistry: () => candidate,
      ConfirmImageRegistry: () => {
        confirms += 1;
        return confirms === 1 ? none : hub;
      },
    },
  });
  await openDocker(page);
  await openContainer(page, 'solo');

  await page.locator('[data-testid="docker-detail-registry"]').click();
  await expect(page.locator('[data-testid="docker-registry-missing"]')).toBeVisible();
  await page.locator('[data-testid="docker-detail-registry"]').click();
  await expect(page.locator('[data-testid="docker-registry-missing"]')).toHaveCount(0);
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.linkOpenExternal))
    .toHaveLength(1);
});

test('a named-registry image opens without a Hub check', async ({ relaunch }) => {
  let confirms = 0;
  const { page, control } = await setupDocker(relaunch, {
    handlers: {
      ImageRegistry: () => ({ url: 'https://ghcr.io/acme/app', unconfirmed: false }),
      ConfirmImageRegistry: () => {
        confirms += 1;
        return { url: '', unconfirmed: false };
      },
    },
  });
  await openDocker(page);
  await openContainer(page, 'solo');

  await page.locator('[data-testid="docker-detail-registry"]').click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.linkOpenExternal))
    .toHaveLength(1);
  expect(confirms).toBe(0);
});

test('start is green, stop is red, and a selected row has no bar', async ({ relaunch }) => {
  const { page } = await setupDocker(relaunch);
  await openDocker(page);

  const stopped = page.locator('[data-testid="docker-row"][data-name="old-job"]');
  const running = page.locator('[data-testid="docker-row"][data-name="solo"]');
  await stopped.hover();
  await expect(stopped.locator('[data-testid="docker-row-start"]')).toHaveClass(/text-ok/);
  await running.hover();
  await expect(running.locator('[data-testid="docker-row-stop"]')).toHaveClass(/text-error/);

  await running.click();
  await expect(running).toHaveAttribute('aria-selected', 'true');
  expect(await running.evaluate((el) => getComputedStyle(el).boxShadow)).toBe('none');
});

test('row tooltips come from one shared tooltip', async ({ relaunch }) => {
  const { page } = await setupDocker(relaunch, {
    handlers: {
      Containers: () => [
        ...seedContainers(),
        container('c-dev', 'dev-box', 'running', { origin: 'devcontainer', originName: 'x' }),
      ],
    },
  });
  await openDocker(page);

  const list = page.locator('[data-testid="docker-container-list"]');
  await expect(list.locator('[data-slot="tooltip-trigger"]')).toHaveCount(0);

  const { common, docker } = en;
  await assertTooltipShows(
    page,
    list.locator('[data-name="solo"] [data-testid="docker-row-stop"]'),
    common.action.stop.label,
  );
  await assertTooltipShows(
    page,
    list.locator('[data-project="shop"] [data-testid="docker-group-start"]'),
    common.action.startAll.label,
  );
  await assertTooltipShows(
    page,
    list.locator('[data-name="dev-box"] [data-testid="docker-origin-icon"]'),
    docker.origin.devcontainerNamed.tooltip.replace('{name}', 'x'),
  );
});
