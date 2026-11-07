import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { withMemory } from './support/memoryNav';

// P272: provenance in the Memory module. Backend halves: memoryflow TestStoreScopedFromModule,
// TestContextOfStoredFact and TestMcpStoreProvenance. The grade guard (TestAgentCannotSupersedeUser)
// is a backend rule with no UI state.

interface Mem {
  id: string;
  createdAt: string;
  lineageId: string;
  fact: string;
  origin: string;
  sourceTag: string | null;
  scope: string;
  project: string | null;
  sessionId: string | null;
}

interface Ctx {
  memory: Mem;
  groups: { id: string; kind: string; key: string; title: string; facts: Mem[] }[];
}

async function enterMemoryMode(page: Page): Promise<void> {
  await page.locator('[data-testid="mode-tab"][data-mode="memory"]').click();
  await expect(page.locator('[data-testid="memory-panel"]')).toBeVisible();
}

// Go masks createdAt; the detail pane formats it, so give each fact a real time.
const timed = <T extends Mem>(m: T): T => ({ ...m, createdAt: '2026-03-01T10:00:00.000Z' });

const row = (page: Page, id: string) => page.locator(`[data-testid="memory-row-${id}"]`);

test('contract: Add memory in Project scope sends the chosen project key', async ({ relaunch }) => {
  const sent = contract<{ scope: string; project: string }>(
    'memory',
    'args:MemoryService.Store#scoped',
  );
  const recent = contract<Mem[]>('memory', 'MemoryService.Recent#scoped');
  const projects = contract<string[]>('memory', 'MemoryService.Projects#one');
  const stored = contract<{ status: string }>('memory', 'MemoryService.Store#stored');
  const key = projects[0] ?? '';
  const { window: page, control } = await relaunch({
    control: withMemory([
      { channel: IPC.memoryRecent, response: recent },
      { channel: IPC.memoryProjects, response: projects },
      { channel: IPC.memoryStore, response: stored },
    ]),
  });
  await enterMemoryMode(page);

  await page.locator('[data-testid="memory-add"]').click();
  const dialog = page.locator('[data-testid="add-memory-dialog"]');
  await dialog.locator('[data-testid="add-memory-text"]').fill('billing runs postgres');
  await expect(dialog.locator('[data-testid="add-memory-project"]')).toHaveCount(0);
  await dialog.locator('[data-testid="add-memory-scope-project"]').click();
  await expect(dialog.locator('[data-testid="add-memory-submit"]')).toBeDisabled();
  await dialog.locator('[data-testid="add-memory-project"]').click();
  await page.locator(`[data-testid="add-memory-project-option-${key}"]`).click();
  await dialog.locator('[data-testid="add-memory-submit"]').click();
  await dialog.locator('[data-testid="add-memory-done"]').click();

  const stores = control.log().filter((e) => e.channel === IPC.memoryStore);
  expect(stores).toHaveLength(1);
  expect(stores[0]?.args).toMatchObject({ scope: sent.scope, project: sent.project });

  for (const m of recent) {
    await expect(row(page, m.id).locator('[data-testid="memory-row-origin"]')).toHaveText('User');
    await expect(row(page, m.id).locator('[data-testid="memory-row-scope"]')).toHaveText('Project');
    await expect(row(page, m.id).locator('[data-testid="memory-row-project"]')).toContainText(key);
  }
});

test('contract: a stored fact lists the facts stored with it and opens one', async ({
  relaunch,
}) => {
  const raw = contract<Ctx>('memory', 'MemoryService.Context#request');
  const first = raw.groups[0]?.facts[0];
  if (!first) throw new Error('contract has no request group');
  const sibling = timed(first);
  const ctx: Ctx = {
    memory: timed(raw.memory),
    groups: raw.groups.map((g) => ({ ...g, facts: g.facts.map(timed) })),
  };
  const { window: page } = await relaunch({
    control: withMemory([
      { channel: IPC.memoryRecent, response: [ctx.memory, sibling] },
      {
        channel: IPC.memoryHistory,
        args: { id: ctx.memory.id },
        response: { memories: [ctx.memory], events: [] },
      },
      {
        channel: IPC.memoryHistory,
        args: { id: sibling.id },
        response: { memories: [sibling], events: [] },
      },
      { channel: IPC.memoryContext, args: { id: ctx.memory.id, limit: 0 }, response: ctx },
      {
        channel: IPC.memoryContext,
        args: { id: sibling.id, limit: 0 },
        response: { memory: sibling, groups: [] },
      },
    ]),
  });
  await enterMemoryMode(page);

  await row(page, ctx.memory.id).click();
  await expect(page.locator('[data-testid="memory-detail-id"]')).toContainText(
    ctx.memory.lineageId,
  );
  await expect(page.locator('[data-testid="memory-detail-grade"]')).toBeVisible();
  await expect(page.locator('[data-testid="memory-detail-scope"]')).toHaveText('Personal');
  const groupEl = page.locator('[data-testid="memory-context-group"][data-kind="request"]');
  await expect(groupEl).toHaveCount(1);
  await groupEl.locator(`[data-testid="memory-context-fact-${sibling.id}"]`).click();
  await expect(page.locator('[data-testid="memory-detail-fact"]')).toContainText(sibling.fact);
  await expect(page.locator('[data-testid="memory-context"]')).toHaveCount(0);
});

test('contract: an agent-stored external fact shows its source, scope, session and groups', async ({
  relaunch,
}) => {
  const recent = contract<Mem[]>('memory', 'MemoryService.Recent#mcp').map(timed);
  const rawCtx = contract<Ctx>('memory', 'MemoryService.Context#mcp');
  const ctx: Ctx = {
    memory: timed(rawCtx.memory),
    groups: rawCtx.groups.map((g) => ({ ...g, facts: g.facts.map(timed) })),
  };
  const { window: page } = await relaunch({
    control: withMemory([
      { channel: IPC.memoryRecent, response: recent },
      { channel: IPC.memoryHistory, response: { memories: [ctx.memory], events: [] } },
      { channel: IPC.memoryContext, response: ctx },
    ]),
  });
  await enterMemoryMode(page);

  const external = recent.find((m) => m.origin === 'external');
  if (!external) throw new Error('contract has no external fact');
  await expect(row(page, external.id).locator('[data-testid="memory-row-origin"]')).toHaveText(
    `External · ${external.sourceTag}`,
  );
  await expect(row(page, external.id).locator('[data-testid="memory-row-scope"]')).toHaveText(
    'Company',
  );

  await row(page, external.id).click();
  await expect(page.locator('[data-testid="memory-detail-origin"]')).toContainText('External');
  await expect(page.locator('[data-testid="memory-detail-scope"]')).toHaveText('Company');
  await expect(page.locator('[data-testid="memory-detail-session"]')).toContainText(
    external.sessionId ?? '',
  );
  const kinds = page.locator('[data-testid="memory-context-group"]');
  await expect(kinds).toHaveCount(ctx.groups.length);
  for (const g of ctx.groups) {
    await expect(
      page.locator(`[data-testid="memory-context-group"][data-kind="${g.kind}"]`),
    ).toHaveCount(1);
  }
});
