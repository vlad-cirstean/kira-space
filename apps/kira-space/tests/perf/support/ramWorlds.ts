import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { buildPackedChunk, type PackedChunkRow } from '@kira/git-core/testing/packedChunk';
import type { Page, TestInfo } from '@playwright/test';
import { closeAllTabs } from '@workbench/testing/ui/heapProbe';
import { type MemSampler, runMemWorld, worldEnabled } from '@workbench/testing/ui/memProbe';
import { emitWailsEvent } from '@workbench/testing/ui/mockRuntime';
import { expect } from '../../ui/fixtures';
import { connectionRow, expandRow, openRowMenu } from '../../ui/studio/support/tree';
import { adeBoard, adeFixture } from '../../ui/support/adeV2';
import { buildGraphStreamChunk } from '../../ui/support/graphStreamFixture';

export const APP = 'space';
export const NOW = new Date('2026-01-15T12:00:00.000Z');
/** Units per tab world (`KIRA_RAM_N` overrides, for smoke runs). */
export const N = Number(process.env.KIRA_RAM_N ?? 10);

export { worldEnabled };

export function modeTab(page: Page, mode: string) {
  return page.locator(`[data-testid="mode-tab"][data-mode="${mode}"]`);
}

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

/** Unit 0 from `first`, every later unit a duplicate of the first tab. */
export const duplicateOpen =
  (page: Page, first: () => Promise<void>, view: string, kind?: string) =>
  (i: number): Promise<void> =>
    i === 0 ? first() : duplicateFirst(page, view, kind);

export interface TabWorldOptions {
  world: string;
  page: Page;
  info: TestInfo;
  units?: number;
  enter?: () => Promise<void>;
  open: (i: number) => Promise<void>;
  /** Tabs that survive close-all (the pinned graph tab). */
  keep?: number;
  closeAll?: () => Promise<void>;
  /** Click through every tab before close-all (default true); false uses the probe default. */
  activateAll?: boolean;
  /** Sample after six idle minutes past close-all (needs the pinned fake clock). */
  idleSample?: boolean;
  snapshotAt?: readonly string[];
}

