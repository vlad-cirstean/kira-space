import type { ContractJson } from '@workbench/testing/ui/contract';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// P415: the Operations dock — Kira Space's op log (git spawns, agent sessions, MCP calls) — over
// the shared OpLogPanel. Records are seeded through `opsRecent`, patched through `kira:op:update`
// and background reads arrive on `kira:op:batch`. Backend halves: appflow TestOpsEveryGitSpawn
// (space-ops), adeflow TestOpsAde (space-ops-ade) and adeflow TestAgentCancelsTaskOperation
// (space-ops-agent-cancel).

const NOW = '2026-01-01T00:00:00.000Z';

function opRecord(overrides: Record<string, unknown>): Record<string, unknown> {
  return {
    id: 'op-1',
    startedAt: NOW,
    durationMs: 12,
    kind: 'branchCreate',
    status: 'ok',
    command: 'git branch feat main',
    error: null,
    category: 'git',
    module: 'git',
    trigger: 'user',
    subject: '',
    repoRoot: '/work/alpha',
    repoName: 'alpha',
    source: 'Kira Space',
    taskId: '',
    parentId: '',
    background: false,
    repeat: 1,
    steps: [],
    stepsDropped: 0,
    cancellable: false,
    ...overrides,
  };
}

test('the dock spans the full shell width, flush under project and main', async ({ relaunch }) => {
  const { window } = await relaunch();
  await window.click('[data-testid="toggle-operations-panel"]');
  const dock = window.locator('[data-testid="operations-panel"]');
  await expect(dock).toBeVisible();

  const project = window.locator('[data-testid="project-panel"]');
  const main = window.locator('[data-testid="main-panel"]');
  const [projectBox, mainBox, dockBox, marginBottom] = await Promise.all([
    project.boundingBox(),
    main.boundingBox(),
    dock.boundingBox(),
    project.evaluate((el) => getComputedStyle(el).marginBottom),
  ]);
  if (!projectBox || !mainBox || !dockBox) throw new Error('bounding boxes not found');

  // ±0.5px: Playwright geometry can land on a sub-pixel boundary even at integer CSS values.
  expect(Math.abs(dockBox.x - projectBox.x)).toBeLessThanOrEqual(0.5);
  expect(Math.abs(dockBox.x + dockBox.width - (mainBox.x + mainBox.width))).toBeLessThanOrEqual(
    0.5,
  );
  expect(
    Math.abs(projectBox.y + projectBox.height - (mainBox.y + mainBox.height)),
  ).toBeLessThanOrEqual(0.5);
  expect(Math.abs(dockBox.y - (projectBox.y + projectBox.height) - 6)).toBeLessThanOrEqual(0.5);
  expect(marginBottom).toBe('0px');
});

test('seeded records render repo, source, kind and command', async ({ relaunch }) => {
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.opsRecent,
      response: [
        opRecord({ id: 'op-a' }),
        opRecord({
          id: 'op-b',
          kind: 'fetch',
          repoName: 'beta',
          repoRoot: '/work/beta',
          source: 'ADE board',
          command: 'git fetch origin',
        }),
      ],
    },
  ];
  const { window } = await relaunch({ control: CONTROL });
  await window.click('[data-testid="toggle-operations-panel"]');

  const rows = window.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(2);
  await expect(rows.filter({ hasText: 'alpha' })).toContainText('Kira Space');
  await expect(rows.filter({ hasText: 'alpha' })).toContainText('branchCreate');
  await expect(rows.filter({ hasText: 'alpha' })).toContainText('git branch feat main');
  await expect(rows.filter({ hasText: 'beta' })).toContainText('ADE board');
  await expect(rows.filter({ hasText: 'beta' })).toContainText('git fetch origin');
});

test('the filter matches the repository name', async ({ relaunch }) => {
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.opsRecent,
      response: [
        opRecord({ id: 'op-a' }),
        opRecord({ id: 'op-b', repoName: 'beta', repoRoot: '/work/beta', command: 'git status' }),
      ],
    },
  ];
  const { window } = await relaunch({ control: CONTROL });
  await window.click('[data-testid="toggle-operations-panel"]');
  const rows = window.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(2);

  await window.locator('[data-testid="ops-filter"]').fill('beta');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('beta');
});

