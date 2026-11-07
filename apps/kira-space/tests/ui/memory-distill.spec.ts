import type { Page } from '@playwright/test';
import { expect, type KiraApp, type Relaunch, type RelaunchOptions, test } from './fixtures';
import { contract } from './support/contract';
import { installGitStreamMock } from './support/gitStreamMock';
import { IPC } from './support/ipcChannels';
import { openMemory, withMemory } from './support/memoryNav';
import { emitWailsEvent } from './support/mockRuntime';
import { repoRowById, treeRow } from './support/repoRows';
import type { ControlSnapshot } from './support/types';

// P285 Part 4: distill in the Memory module against a mocked bridge. Backend half: memoryflow
// TestBridgeDistill*; fixtures from contract memory-distill. P315: rewrite mode, backend half
// TestBridgeDistillRewrite.

interface Rewritten {
  text: string;
  changed: boolean;
  gate: string;
  reason: string;
  dropped: { text: string; why: string }[];
}
interface Distilled {
  mode: 'facts' | 'rewrite';
  rewrite: Rewritten | null;
  lines: { text: string; kind: 'fact' | 'rule' }[];
  unresolved: { fact: string; questions: string[] }[];
  costUsd: number;
  chunks: number;
  inChars: number;
  outChars: number;
  inTokens: number;
  outTokens: number;
}
interface Limits {
  maxChars: number;
  maxParts: number;
  spendCapUsd: number;
  storeMaxLines: number;
  rewriteMaxChars: number;
}

const done = contract<Distilled>('memory-distill', 'MemoryDistillService.Distill#done');
const rewritten = contract<Distilled>('memory-distill', 'MemoryDistillService.Distill#rewrite');
const unchanged = contract<Distilled>(
  'memory-distill',
  'MemoryDistillService.Distill#rewriteUnchanged',
);
const kept = contract<Distilled>('memory-distill', 'MemoryDistillService.Distill#rewriteKept');
const limits = contract<Limits>('memory-distill', 'MemoryDistillService.Limits');
const progress = contract<{
  requestId: string;
  stage: string;
  done: number;
  total: number;
  costUsd: number;
}>('memory-distill', 'kira:memory:distill#progress');
const stored = contract<{ status: string; outcomes: { action: string }[] }>(
  'memory',
  'MemoryService.Store#stored',
);
const scanned = contract<{ job: Record<string, unknown>; files: unknown[] }>(
  'memory-import',
  'MemoryImportService.Job#awaiting',
);

const BASE: ControlSnapshot[] = withMemory([
  { channel: IPC.memoryDistillLimits, response: limits },
]);
const TEXT = 'The worker keeps port 8000 open and the queue never retries.';

async function openDistill(page: Page): Promise<void> {
  await openMemory(page);
  await page.locator('[data-testid="memory-distill"]').click();
  await expect(page.locator('[data-testid="distill-view"]')).toBeVisible();
}

function runCalls(control: { log(): { channel: string; args: unknown }[] }) {
  return control.log().filter((e) => e.channel === IPC.memoryDistillRun);
}

