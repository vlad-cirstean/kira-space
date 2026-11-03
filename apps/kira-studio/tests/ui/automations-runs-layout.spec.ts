import type { Page } from '@playwright/test';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { modeTab } from './support/apiMode';
import { IPC } from './support/ipcChannels';

// The Runs section takes at most half of the panel, sized against the panel, not the viewport; the
// script list keeps the rest.

const SCRIPT = {
  id: 'script-1',
  kind: 'script',
  params: [],
  smart: null,
  name: 'Build',
  command: 'make build',
  workingDir: '/tmp/demo',
  dirMode: 'fixed',
  useAdeDir: false,
  color: 'blue',
  collection: '',
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

function run(i: number) {
  return {
    id: `r-${i}`,
    scriptId: SCRIPT.id,
    scriptName: SCRIPT.name,
    color: 'blue',
    kind: 'script',
    trigger: 'terminal',
    state: 'running',
    terminalId: `t-${i}`,
    cwd: '/tmp/demo',
    command: 'make build',
    outcome: null,
    createdAt: 1_000 + i,
    startedAt: 1_000 + i,
    finishedAt: null,
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

function control(runCount: number): ControlSnapshot[] {
  return [
    { channel: IPC.customScriptsList, response: { collections: [], scripts: [SCRIPT] } },
    {
      channel: IPC.scriptRunsList,
      response: Array.from({ length: runCount }, (_, i) => run(i)),
    },
  ];
}

async function heights(page: Page): Promise<{ panel: number; runs: number; list: number }> {
  const box = async (id: string) =>
    (await page.locator(`[data-testid="${id}"]`).boundingBox())?.height ?? 0;
  return {
    panel: await box('automations-panel'),
    runs: await box('runs-section'),
    list: await box('automations-body'),
  };
}

test('many runs cap at half the panel and the script list keeps the rest', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: control(60) });
  await page.setViewportSize({ width: 1280, height: 520 });
  await modeTab(page, 'automations').click();
  await expect(page.locator('[data-testid="run-row"]').first()).toBeVisible();

  const h = await heights(page);
  expect(h.runs).toBeGreaterThan(0);
  expect(h.runs).toBeLessThanOrEqual(h.panel / 2 + 1);
  expect(h.list).toBeGreaterThanOrEqual(h.panel / 2 - 60);
  await expect(page.locator('[data-testid="script-list"]')).toBeVisible();
});

test('few runs size the section to its content', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: control(2) });
  await page.setViewportSize({ width: 1280, height: 720 });
  await modeTab(page, 'automations').click();
  await expect(page.locator('[data-testid="run-row"]').first()).toBeVisible();

  const h = await heights(page);
  expect(h.runs).toBeLessThan(h.panel / 3);
});