test('a live update patches the row from running to ok', async ({ relaunch }) => {
  const { window } = await relaunch();
  await window.click('[data-testid="toggle-operations-panel"]');
  const rows = window.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(0);

  await emitWailsEvent(
    window,
    IPC.opUpdate,
    opRecord({ id: 'op-live', status: 'running', durationMs: null }),
  );
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toHaveAttribute('data-status', 'running');

  await emitWailsEvent(window, IPC.opUpdate, opRecord({ id: 'op-live', durationMs: 40 }));
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toHaveAttribute('data-status', 'ok');
});

test('cancel shows only on a running cancellable row and sends its id', async ({ relaunch }) => {
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.opsRecent,
      response: [
        opRecord({
          id: 'op-fetch',
          kind: 'fetch',
          status: 'running',
          durationMs: null,
          cancellable: true,
          command: 'git fetch origin',
        }),
        opRecord({
          id: 'op-push',
          kind: 'push',
          status: 'running',
          durationMs: null,
          cancellable: false,
          command: 'git push origin main',
        }),
      ],
    },
    { channel: IPC.opsCancel, response: null },
  ];
  const { window, control } = await relaunch({ control: CONTROL });
  await window.click('[data-testid="toggle-operations-panel"]');

  const rows = window.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(2);
  const cancel = window.getByRole('button', { name: 'Cancel operation' });
  await expect(cancel).toHaveCount(1);
  await expect(
    rows.filter({ hasText: 'push' }).getByRole('button', { name: 'Cancel operation' }),
  ).toHaveCount(0);

  await cancel.click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.opsCancel))
    .toEqual([{ channel: IPC.opsCancel, args: { opId: 'op-fetch' } }]);
});

async function openDock(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  records: Record<string, unknown>[],
) {
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.opsRecent, response: records }];
  const launched = await relaunch({ control: CONTROL });
  await launched.window.click('[data-testid="toggle-operations-panel"]');
  return launched;
}

test('module, source and subject columns show where an op came from', async ({ relaunch }) => {
  const mcp = {
    ...contract<ContractJson>('space-ops-ade', 'record#mcp'),
    id: 'op-mcp',
    startedAt: NOW,
  };
  const run = {
    ...contract<ContractJson>('space-ops-ade', 'record#run'),
    id: 'op-run',
    startedAt: NOW,
    durationMs: 5,
  };
  const { window } = await openDock(relaunch, [mcp, run, opRecord({ id: 'op-git' })]);
  const rows = window.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(3);
  await expect(rows.filter({ hasText: 'request_branches' })).toContainText('MCP request_branches');
  await expect(rows.filter({ hasText: 'request_branches' })).toContainText('Fix login');
  await expect(rows.filter({ hasText: 'run' }).first()).toContainText('Claude');
  await expect(rows.filter({ hasText: 'branchCreate' })).toContainText('alpha');
});

test('background reads stay hidden until the toggle, and do not count as running', async ({
  relaunch,
}) => {
  const bare = {
    ...contract<ContractJson[]>('space-ops', 'event:kira:op:batch#background')[0],
    id: 'op-bg',
    startedAt: NOW,
    status: 'running',
    durationMs: null,
  };
  const { window } = await openDock(relaunch, [opRecord({ id: 'op-fg' })]);
  const rows = window.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(1);

  const runningTab = window.locator('[data-testid="ops-tab-running"]');
  await emitWailsEvent(window, IPC.opBatch, [bare]);
  await expect(rows).toHaveCount(1);
  await expect(runningTab).toHaveText('Running');
  await expect(window.locator('[data-testid="ops-background"]')).toContainText('Background 1');

  await window.locator('[data-testid="ops-background"]').click();
  await expect(rows).toHaveCount(2);
  await expect(runningTab).toHaveText('Running');
});

test('an expanded op lists its steps, with the stderr of a failed one', async ({ relaunch }) => {
  const write = contract<ContractJson>('space-ops', 'record#write');
  const failing = {
    ...write,
    id: 'op-steps',
    startedAt: NOW,
    status: 'error',
    error: 'git failed',
    steps: [
      ...write.steps.map((s: Record<string, unknown>) => ({ ...s, durationMs: 3 })),
      {
        argv: 'git push origin main',
        readOnly: false,
        exitCode: 1,
        durationMs: 9,
        status: 'error',
        stderr: 'remote: denied',
      },
    ],
  };
  const { window } = await openDock(relaunch, [failing]);
  await window.locator('[data-testid="op-row"]').first().click();
  const lines = window.locator('[data-testid="op-detail-line"]');
  await expect(lines.first()).toContainText('git ');
  await expect(lines.filter({ hasText: 'Exit 1' })).toBeVisible();
  await expect(window.locator('[data-testid="op-detail-stderr"]')).toHaveText('remote: denied');
});

