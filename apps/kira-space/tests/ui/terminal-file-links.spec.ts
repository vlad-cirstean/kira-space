import { expect, type Locator, type Page } from '@playwright/test';
import { test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import { repoRowById, repoTab } from './support/repoRows';
import type { ControlSnapshot } from './support/types';

// Same scenario as flows/windowflow TestTerminalFileLinks. The UI harness runs a macOS user agent,
// so the primary modifier is Meta.

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const SCENARIO = 'terminal-file-links';
const resolveArgs = contract<{ paths: string[] }>(
  SCENARIO,
  'args:OpenedFilesService.TerminalResolve',
);
const resolved = contract<{ openable: boolean[] }>(SCENARIO, 'OpenedFilesService.TerminalResolve');
const openArgs = contract<{ path: string; line: number; col: number }>(
  SCENARIO,
  'args:OpenedFilesService.TerminalOpen',
);
const notOpenable = JSON.parse(
  contract<string>(SCENARIO, 'error:OpenedFilesService.TerminalOpen'),
) as { code: string; message: string };
const pendingTerminal = contract<Array<Record<string, unknown>>>(
  SCENARIO,
  'OpenedFilesService.Pending:terminal',
);
const { url } = contract<{ url: string }>('terminal-link', 'args:LinkService.OpenExternal');

const PATHS = resolveArgs.paths;
// A bare word with no slash or extension (`src`) is never a candidate, so the parser never sends it.
const CANDIDATES = PATHS.filter((p) => p !== 'src');
// The first path carries the position the open call is checked against.
const LINE = `${PATHS[0]}:${openArgs.line}:${openArgs.col} ${PATHS.slice(1).join(' ')}`;

const BASE: ControlSnapshot[] = [
  { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
  { channel: IPC.codeWorkspaceListFiles, args: { id: REPO.id }, response: [] },
  { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
  { channel: IPC.linkOpenExternal, response: null },
  { channel: IPC.openedFilesTerminalResolve, response: resolved },
  { channel: IPC.openedFilesTerminalOpen, response: null },
];

async function printLine(page: Page, line: string): Promise<string> {
  await repoRowById(page, REPO.id).click();
  await page.locator('[data-testid="tab-strip-new"]').click();
  await expect(page.locator('.xterm-rows')).toBeVisible();
  const terminalId = await page
    .locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"][data-active="true"]')
    .getAttribute('data-tab-id');
  expect(terminalId).not.toBeNull();
  await emitWailsEvent(page, IPC.terminal, {
    terminalId,
    data: Buffer.from(`${line}\r\n`).toString('base64'),
    exited: false,
  });
  return terminalId ?? '';
}

/** Pointer position, inside `row`, over the middle of the first occurrence of `needle`. */
async function over(row: Locator, needle: string): Promise<{ x: number; y: number }> {
  const point = await row.evaluate((el, text) => {
    const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
    const nodes: Text[] = [];
    for (let n = walker.nextNode(); n; n = walker.nextNode()) nodes.push(n as Text);
    const all = nodes.map((n) => n.data).join('');
    const mid = all.indexOf(text) + Math.floor(text.length / 2);
    let seen = 0;
    for (const node of nodes) {
      if (mid < seen + node.data.length) {
        const range = document.createRange();
        range.setStart(node, mid - seen);
        range.setEnd(node, mid - seen + 1);
        const r = range.getBoundingClientRect();
        const box = el.getBoundingClientRect();
        return { x: r.left - box.left + r.width / 2, y: box.height / 2 };
      }
      seen += node.data.length;
    }
    return null;
  }, needle);
  if (!point) throw new Error(`${needle} not on the row`);
  return point;
}

const calls = (control: { log(): { channel: string; args?: unknown }[] }, channel: string) =>
  control.log().filter((e) => e.channel === channel);
const hint = (page: Page) => page.locator('[data-testid="terminal-link-hint"]');

test('contract: a file path opens in Kira Space only on Cmd/Ctrl-click, with a hover hint', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: BASE });
  const terminalId = await printLine(page, LINE);
  const row = page.locator('.xterm-rows > div', { hasText: PATHS[0] });
  await expect(row).toBeVisible();
  const at = await over(row, `${PATHS[0]}:${openArgs.line}`);

  await row.hover({ position: at, force: true });
  await expect(hint(page)).toBeVisible();
  await expect(hint(page)).toContainText(`${PATHS[0]}:${openArgs.line}:${openArgs.col}`);
  await expect(hint(page)).toContainText('-click to open');
  const asked = calls(control, IPC.openedFilesTerminalResolve);
  expect(asked[0]?.args).toMatchObject({ terminalId, paths: CANDIDATES });

  await row.click({ position: at, force: true });
  expect(calls(control, IPC.openedFilesTerminalOpen)).toHaveLength(0);
  await row.click({ position: at, modifiers: ['Meta'], force: true });
  await expect.poll(() => calls(control, IPC.openedFilesTerminalOpen)).toHaveLength(1);
  expect(calls(control, IPC.openedFilesTerminalOpen)[0]?.args).toMatchObject({
    terminalId,
    path: PATHS[0],
    line: openArgs.line,
    col: openArgs.col,
  });
});

