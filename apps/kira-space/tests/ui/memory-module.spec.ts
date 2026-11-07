import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { withMemory } from './support/memoryNav';
import type { ControlSnapshot } from './support/types';

// P201: the Memory module against a mocked bridge — search, history flags, the version trail, the
// Add memory challenge/resubmit flow and the Connect dialog.

function memory(over: Record<string, unknown>) {
  return {
    id: 'm1',
    lineageId: 'm1',
    version: 1,
    fact: 'The billing API listens on port 9090.',
    kind: 'fact',
    reason: 'Manifest changed in March.',
    keywords: ['billing', 'port'],
    origin: 'user',
    sourceTag: null,
    scope: 'personal',
    project: null,
    sessionId: null,
    status: 'current',
    historical: false,
    supersedesId: null,
    supersededById: null,
    createdAt: '2026-03-01T10:00:00.000Z',
    supersededAt: null,
    versions: 1,
    ...over,
  };
}

const CURRENT = memory({
  id: 'm2',
  lineageId: 'm1',
  version: 2,
  versions: 2,
  supersedesId: 'm1',
  origin: 'agent',
});
const OLD = memory({
  id: 'm1',
  fact: 'The billing API listens on port 8080.',
  status: 'superseded',
  historical: true,
  supersededById: 'm2',
  supersededAt: '2026-03-02T10:00:00.000Z',
});

const HISTORY = {
  memories: [OLD, CURRENT],
  events: [
    {
      seq: 1,
      requestId: 'r1',
      source: 'ui',
      sourceRef: null,
      action: 'add',
      lineageId: 'm1',
      memoryId: 'm1',
      previousId: null,
      origin: 'user',
      sourceTag: null,
      sessionId: null,
      rationale: 'No related memory.',
      createdAt: OLD.createdAt,
    },
    {
      seq: 2,
      requestId: 'r2',
      source: 'mcp',
      sourceRef: null,
      action: 'update',
      lineageId: 'm1',
      memoryId: 'm2',
      previousId: 'm1',
      origin: 'agent',
      sourceTag: null,
      sessionId: null,
      rationale: 'Port corrected.',
      createdAt: CURRENT.createdAt,
    },
  ],
};

const BASE: ControlSnapshot[] = [
  { channel: IPC.memoryRecent, response: [CURRENT] },
  {
    channel: IPC.memorySearch,
    args: { query: 'billing', includeHistory: false, scopes: [], origins: [], project: '' },
    response: [CURRENT],
  },
  {
    channel: IPC.memorySearch,
    args: { query: 'billing', includeHistory: true, scopes: [], origins: [], project: '' },
    response: [CURRENT, OLD],
  },
  { channel: IPC.memoryHistory, response: HISTORY },
  { channel: IPC.memoryContext, response: { memory: CURRENT, groups: [] } },
];

function modeTab(page: Page, mode: 'git' | 'automations' | 'ade' | 'memory') {
  return page.locator(`[data-testid="mode-tab"][data-mode="${mode}"]`);
}

async function openMemory(page: Page): Promise<void> {
  await modeTab(page, 'memory').click();
  await expect(modeTab(page, 'memory')).toHaveClass(/is-active/);
  await expect(page.locator('[data-testid="memory-panel"]')).toBeVisible();
  await expect(page.locator('[data-testid="tab-strip"]')).toHaveCount(0);
}

test('Memory mode lists recent memories, searches, and flags history', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: withMemory(BASE) });
  await openMemory(page);

  const rows = page.locator('button[data-testid^="memory-row-"]');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('port 9090');
  await expect(rows.first()).toContainText('Agent');
  await expect(page.locator('[data-testid="memory-detail-empty"]')).toBeVisible();

  await page.locator('[data-testid="memory-search"]').fill('billing');
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.memorySearch).length)
    .toBeGreaterThan(0);

  await page.locator('[data-testid="memory-include-history"]').click();
  await expect(rows).toHaveCount(2);
  await expect(page.locator('[data-testid="memory-row-m1"]')).toContainText('Historical');
  await expect(page.locator('[data-testid="memory-row-m2"]')).not.toContainText('Historical');
});

test('a rule carries a Rule badge in the list and detail; a fact carries none', async ({
  relaunch,
}) => {
  const rule = memory({
    id: 'r1',
    lineageId: 'r1',
    fact: 'Never write amounts as floats.',
    kind: 'rule',
    reason: 'The ledger rejects fractions.',
  });
  const { window: page } = await relaunch({
    control: withMemory([
      { channel: IPC.memoryRecent, response: [CURRENT, rule] },
      { channel: IPC.memoryHistory, response: { memories: [rule], events: [] } },
      { channel: IPC.memoryContext, response: { memory: rule, groups: [] } },
    ]),
  });
  await openMemory(page);

  await expect(page.locator('[data-testid="memory-row-kind"]')).toHaveCount(1);
  await expect(
    page.locator('[data-testid="memory-row-r1"] [data-testid="memory-row-kind"]'),
  ).toHaveText('Rule');
  await expect(
    page.locator('[data-testid="memory-row-m2"] [data-testid="memory-row-kind"]'),
  ).toHaveCount(0);
  await page.locator('[data-testid="memory-row-r1"]').click();
  await expect(page.locator('[data-testid="memory-detail-kind"]')).toHaveText('Rule');
});