test('an expanded failed op keeps the newlines of its error and stderr, and several rows stay open', async ({
  relaunch,
}) => {
  const failing = opRecord({
    id: 'op-multi',
    kind: 'push',
    status: 'error',
    command: 'git push origin main',
    error: 'fatal: nope\nsecond line',
    steps: [
      {
        argv: 'git push origin main',
        readOnly: false,
        exitCode: 1,
        durationMs: 9,
        status: 'error',
        stderr: 'remote: denied\nhint: check access',
      },
    ],
  });
  const { window } = await openDock(relaunch, [failing, opRecord({ id: 'op-other' })]);
  await window.locator('[data-testid="op-row"][data-op-id="op-multi"]').click();
  await window.locator('[data-op-id="op-other"]').click();
  await expect(window.locator('[data-testid="op-detail"]')).toHaveCount(2);
  await expect(window.locator('[data-testid="op-detail-error"]')).toHaveText(
    'fatal: nope\nsecond line',
    { useInnerText: true },
  );
  await expect(window.locator('[data-testid="op-detail-stderr"]')).toHaveText(
    'remote: denied\nhint: check access',
    { useInnerText: true },
  );
  await window.locator('[data-testid="op-row"][data-op-id="op-multi"]').click();
  await expect(window.locator('[data-testid="op-detail"]')).toHaveCount(1);
});

test('Copy details and Copy error write the clipboard', async ({ relaunch }) => {
  const rec = opRecord({
    id: 'op-copy',
    status: 'error',
    error: 'fatal: nope',
    steps: [
      {
        argv: 'git push',
        readOnly: false,
        exitCode: 1,
        durationMs: 4,
        status: 'error',
        stderr: 'fatal: nope',
      },
    ],
  });
  const { window } = await openDock(relaunch, [rec]);
  await window.evaluate(() => {
    const w = window as unknown as { __clipboard: string[] };
    w.__clipboard = [];
    navigator.clipboard.writeText = async (text: string) => {
      w.__clipboard.push(text);
    };
  });
  const clip = () =>
    window.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard);

  await window.locator('[data-testid="op-row"]').first().click({ button: 'right' });
  await window.getByRole('menuitem', { name: 'Copy details' }).click();
  await expect.poll(clip).toHaveLength(1);
  expect((await clip())[0]).toContain('git branch feat main');
  expect((await clip())[0]).toContain('git push');
  expect((await clip())[0]).toContain('fatal: nope');

  await window.locator('[data-testid="op-row"]').first().click({ button: 'right' });
  await window.getByRole('menuitem', { name: 'Copy error' }).click();
  await expect.poll(clip).toHaveLength(2);
  expect((await clip())[1]).toBe('fatal: nope');
});

test('a repeated background failure shows its count', async ({ relaunch }) => {
  const { window } = await openDock(relaunch, [
    opRecord({ id: 'op-rep', status: 'error', error: 'fatal: x', background: true, repeat: 2 }),
  ]);
  await window.locator('[data-testid="ops-background"]').click();
  await expect(window.locator('[data-testid="op-status-note"]')).toHaveText('×2');
});

test('a running session op shows Cancel and sends its id', async ({ relaunch }) => {
  const running = {
    ...contract<ContractJson>('space-ops-ade', 'record#running'),
    id: 'op-session',
    startedAt: NOW,
  };
  const { window, control } = await openDock(relaunch, [running]);
  await window.getByRole('button', { name: 'Cancel operation' }).click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.opsCancel))
    .toEqual([{ channel: IPC.opsCancel, args: { opId: 'op-session' } }]);
});

test('contract: a git op an agent cancelled renders cancelled with no Cancel button', async ({
  relaunch,
}) => {
  const cancelled = {
    ...contract<ContractJson>('space-ops-agent-cancel', 'record#cancelled'),
    id: 'op-agent-cancelled',
    startedAt: NOW,
  };
  const { window } = await openDock(relaunch, [cancelled]);
  const row = window.locator('[data-testid="op-row"]');
  await expect(row).toHaveCount(1);
  await expect(row).toHaveAttribute('data-status', 'cancelled');
  await expect(row).toContainText('fetch');
  await expect(window.getByRole('button', { name: 'Cancel operation' })).toHaveCount(0);
});