test('contract: distill: paste, size line, progress push, result summary and Copy', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [...BASE, { channel: IPC.memoryDistillRun, response: done, hold: true }],
  });
  await page.evaluate(() => {
    const w = window as unknown as { __clipboard: string[] };
    w.__clipboard = [];
    navigator.clipboard.writeText = async (text: string) => {
      w.__clipboard.push(text);
    };
  });
  await openDistill(page);

  const run = page.locator('[data-testid="distill-run"]');
  await expect(run).toBeDisabled();
  await page.locator('[data-testid="distill-input"]').fill(TEXT);
  await expect(page.locator('[data-testid="distill-size"]')).toContainText(
    `${TEXT.length} characters, about ${Math.floor((TEXT.length + 2) / 3)} tokens`,
  );
  // Facts is the default mode.
  await expect(page.locator('[data-testid="distill-mode-facts"]')).toBeVisible();
  await expect(page.locator('[data-testid="distill-mode-rewrite"]')).toBeVisible();
  await expect(page.locator('[data-testid="distill-hint"]')).toContainText(
    'rules as short imperatives',
  );
  await run.click();

  await expect.poll(() => runCalls(control)).toHaveLength(1);
  const sent = runCalls(control)[0]?.args as { requestId: string; text: string };
  expect(sent).toMatchObject({ text: TEXT, mode: 'facts' });
  await emitWailsEvent(page, IPC.memoryDistill, {
    ...progress,
    stage: 'extract',
    requestId: sent.requestId,
  });
  await expect(page.locator('[data-testid="distill-progress"]')).toContainText(
    `Reading parts: ${progress.done} of ${progress.total}`,
  );
  await emitWailsEvent(page, IPC.memoryDistill, {
    ...progress,
    stage: 'merge',
    requestId: sent.requestId,
  });
  await expect(page.locator('[data-testid="distill-progress"]')).toContainText('Merging lines');
  // Another request's push changes nothing.
  await emitWailsEvent(page, IPC.memoryDistill, { ...progress, requestId: 'other', done: 9 });
  await expect(page.locator('[data-testid="distill-progress"]')).not.toContainText('9 of');

  control.release(IPC.memoryDistillRun);
  const result = page.locator('[data-testid="distill-result"]');
  await expect(result).toBeVisible();
  await expect(page.locator('[data-testid="distill-summary"]')).toContainText(
    `${done.inChars} to ${done.outChars} characters`,
  );
  await expect(page.locator('[data-testid="distill-summary"]')).toContainText(
    `${done.inTokens} to ${done.outTokens} tokens`,
  );
  for (const [i, line] of done.lines.entries()) {
    await expect(result.locator(`[data-testid="distill-text-${i}"]`)).toHaveText(line.text);
    // Only a rule is badged.
    await expect(result.locator(`[data-testid="distill-kind-${i}"]`)).toHaveCount(
      line.kind === 'rule' ? 1 : 0,
    );
  }
  // What the input leaves open is listed apart from the lines.
  const unclear = result.locator('[data-testid="distill-unresolved"]');
  await expect(unclear).toContainText('Unclear, left out');
  for (const [i, item] of done.unresolved.entries()) {
    const row = unclear.locator(`[data-testid="distill-unresolved-${i}"]`);
    await expect(row).toContainText(item.fact);
    await expect(row).toContainText(item.questions.join(' '));
  }

  await result.locator('[data-testid="distill-line-0"]').click();
  await result.locator('[data-testid="distill-copy"]').click();
  await expect
    .poll(() =>
      page.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard[0]),
    )
    .toBe(
      done.lines
        .slice(1)
        .map((l) => l.text)
        .join('\n'),
    );
});

async function runRewrite(
  relaunch: (options?: RelaunchOptions) => Promise<KiraApp>,
  response: Distilled,
  input: string,
) {
  const { window: page, control } = await relaunch({
    control: [...BASE, { channel: IPC.memoryDistillRun, response }],
  });
  await page.evaluate(() => {
    const w = window as unknown as { __clipboard: string[] };
    w.__clipboard = [];
    navigator.clipboard.writeText = async (text: string) => {
      w.__clipboard.push(text);
    };
  });
  await openDistill(page);
  await page.locator('[data-testid="distill-mode-rewrite"]').click();
  await page.locator('[data-testid="distill-input"]').fill(input);
  await page.locator('[data-testid="distill-run"]').click();
  await expect(page.locator('[data-testid="distill-rewrite-text"]')).toBeVisible();
  return { page, control };
}

const clipboard = (page: Page) =>
  page.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard[0]);

test('contract: distill: Rewrite sends mode rewrite and shows the text with Copy', async ({
  relaunch,
}) => {
  const input = rewritten.rewrite?.text ?? '';
  const { page, control } = await runRewrite(relaunch, rewritten, input);
  expect(runCalls(control)[0]?.args).toMatchObject({ text: input, mode: 'rewrite' });
  await expect(page.locator('[data-testid="distill-rewrite-text"]')).toHaveText(input);
  await expect(page.locator('[data-testid="distill-rewrite-dropped-0"]')).toContainText(
    rewritten.rewrite?.dropped[0]?.text ?? '',
  );
  await expect(page.locator('[data-testid="distill-rewrite-unchanged"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="distill-rewrite-kept"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="distill-store"]')).toHaveCount(0);
  await page.locator('[data-testid="distill-rewrite-copy"]').click();
  await expect.poll(() => clipboard(page)).toBe(input);
});

test('contract: distill: a kept rewrite shows the original and why', async ({ relaunch }) => {
  const { page } = await runRewrite(relaunch, kept, kept.rewrite?.text ?? '');
  await expect(page.locator('[data-testid="distill-rewrite-text"]')).toHaveText(
    kept.rewrite?.text ?? '',
  );
  await expect(page.locator('[data-testid="distill-rewrite-kept"]')).toContainText(
    'Kept the original. The rewrite was longer.',
  );
  await expect(page.locator('[data-testid="distill-rewrite-unchanged"]')).toHaveCount(0);
});

test('contract: distill: an already clean text comes back unchanged', async ({ relaunch }) => {
  const text = unchanged.rewrite?.text ?? '';
  const { page } = await runRewrite(relaunch, unchanged, text);
  await expect(page.locator('[data-testid="distill-rewrite-text"]')).toHaveText(text);
  await expect(page.locator('[data-testid="distill-rewrite-unchanged"]')).toContainText(
    'Already concise',
  );
  await expect(page.locator('[data-testid="distill-rewrite-kept"]')).toHaveCount(0);
  await expect(page.locator('[data-testid^="distill-rewrite-dropped-"]')).toHaveCount(0);
  await page.locator('[data-testid="distill-rewrite-copy"]').click();
  await expect.poll(() => clipboard(page)).toBe(text);
});

