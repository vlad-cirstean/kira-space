import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { openMemory, withMemory } from './support/memoryNav';

// P275: ranked search in the Memory module. Backend halves: memoryflow TestSearchRankedWithBoost and
// TestSearchOriginFilter. TestMcpSearchScoreMeta is a backend text format with no UI state.

interface Mem {
  id: string;
  fact: string;
  origin: string;
  score: { total: number };
}

interface SearchArgs {
  query: string;
  includeHistory: boolean;
  scopes: string[];
  origins: string[];
  project: string;
}

const BILLING = 'github.com/acme/billing';
const WEB = 'github.com/acme/web';

const score = (n: number) =>
  new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(
    n,
  );

const rowButtons = (page: Page) => page.locator('button[data-testid^="memory-row-"]');

test('contract: a ranked search sends the filters and boost project and shows scores in order', async ({
  relaunch,
}) => {
  const sent = contract<SearchArgs>('memory', 'args:MemoryService.Search#ranked');
  const hits = contract<Mem[]>('memory', 'MemoryService.Search#ranked');
  const { window: page, control } = await relaunch({
    control: withMemory([
      { channel: IPC.memoryRecent, response: [] },
      { channel: IPC.memoryProjects, response: [BILLING, WEB] },
      { channel: IPC.memorySearch, response: hits },
    ]),
  });
  await openMemory(page);

  await page.locator('[data-testid="memory-search"]').fill(sent.query);
  await page.locator('[data-testid="memory-filter"]').click();
  await page.locator('[data-testid="memory-filter-scope-personal"]').click();
  await page.keyboard.press('Escape');
  await page.locator('[data-testid="memory-boost-project"]').click();
  await page.locator(`[data-testid="memory-boost-project-option-${sent.project}"]`).click();

  await expect
    .poll(
      () =>
        control
          .log()
          .filter((e) => e.channel === IPC.memorySearch)
          .at(-1)?.args,
    )
    .toEqual(sent);

  await expect(rowButtons(page)).toHaveCount(hits.length);
  for (const [i, m] of hits.entries()) {
    const row = rowButtons(page).nth(i);
    await expect(row).toHaveAttribute('data-testid', `memory-row-${m.id}`);
    await expect(row.locator('[data-testid="memory-row-score"]')).toHaveText(score(m.score.total));
  }

  await rowButtons(page).first().locator('[data-testid="memory-row-score"]').hover();
  const parts = page.locator('[data-testid="memory-score-parts"]').first();
  await expect(parts).toContainText('Keyword');
  await expect(parts).toContainText('Current project');
});

test('contract: unchecking agent and external searches user facts only', async ({ relaunch }) => {
  const sent = contract<SearchArgs>('memory', 'args:MemoryService.Search#user');
  const hits = contract<Mem[]>('memory', 'MemoryService.Search#user');
  const { window: page, control } = await relaunch({
    control: withMemory([
      { channel: IPC.memoryRecent, response: [] },
      { channel: IPC.memoryProjects, response: [BILLING, WEB] },
      { channel: IPC.memorySearch, response: hits },
    ]),
  });
  await openMemory(page);

  await page.locator('[data-testid="memory-search"]').fill(sent.query);
  await page.locator('[data-testid="memory-filter"]').click();
  await page.locator('[data-testid="memory-filter-origin-agent"]').click();
  await page.locator('[data-testid="memory-filter-origin-external"]').click();
  await page.keyboard.press('Escape');
  await page.locator('[data-testid="memory-boost-project"]').click();
  await page.locator(`[data-testid="memory-boost-project-option-${sent.project}"]`).click();

  await expect
    .poll(
      () =>
        control
          .log()
          .filter((e) => e.channel === IPC.memorySearch)
          .at(-1)?.args,
    )
    .toEqual(sent);

  await expect(rowButtons(page)).toHaveCount(hits.length);
  for (const m of hits) {
    await expect(page.locator(`[data-testid="memory-row-${m.id}"]`)).toContainText(m.fact);
    await expect(
      page.locator(`[data-testid="memory-row-${m.id}"] [data-testid="memory-row-origin"]`),
    ).toHaveText('User');
  }
});