test('selecting a memory shows its reason and version trail', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: withMemory(BASE) });
  await openMemory(page);

  await page.locator('[data-testid="memory-row-m2"]').click();
  const detail = page.locator('[data-testid="memory-detail"]');
  await expect(detail).toContainText('Manifest changed in March.');
  const trail = page.locator('[data-testid="memory-trail"]');
  await expect(trail.locator('[data-testid="memory-version-1"]')).toContainText('Historical');
  await expect(trail.locator('[data-testid="memory-version-2"]')).toContainText('Port corrected.');
});

test('contract: Add memory: free text, a challenge shows questions, resubmit sends clarifications, stored lists outcomes', async ({
  relaunch,
}) => {
  const sent = contract<{ items: { fact: string; reason: string }[] }>(
    'memory',
    'args:MemoryService.Store#free-text',
  );
  const challenged = contract<{
    status: string;
    challenges: { index: number; fact: string; questions: string[] }[];
  }>('memory', 'MemoryService.Store#challenged');
  const stored = contract<{ status: string; outcomes: { action: string }[] }>(
    'memory',
    'MemoryService.Store#stored',
  );
  const item = {
    fact: 'it uses port 8080 and deploys on Fridays',
    reason: 'Stated by the user, added manually in the Memory module.',
  };
  expect(Object.keys(sent.items[0] ?? {}).sort()).toEqual(Object.keys(item).sort());
  const question = challenged.challenges[0]?.questions[0] ?? '';
  const answer = { question, answer: 'billing-api' };
  const { window: page, control } = await relaunch({
    control: withMemory([
      ...BASE,
      {
        channel: IPC.memoryStore,
        args: { items: [item], clarifications: [], scope: 'personal', project: '' },
        response: {
          ...challenged,
          challenges: [{ index: 0, fact: item.fact, questions: [question] }],
        },
      },
      {
        channel: IPC.memoryStore,
        args: { items: [item], clarifications: [answer], scope: 'personal', project: '' },
        response: stored,
      },
    ]),
  });
  await openMemory(page);

  await page.locator('[data-testid="memory-add"]').click();
  const dialog = page.locator('[data-testid="add-memory-dialog"]');
  await dialog.locator('[data-testid="add-memory-text"]').fill(item.fact);
  await dialog.locator('[data-testid="add-memory-submit"]').click();

  await expect(dialog.locator('[data-testid="add-memory-challenge"]')).toContainText(question);
  await dialog.locator('[data-testid="add-memory-answer"]').fill(answer.answer);
  await dialog.locator('[data-testid="add-memory-submit"]').click();

  for (const [i, o] of stored.outcomes.entries()) {
    await expect(dialog.locator(`[data-testid="add-memory-outcome-${i}"]`)).toHaveText(
      o.action === 'add' ? 'Added' : 'Already known',
    );
  }
  const stores = control.log().filter((e) => e.channel === IPC.memoryStore);
  expect(stores).toHaveLength(2);
  expect(stores[1]?.args).toMatchObject({ clarifications: [answer] });

  await dialog.locator('[data-testid="add-memory-done"]').click();
  await expect(dialog).toHaveCount(0);
});

test('module hosts no setup; hints open Settings > Memory', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: withMemory([
      { channel: IPC.memoryRecent, response: [] },
      {
        channel: IPC.memorySemanticStatus,
        response: { state: 'notInstalled', message: '', model: '', done: 0, total: 0 },
      },
    ]),
  });
  await openMemory(page);

  await expect(page.locator('[data-testid="memory-connect"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="memory-setup-hint-semantic"]')).toBeVisible();
  await expect(page.locator('[data-testid="memory-setup-hint-agent"]')).toBeVisible();
  await page.locator('[data-testid="memory-open-settings"]').first().click();
  await expect(page.locator('[data-testid="memory-semantic"]')).toBeVisible();
});

// Contract memory + restart. Backend halves: memoryflow TestStoreThroughGate (stored, found by
// search) and journeyflow TestRestartKeepsUserData (same list after a relaunch).
for (const [scenario, key] of [
  ['memory', 'MemoryService.Recent#gated'],
  ['restart', 'MemoryService.Recent#after-restart'],
] as const) {
  test(`contract: ${scenario} Recent list shows every stored fact`, async ({ relaunch }) => {
    const recent = contract<{ id: string; fact: string }[]>(scenario, key);
    const { window: page } = await relaunch({
      control: withMemory([{ channel: IPC.memoryRecent, response: recent }]),
    });
    await openMemory(page);
    await expect(page.locator('button[data-testid^="memory-row-"]')).toHaveCount(recent.length);
    for (const m of recent) {
      await expect(page.locator(`[data-testid="memory-row-${m.id}"]`)).toContainText(m.fact);
    }
  });
}

test('contract: a search hit lists the fact found by its gate keyword', async ({ relaunch }) => {
  const hits = contract<{ id: string; fact: string }[]>(
    'memory',
    'MemoryService.Search#postgresql',
  );
  const { window: page } = await relaunch({
    control: withMemory([
      { channel: IPC.memoryRecent, response: [] },
      { channel: IPC.memorySearch, response: hits },
    ]),
  });
  await openMemory(page);
  await page.locator('[data-testid="memory-search"]').fill('postgresql');
  await expect(page.locator('button[data-testid^="memory-row-"]')).toHaveCount(hits.length);
  await expect(page.locator(`[data-testid="memory-row-${hits[0]?.id}"]`)).toContainText(
    hits[0]?.fact ?? '',
  );
});