test('contract: a child op renders under its MCP parent, indented', async ({ relaunch }) => {
  const parent = {
    ...contract<ContractJson>('space-ops-ade', 'record#mcp'),
    id: 'op-parent',
    startedAt: '2026-01-01T00:00:00.000Z',
  };
  const child = {
    ...contract<ContractJson>('space-ops-ade', 'record#child'),
    id: 'op-child',
    parentId: 'op-parent',
    startedAt: '2026-01-01T00:00:01.000Z',
  };
  const other = opRecord({ id: 'op-other', startedAt: '2025-12-31T00:00:00.000Z' });
  const { window } = await openDock(relaunch, [child, parent, other]);
  const rows = window.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(3);
  const ids = () => rows.evaluateAll((els) => els.map((el) => el.getAttribute('data-op-id')));
  expect(await ids()).toEqual(['op-parent', 'op-child', 'op-other']);
  const childRow = window.locator('[data-op-id="op-child"]');
  await expect(childRow).toHaveAttribute('data-depth', '1');
  await expect(childRow).not.toContainText('Via');

  await window.locator('[data-testid="ops-filter"]').fill('worktreeAdd');
  await expect(rows).toHaveCount(1);
  await expect(childRow).toHaveAttribute('data-depth', '0');
  await expect(childRow).toContainText('Via request_branches');
});

test('status tabs, the module facet and Clear filters narrow and restore the list', async ({
  relaunch,
}) => {
  const { window } = await openDock(relaunch, [
    opRecord({ id: 'op-git' }),
    opRecord({ id: 'op-ade', module: 'ade', kind: 'worktreeAdd', source: 'Kira Space ade board' }),
    opRecord({ id: 'op-bad', status: 'error', error: 'fatal: x', kind: 'push' }),
    opRecord({ id: 'op-run', status: 'running', durationMs: null, kind: 'fetch' }),
  ]);
  const rows = window.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(4);
  await expect(window.locator('[data-testid="ops-tab-running"]')).toContainText('1');

  await window.locator('[data-testid="ops-tab-error"]').click();
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toHaveAttribute('data-op-id', 'op-bad');
  await window.locator('[data-testid="ops-tab-all"]').click();
  await expect(rows).toHaveCount(4);

  await window.locator('[data-testid="ops-facet"]').click();
  await window.locator('[data-testid="ops-facet-ade"]').click();
  await window.keyboard.press('Escape');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toHaveAttribute('data-op-id', 'op-ade');
  await expect(window.locator('[data-testid="ops-facet-count"]')).toHaveText('1');

  await window.locator('[data-testid="ops-filter"]').fill('no such command');
  await expect(window.locator('[data-testid="ops-no-match"]')).toBeVisible();
  await window.locator('[data-testid="ops-clear-filters"]').click();
  await expect(rows).toHaveCount(4);
  await expect(window.locator('[data-testid="ops-filter"]')).toHaveValue('');
});

test('hovering a row shows Copy command, which writes the clipboard', async ({ relaunch }) => {
  const { window } = await openDock(relaunch, [opRecord({ id: 'op-copy' })]);
  await window.evaluate(() => {
    const w = window as unknown as { __clipboard: string[] };
    w.__clipboard = [];
    navigator.clipboard.writeText = async (text: string) => {
      w.__clipboard.push(text);
    };
  });
  const row = window.locator('[data-testid="op-row"]').first();
  await row.hover();
  await row.locator('[data-testid="op-copy"]').click();
  await expect
    .poll(() => window.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard))
    .toEqual(['git branch feat main']);
});

test('a narrow window drops low-priority columns and keeps the command', async ({ relaunch }) => {
  const { window } = await openDock(relaunch, [opRecord({ id: 'op-narrow' })]);
  await window.setViewportSize({ width: 820, height: 700 });
  const dock = window.locator('[data-testid="operations-panel"]');
  await expect(dock.getByText('Command', { exact: true })).toBeVisible();
  await expect(dock.getByText('Kind', { exact: true })).toHaveCount(0);
  await expect(window.locator('[data-testid="op-row"]')).toContainText('git branch feat main');
});