test('contract: distill: in Rewrite mode a text over the rewrite limit blocks Run', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: BASE });
  await openDistill(page);
  await page.locator('[data-testid="distill-mode-rewrite"]').click();
  await page.locator('[data-testid="distill-input"]').fill('a'.repeat(limits.rewriteMaxChars + 1));
  await expect(page.locator('[data-testid="distill-size"]')).toContainText(
    `Over the limit of ${limits.rewriteMaxChars.toLocaleString('en-US')} characters`,
  );
  await expect(page.locator('[data-testid="distill-run"]')).toBeDisabled();
  await page.locator('[data-testid="distill-mode-facts"]').click();
  await expect(page.locator('[data-testid="distill-run"]')).toBeEnabled();
});

test('contract: distill: Cancel cancels the call and shows the cancelled status', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [...BASE, { channel: IPC.memoryDistillRun, response: done, hold: true }],
  });
  const cancels: string[] = [];
  page.on('request', (r) => {
    // Wails runtime object 10 is CancelCall.
    if (!r.url().endsWith('/wails/runtime') || r.method() !== 'POST') return;
    const body = JSON.parse(r.postData() ?? '{}') as { object?: number };
    if (body.object === 10) cancels.push(r.postData() ?? '');
  });
  await openDistill(page);
  await page.locator('[data-testid="distill-input"]').fill(TEXT);
  await page.locator('[data-testid="distill-run"]').click();
  await expect.poll(() => runCalls(control)).toHaveLength(1);

  await page.locator('[data-testid="distill-cancel"]').click();
  await expect.poll(() => cancels.length).toBe(1);
  await expect(page.locator('[data-testid="distill-cancelled"]')).toBeVisible();
  await expect(page.locator('[data-testid="distill-error"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="distill-run"]')).toBeEnabled();
  control.release(IPC.memoryDistillRun);
});

test('contract: distill: the limit blocks Run and a backend error shows in the view', async ({
  relaunch,
}) => {
  const message = `The input splits into ${limits.maxParts + 1} parts. The limit is ${limits.maxParts}.`;
  const { window: page } = await relaunch({
    control: [
      ...BASE,
      { channel: IPC.memoryDistillRun, error: { code: 'E_BAD_REQUEST', message } },
    ],
  });
  await openDistill(page);
  const input = page.locator('[data-testid="distill-input"]');
  await input.fill('a'.repeat(limits.maxChars + 1));
  await expect(page.locator('[data-testid="distill-size"]')).toContainText(
    `Over the limit of ${limits.maxChars.toLocaleString('en-US')} characters`,
  );
  await expect(page.locator('[data-testid="distill-run"]')).toBeDisabled();

  await input.fill(TEXT);
  await page.locator('[data-testid="distill-run"]').click();
  await expect(page.locator('[data-testid="distill-error"]')).toHaveText(message);
  await expect(page.locator('[data-testid="distill-result"]')).toHaveCount(0);
});

test('contract: distill: Store sends the picked lines with the distilled reason', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      ...BASE,
      { channel: IPC.memoryDistillRun, response: done },
      { channel: IPC.memoryStore, response: stored },
    ],
  });
  await openDistill(page);
  await page.locator('[data-testid="distill-input"]').fill(TEXT);
  await page.locator('[data-testid="distill-run"]').click();
  await expect(page.locator('[data-testid="distill-result"]')).toBeVisible();

  await page.locator('[data-testid="distill-store"]').click();
  const dialog = page.locator('[data-testid="add-memory-dialog"]');
  const items = dialog.locator('[data-testid="add-memory-lines"] li');
  await expect(items).toHaveCount(done.lines.length);
  await dialog.locator('[data-testid="add-memory-submit"]').click();
  await expect(dialog.locator('[data-testid="add-memory-outcomes"]')).toBeVisible();

  const sent = control.log().find((e) => e.channel === IPC.memoryStore)?.args as {
    items: { fact: string; reason: string }[];
  };
  expect(sent.items.map((i) => i.fact)).toEqual(done.lines.map((l) => l.text));
  expect(sent.items.every((i) => i.reason.startsWith('Distilled by the user from'))).toBe(true);
});