test('contract: a path that is not a file is not a link', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: BASE });
  await printLine(page, LINE);
  const row = page.locator('.xterm-rows > div', { hasText: PATHS[0] });
  await expect(row).toBeVisible();

  for (const name of ['missing.go', ' src ']) {
    const at = await over(row, name);
    await row.hover({ position: at, force: true });
    await expect
      .poll(() => calls(control, IPC.openedFilesTerminalResolve).length)
      .toBeGreaterThan(0);
    await expect(hint(page)).toBeHidden();
    await row.click({ position: at, modifiers: ['Meta'], force: true });
  }
  expect(calls(control, IPC.openedFilesTerminalOpen)).toHaveLength(0);
  expect(calls(control, IPC.linkOpenExternal)).toHaveLength(0);
});

test('contract: a web link on the same line still opens in the browser', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: BASE });
  await printLine(page, `${url} ${PATHS[0]}`);
  const row = page.locator('.xterm-rows > div', { hasText: url });
  await expect(row).toBeVisible();
  const at = await over(row, url);

  await row.hover({ position: at, force: true });
  await expect(hint(page)).toContainText(url);
  await row.click({ position: at, modifiers: ['Meta'], force: true });
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.openedFilesTerminalOpen)).toHaveLength(0);
  for (const e of calls(control, IPC.openedFilesTerminalResolve)) {
    expect(JSON.stringify(e.args)).not.toContain('example.com');
  }
});

test('contract: a file gone by click time announces the error', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      ...BASE.filter((s) => s.channel !== IPC.openedFilesTerminalOpen),
      { channel: IPC.openedFilesTerminalOpen, error: notOpenable },
    ],
  });
  await printLine(page, LINE);
  const row = page.locator('.xterm-rows > div', { hasText: PATHS[0] });
  await expect(row).toBeVisible();
  const at = await over(row, PATHS[0] ?? '');

  await row.hover({ position: at, force: true });
  await expect(hint(page)).toBeVisible();
  await row.click({ position: at, modifiers: ['Meta'], force: true });
  await expect.poll(() => calls(control, IPC.openedFilesTerminalOpen)).toHaveLength(1);
  await expect(page.locator('[data-testid="app-live-region"]')).toContainText(notOpenable.message);
});

test('contract: the opened window places the cursor', async ({ relaunch }) => {
  const target = pendingTerminal[0];
  if (!target) throw new Error('contract has no pending target');
  const { line, col } = target as { line: number; col: number };
  const text = 'package a\nfunc a() {}\nvar x = 1\n';
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
      {
        channel: IPC.codeWorkspaceListFiles,
        args: { id: REPO.id },
        response: { paths: ['src/a.go'], status: {}, truncated: false },
      },
      {
        channel: IPC.codeWorkspaceReadFile,
        args: { id: REPO.id, path: target.relPath },
        response: {
          kind: 'found',
          text,
          bytes: text.length,
          limitBytes: 8 * 1024 * 1024,
          language: 'go',
        },
      },
      { channel: IPC.openedFilesPending, response: [{ ...target, repoId: REPO.id }] },
    ],
  });
  await expect(repoTab(page, 'repo-file')).toHaveAttribute('data-active', 'true');
  const lines = page.locator('[data-testid="repo-file-editor"] .view-lines');
  await expect(lines).toContainText('func a() {}');
  await expect
    .poll(() => cursorAt(page, '[data-testid="repo-file-editor"]'))
    .toEqual({ line, col });
  await expect
    .poll(() =>
      calls(control, IPC.tabsSave).some((e) =>
        JSON.stringify(e.args).includes(`"revealLine":${line}`),
      ),
    )
    .toBe(true);
});

test('contract: a local file opens at its line', async ({ relaunch }) => {
  const outside = { path: '/tmp/work/notes/b.json', repoId: '', relPath: '', line: 2 };
  const text = '{\n  "k": 1\n}\n';
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.openedFilesPending, response: [outside] },
      {
        channel: IPC.openedFilesRead,
        args: { path: outside.path },
        response: {
          kind: 'found',
          text,
          bytes: text.length,
          limitBytes: 8 * 1024 * 1024,
          language: 'json',
          stat: { hash: 'f'.repeat(64), size: text.length },
        },
      },
    ],
  });
  await expect(page.locator('[data-testid="local-file-editor"] .view-lines')).toContainText(
    '"k": 1',
  );
  await expect
    .poll(() => cursorAt(page, '[data-testid="local-file-editor"]'))
    .toMatchObject({ line: outside.line });
});

/** The cursor's 1-based line and column, read from where Monaco draws it. */
async function cursorAt(page: Page, editor: string): Promise<{ line: number; col: number } | null> {
  return page.locator(editor).evaluate((el) => {
    const cursor = el.querySelector('.cursor')?.getBoundingClientRect();
    const rows = [...el.querySelectorAll('.view-lines .view-line')].map((r) => ({
      r: r.getBoundingClientRect(),
      text: r.textContent ?? '',
    }));
    if (!cursor || rows.length === 0) return null;
    rows.sort((a, b) => a.r.top - b.r.top);
    const idx = rows.findIndex((row) => cursor.top + cursor.height / 2 < row.r.bottom);
    const row = rows[idx];
    const widest = rows.reduce((a, b) => (b.text.length > a.text.length ? b : a));
    if (!row || !widest.text.length) return null;
    const span = el.querySelector('.view-line span span, .view-line span');
    const charWidth = (span?.getBoundingClientRect().width ?? 0) / (span?.textContent?.length || 1);
    return {
      line: idx + 1,
      col: Math.round((cursor.left - row.r.left) / (charWidth || 1)) + 1,
    };
  });
}