/** `open(i)` opens unit i; close-all keeps `keep` pinned tabs. */
export function tabWorld(o: TabWorldOptions): Promise<MemSampler> {
  return runMemWorld({
    app: APP,
    world: o.world,
    page: o.page,
    info: o.info,
    units: o.units ?? N,
    enter: o.enter,
    open: o.open,
    closeAll: o.closeAll ?? (() => closeAllTabs(o.page, o.keep ?? 0)),
    activateAll:
      o.activateAll === false
        ? undefined
        : async () => {
            const tabs = o.page.locator('[data-testid="tab"]');
            const count = await tabs.count();
            for (let i = 0; i < count; i++) {
              await tabs.nth(i).click();
              await o.page.waitForTimeout(500);
            }
          },
    snapshotAt: o.snapshotAt,
    atCheckpoint: o.idleSample
      ? async (name, s) => {
          // TanStack default gcTime is 5 min: what six idle minutes after close-all free.
          if (name !== 'closed') return;
          await o.page.clock.fastForward(6 * 60_000);
          await o.page.waitForTimeout(500);
          await s.sample('closed6m');
        }
      : undefined,
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

/** Opens a fresh terminal tab through the new-tab menu; returns its tab id. */
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

/** The stream id of the most recent `LogsOpen` call. */
export function lastStreamId(calls: readonly { args?: unknown }[]): string {
  const last = calls[calls.length - 1];
  if (!last) throw new Error('no LogsOpen call');
  return (last.args as { streamId: string }).streamId;
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

/** P661 `ade-agents` fixture sizes: archived tasks, stopped session rows, running TUI sessions. */
export const AGENTS_WORLD = { history: 200, stopped: 300, running: 4, bytesPerSession: 2_000_000 };

/** ADE board with `AGENTS_WORLD.history` archived tasks and ten active tasks. */
export function agentsBoard() {
  return adeBoard((b) => {
    b.tasks = b.tasks.slice(0, 10);
    b.history = Array.from({ length: AGENTS_WORLD.history }, (_, i) => ({
      taskId: `H_${i}`,
      title: `Archived task ${i}: ship the billing export for tenant ${i}`,
      codeRepoIds: ['repo-web-app', 'repo-api'],
      mergedAt: 1_789_000_000_000 + i * 3_600_000,
      archivedAt: 1_789_000_000_000 + i * 3_600_000,
    }));
  });
}

/** The first `running` active tasks, each with one running TUI session; plus the stopped rows. */
export function agentsSessions(taskIds: readonly string[]) {
  const base = adeFixture<{ sessions: Record<string, unknown>[] }>('sessions').sessions;
  const stopped = base.find((s) => s.state === 'stopped' && s.mode === 'headless');
  if (!stopped) throw new Error('sessions fixture has no stopped headless row');
  const rows: Record<string, unknown>[] = [];
  for (let i = 0; i < AGENTS_WORLD.running; i++) {
    rows.push({
      ...base.find((s) => s.id === 'tk01'),
      id: `ag${i}`,
      agentSessionId: `claude-ag${i}`,
      terminalId: `term-ag${i}`,
      taskId: taskIds[i],
      branchId: '',
      resumes: '',
    });
  }
  for (let i = 0; i < AGENTS_WORLD.stopped; i++) {
    rows.push({
      ...stopped,
      id: `st${i}`,
      agentSessionId: `claude-st${i}`,
      taskId: taskIds[i % taskIds.length],
      branchId: '',
      runId: `r_st${i}`,
      startedAt: 1_789_000_000_000 + i * 60_000,
      lastActiveAt: 1_789_000_000_000 + i * 60_000 + 30_000,
    });
  }
  return { sessions: rows };
}

/** Claude-like redraws: ED2, home, a 40-line frame of 190-column colored text; until `bytes`. */
export function agentFrames(seed: number, bytes: number): string[] {
  const out: string[] = [];
  let total = 0;
  for (let f = 0; total < bytes; f++) {
    let frame = '\x1b[2J\x1b[H';
    for (let l = 0; l < 40; l++) {
      const text = `agent ${seed} frame ${f} line ${l} `.padEnd(190, `${l % 10}`);
      frame += `\x1b[3${(l % 6) + 1}m${text}\x1b[0m\r\n`;
    }
    out.push(frame);
    total += frame.length;
  }
  return out;
}

export async function feedFrames(
  page: Page,
  channel: string,
  terminalId: string,
  frames: readonly string[],
): Promise<void> {
  const per = 8;
  for (let at = 0; at < frames.length; at += per) {
    await emitWailsEvent(page, channel, {
      terminalId,
      data: Buffer.from(frames.slice(at, at + per).join('')).toString('base64'),
      exited: false,
    });
  }
}

const WEBKIT_PROC = /WPEWebProcess|WPENetworkProcess|WPEGraphicsProcess|WebKitWebProcess/;

/** PSS in MB, by process kind, of the browser under test (`/proc/<pid>/smaps_rollup`). */
export function browserPss(engine: 'chromium' | 'webkit'): Record<string, number> {
  const out: Record<string, number> = {};
  const ps = execFileSync('ps', ['-axo', 'pid=,ppid=,command='], { encoding: 'utf8' });
  const rows = ps
    .split('\n')
    .map((l) => l.trim().match(/^(\d+)\s+(\d+)\s+(.*)$/))
    .filter((m): m is RegExpMatchArray => m !== null)
    .map((m) => ({ pid: m[1], ppid: m[2], cmd: m[3] }));
  const parent = new Map(rows.map((r) => [r.pid, r.ppid]));
  // Only this run's browser: a sibling Playwright on the host owns the other processes.
  const mine = (pid: string): boolean => {
    for (let p: string | undefined = pid; p && p !== '1'; p = parent.get(p)) {
      if (p === String(process.pid)) return true;
    }
    return false;
  };
  for (const { pid, cmd } of rows) {
    if (!mine(pid)) continue;
    let kind = '';
    if (engine === 'webkit') {
      if (!WEBKIT_PROC.test(cmd)) continue;
      kind = /Network/.test(cmd) ? 'network' : /Graphics/.test(cmd) ? 'gpu' : 'webcontent';
    } else {
      if (!cmd.includes('/opt/pw-browsers')) continue;
      const t = cmd.match(/--type=([a-z-]+)/)?.[1];
      kind = t ?? 'browser';
    }
    try {
      const roll = readFileSync(`/proc/${pid}/smaps_rollup`, 'utf8');
      const kb = Number(roll.match(/^Pss:\s+(\d+)/m)?.[1] ?? 0);
      out[kind] = Math.round(((out[kind] ?? 0) + kb / 1024) * 10) / 10;
    } catch {
      // process gone
    }
  }
  return out;
}