test('contract: distill: Import as document sends the lines and opens the import confirmation', async ({
  relaunch,
}) => {
  const job = { ...scanned.job, id: 'JD1' };
  const { window: page, control } = await relaunch({
    control: [
      ...BASE,
      { channel: IPC.memoryDistillRun, response: done },
      { channel: IPC.memoryDistillToImport, response: job },
      { channel: IPC.memoryImportJob, args: { id: 'JD1' }, response: { job, files: [] } },
      { channel: IPC.memoryImportJobs, response: [job] },
    ],
  });
  await openDistill(page);
  await page.locator('[data-testid="distill-input"]').fill(TEXT);
  await page.locator('[data-testid="distill-run"]').click();
  await expect(page.locator('[data-testid="distill-result"]')).toBeVisible();

  await page.locator('[data-testid="distill-import"]').click();
  await expect(page.locator('[data-testid="import-confirm-dialog"]')).toBeVisible();
  const sent = control.log().find((e) => e.channel === IPC.memoryDistillToImport)?.args;
  expect(sent).toEqual({ lines: done.lines.map((l) => l.text), source: '' });
});

test('contract: distill: a chosen file is sent by path', async ({ relaunch }) => {
  const path = '/docs/notes.md';
  const { window: page, control } = await relaunch({
    control: [
      ...BASE,
      { channel: IPC.memoryDistillChoose, response: { canceled: false, path } },
      { channel: IPC.memoryDistillRun, response: done },
    ],
  });
  await openDistill(page);
  await page.locator('[data-testid="distill-source-file"]').click();
  await expect(page.locator('[data-testid="distill-run"]')).toBeDisabled();
  await page.locator('[data-testid="distill-choose"]').click();
  await expect(page.locator('[data-testid="distill-path"]')).toContainText('notes.md');
  await page.locator('[data-testid="distill-run"]').click();
  await expect(page.locator('[data-testid="distill-result"]')).toBeVisible();
  expect(runCalls(control)[0]?.args).toMatchObject({ path, text: '' });
});

test('distill: Back returns to the memory view and the draft survives a module switch', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: BASE });
  await openDistill(page);
  await page.locator('[data-testid="distill-input"]').fill(TEXT);

  await page.locator('[data-testid="mode-tab"][data-mode="git"]').click();
  await openMemory(page);
  await expect(page.locator('[data-testid="distill-input"]')).toHaveValue(TEXT);

  await page.locator('[data-testid="distill-back"]').click();
  await expect(page.locator('[data-testid="memory-start"]')).toBeVisible();
  await expect(page.locator('[data-testid="distill-view"]')).toHaveCount(0);
});

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};
const FILE = 'export const a = 1;\n';

// Opens a.ts with its first line selected, Memory on or off.
async function selectInFile(relaunch: Relaunch, memory: boolean): Promise<Page> {
  const { window: page } = await relaunch({
    control: [
      ...(memory ? BASE : []),
      { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
      {
        channel: IPC.codeWorkspaceListFiles,
        args: { id: REPO.id },
        response: { paths: ['a.ts'], status: {}, truncated: false },
      },
      {
        channel: IPC.codeWorkspaceReadFile,
        args: { id: REPO.id, path: 'a.ts' },
        response: {
          kind: 'found',
          text: FILE,
          bytes: FILE.length,
          limitBytes: 8 * 1024 * 1024,
          language: 'typescript',
        },
      },
    ],
  });
  await installGitStreamMock(page, REPO.repoId);
  await repoRowById(page, REPO.id).click();
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await treeRow(page, 'a.ts').click();

  const editor = page.locator('[data-testid="repo-file-editor"]');
  await expect(editor).toBeVisible();
  await editor
    .locator('.view-line')
    .first()
    .click({ position: { x: 10, y: 5 } });
  await page.keyboard.press('Home');
  await page.keyboard.press('Shift+End');
  return page;
}

test('distill: Distill selection in a file view opens the distill view with the text and origin', async ({
  relaunch,
}) => {
  const page = await selectInFile(relaunch, true);
  // The action sits in the context menu and the command palette; the palette keeps the selection.
  await page.keyboard.press('F1');
  await page.keyboard.type('Distill selection');
  await expect(page.locator('.quick-input-list .monaco-list-row').first()).toContainText(
    'Distill selection',
  );
  await page.keyboard.press('Enter');

  await expect(page.locator('[data-testid="distill-view"]')).toBeVisible();
  await expect(page.locator('[data-testid="distill-input"]')).toHaveValue(FILE.trimEnd());
  await expect(page.locator('[data-testid="distill-origin"]')).toContainText('a.ts lines 1 to 1');
});

test('distill: Distill selection is absent while Memory is off', async ({ relaunch }) => {
  const page = await selectInFile(relaunch, false);
  await page.keyboard.press('F1');
  await page.keyboard.type('Distill selection');
  await expect(page.locator('.quick-input-widget')).toBeVisible();
  await expect(
    page.locator('.quick-input-list .monaco-list-row', { hasText: 'Distill selection' }),
  ).toHaveCount(0);
});
