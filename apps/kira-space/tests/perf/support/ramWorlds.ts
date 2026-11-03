import { buildPackedChunk, type PackedChunkRow } from '@kira/git-core/testing/packedChunk';
import type { Page, TestInfo } from '@playwright/test';
import { closeAllTabs } from '@workbench/testing/ui/heapProbe';
import { type MemSampler, runMemWorld, worldEnabled } from '@workbench/testing/ui/memProbe';
import { emitWailsEvent } from '@workbench/testing/ui/mockRuntime';
import { expect } from '../../ui/fixtures';
import { buildGraphStreamChunk } from '../../ui/support/graphStreamFixture';

export const APP = 'space';
export const N = Number(process.env.KIRA_RAM_N ?? 10);

export { worldEnabled };

export function modeTab(page: Page, mode: 'git' | 'automations' | 'ade' | 'memory') {
  return page.locator(`[data-testid="mode-tab"][data-mode="${mode}"]`);
}

export interface TabWorldOptions {
  world: string;
  page: Page;
  info: TestInfo;
  units?: number;
  enter?: () => Promise<void>;
  open: (i: number) => Promise<void>;
  /** Tabs that survive close-all (the pinned graph tab). */
  keep?: number;
  snapshotAt?: readonly string[];
}

/** Distinct tabs per unit; close-all keeps `keep` pinned tabs. */
export function tabWorld(o: TabWorldOptions): Promise<MemSampler> {
  return runMemWorld({
    app: APP,
    world: o.world,
    page: o.page,
    info: o.info,
    units: o.units ?? N,
    enter: o.enter,
    open: o.open,
    closeAll: () => closeAllTabs(o.page, o.keep ?? 0),
    activateAll: async () => {
      const tabs = o.page.locator('[data-testid="tab"]');
      const count = await tabs.count();
      for (let i = 0; i < count; i++) {
        await tabs.nth(i).click();
        await o.page.waitForTimeout(500);
      }
    },
    snapshotAt: o.snapshotAt,
  });
}

/** Linear history with a merge every 7 rows, as `perf:probe` graph-scroll builds it. */
export function graphChunks(repoId: string, n: number) {
  const sha = (i: number): string => (i + 1).toString(16).padStart(40, '0');
  const rows: PackedChunkRow[] = Array.from({ length: n }, (_, i) => {
    const parents: string[] = [];
    if (i + 1 < n) parents.push(sha(i + 1));
    if (i % 7 === 0 && i + 4 < n) parents.push(sha(i + 4));
    return {
      sha: sha(i),
      subject: `commit ${i}: realistic length subject line for row ${i}`,
      parents,
    };
  });
  const CHUNK = 500;
  const chunks = Array.from({ length: Math.ceil(n / CHUNK) }, (_, c) =>
    buildGraphStreamChunk(
      repoId,
      c,
      buildPackedChunk(
        rows.slice(c * CHUNK, (c + 1) * CHUNK),
        c === 0 ? { from: 0 } : { from: c * CHUNK, dictionary: [], dictionaryBase: 2 },
      ),
    ),
  );
  return { chunks, tip: sha(0) };
}

export async function openTerminalTab(page: Page): Promise<string> {
  await page.locator('[data-testid="tab-strip-new"]').click();
  await expect(page.locator('.xterm-rows').first()).toBeVisible();
  const id = await page
    .locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"][data-active="true"]')
    .getAttribute('data-tab-id');
  if (!id) throw new Error('terminal tab has no id');
  return id;
}

export async function feedTerminal(
  page: Page,
  channel: string,
  terminalId: string,
  lines: number,
): Promise<void> {
  const chunk = 200;
  for (let at = 0; at < lines; at += chunk) {
    let text = '';
    for (let i = at; i < Math.min(at + chunk, lines); i++) {
      text += `line ${i} the quick brown fox jumps over the lazy dog ${i * 7919}\r\n`;
    }
    await emitWailsEvent(page, channel, {
      terminalId,
      data: Buffer.from(text).toString('base64'),
      exited: false,
    });
  }
}
