import type { Page, TestInfo } from '@playwright/test';
import { closeAllTabs } from '@workbench/testing/ui/heapProbe';
import { type MemSampler, runMemWorld, worldEnabled } from '@workbench/testing/ui/memProbe';
import { emitWailsEvent } from '@workbench/testing/ui/mockRuntime';
import { expect } from '../../ui/fixtures';
import { modeTab } from '../../ui/support/apiMode';
import { connectionRow, expandRow, openRowMenu } from '../../ui/support/tree';

export const APP = 'studio';
export const NOW = new Date('2026-01-15T12:00:00.000Z');
/** Units per tab world (`KIRA_RAM_N` overrides, for smoke runs). */
export const N = Number(process.env.KIRA_RAM_N ?? 10);

export { worldEnabled };

export async function pinClock(page: Page): Promise<void> {
  await page.clock.install({ time: NOW });
  await page.clock.setFixedTime(NOW);
}

export async function connect(page: Page): Promise<void> {
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
}

/** Opens a view from a tree row's context menu. */
export const menuOpen = (page: Page, path: string, item: string, view: string) => async () => {
  await openRowMenu(page, path);
  await page.click(`[data-testid="menu-item-${item}"]`);
  await expect(page.locator(`[data-testid="${view}"]`)).toBeVisible({ timeout: 15_000 });
};

/** Duplicates the first tab through its context menu; the copy becomes active. */
export async function duplicateFirst(page: Page, view: string, kind?: string): Promise<void> {
  const tabs = page.locator('[data-testid="tab"]');
  const before = await tabs.count();
  const source = kind
    ? page.locator(`[data-testid="tab"][data-tab-kind="${kind}"]`).first()
    : tabs.first();
  await source.scrollIntoViewIfNeeded();
  await source.click({ button: 'right' });
  await page.click('[data-testid="menu-item-duplicate-tab"]');
  await expect(tabs).toHaveCount(before + 1);
  await expect(page.locator(`[data-testid="${view}"]:visible`).first()).toBeVisible({
    timeout: 15_000,
  });
}

export interface TabWorldOptions {
  world: string;
  page: Page;
  info: TestInfo;
  units?: number;
  /** Opens the first unit. */
  first: () => Promise<void>;
  /** Test id of the view each duplicate shows; unset means a distinct `next(i)` opens unit i. */
  view?: string;
  /** Tab kind to duplicate when it is not the first tab. */
  kind?: string;
  next?: (i: number) => Promise<void>;
  enter?: () => Promise<void>;
  closeAll?: () => Promise<void>;
  /** Checkpoints that take a pass-B heap snapshot. */
  snapshotAt?: readonly string[];
}

/** A tab-kind world: unit 0 from `first`, the rest duplicates of tab 0 (or `next`). */
export async function tabWorld(o: TabWorldOptions): Promise<MemSampler> {
  const closeAll = o.closeAll ?? (() => closeAllTabs(o.page));
  return runMemWorld({
    app: APP,
    world: o.world,
    page: o.page,
    info: o.info,
    units: o.units ?? N,
    enter: o.enter,
    open: async (i) => {
      if (i === 0) return o.first();
      if (o.next) return o.next(i);
      if (!o.view) throw new Error('tabWorld needs view or next');
      return duplicateFirst(o.page, o.view, o.kind);
    },
    closeAll,
    snapshotAt: o.snapshotAt,
    atCheckpoint: async (name, s) => {
      // TanStack default gcTime is 5 min: what six idle minutes after close-all free.
      if (name !== 'closed') return;
      await o.page.clock.fastForward(6 * 60_000);
      await o.page.waitForTimeout(500);
      await s.sample('closed6m');
    },
  });
}

/** Opens a fresh terminal tab in the automations mode; returns its tab id. */
export async function newTerminal(page: Page): Promise<string> {
  await page.locator('[data-testid="tab-strip-new"]').click();
  await page.locator('[data-testid="menu-item-new-terminal"]').click();
  await expect(page.locator('.xterm-rows').first()).toBeVisible();
  const id = await page
    .locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"][data-active="true"]')
    .getAttribute('data-tab-id');
  if (!id) throw new Error('terminal tab has no id');
  return id;
}

/** Feeds `lines` through the terminal event in 200-line chunks. */
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

export { modeTab };

/** The stream id of the most recent `LogsOpen` call. */
export function lastStreamId(calls: readonly { args?: unknown }[]): string {
  const last = calls[calls.length - 1];
  if (!last) throw new Error('no LogsOpen call');
  return (last.args as { streamId: string }).streamId;
}
