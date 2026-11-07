import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { openMemory, withMemory } from './support/memoryNav';
import type { ControlSnapshot } from './support/types';

// P211: bulk memory import against a mocked bridge — pick, scan confirmation with the estimate,
// Start, then the imports view for a paused job with a done, a failed and a skipped file.

function job(over: Record<string, unknown>) {
  return {
    id: 'J1',
    roots: ['/docs'],
    base: '/docs',
    state: 'awaiting',
    reason: '',
    truncated: false,
    ignoredCount: 2,
    estimate: { files: 3, chunks: 5, tokens: 9000, calls: 8, seconds: 150 },
    progress: { filesDone: 0, filesTotal: 3, chunksDone: 0, chunksTotal: 5 },
    totals: {
      added: 0,
      updated: 0,
      noop: 0,
      dropped: 0,
      unresolved: 0,
      failedFiles: 0,
      skippedFiles: 1,
    },
    channel: 'ui',
    origin: 'external',
    sourceTag: 'doc',
    scope: 'personal',
    project: null,
    sessionId: null,
    groups: [],
    autoStart: false,
    writeDoc: false,
    calls: 0,
    costUsd: 0,
    createdAt: '2026-03-01T10:00:00.000Z',
    startedAt: null,
    finishedAt: null,
    ...over,
  };
}

function file(over: Record<string, unknown>) {
  return {
    id: 'f1',
    relPath: 'billing.md',
    kind: 'markdown',
    size: 1200,
    state: 'done',
    reason: '',
    title: 'Billing',
    chunkCount: 2,
    chunksDone: 2,
    factCount: 4,
    added: 3,
    updated: 1,
    noop: 0,
    unresolved: [],
    dropped: [],
    costUsd: 0.01,
    verified: 0,
    orphaned: 0,
    groupId: '',
    prov: null,
    docPath: '',
    docError: '',
    ...over,
  };
}

// Backend values (flows/memoryflow import tests, contract memory-import); paths and times stay the
// spec's own, and the paused reason is the usage limit the fake agent cannot produce.
interface ImportDetail {
  job: Record<string, unknown>;
  files: Record<string, unknown>[];
}
const scanned = contract<ImportDetail>('memory-import', 'MemoryImportService.Job#awaiting');
const retried = contract<ImportDetail>('memory-import', 'MemoryImportService.Job#failed');
const est = scanned.job.estimate as { files: number; chunks: number; calls: number };
const failedFile = retried.files.find((f) => f.state === 'failed');
if (!failedFile) throw new Error('memory-import has no failed file');

const PAUSED = job({
  id: 'J2',
  state: 'paused',
  reason: 'Claude usage limit reached. Resume after the limit resets.',
  progress: { filesDone: 1, filesTotal: 3, chunksDone: 2, chunksTotal: 5 },
  totals: {
    added: 3,
    updated: 1,
    noop: 0,
    dropped: 0,
    unresolved: 0,
    failedFiles: 1,
    skippedFiles: 1,
  },
});

const PAUSED_DETAIL = {
  job: PAUSED,
  files: [
    file(retried.files.find((f) => f.state === 'done') ?? {}),
    file({ ...failedFile, id: 'f2', relPath: 'releases.md' }),
    file({ id: 'f3', relPath: 'logo.txt', state: 'skipped', reason: 'empty', chunkCount: 0 }),
  ],
};

const SCANNED = job({
  estimate: scanned.job.estimate,
  progress: scanned.job.progress,
  totals: scanned.job.totals,
  ignoredCount: scanned.job.ignoredCount,
});

const BASE: ControlSnapshot[] = [
  {
    channel: IPC.memoryImportChoose,
    args: { kind: 'folder' },
    response: { canceled: false, paths: ['/docs'] },
  },
  {
    channel: IPC.memoryImportCreate,
    args: { paths: ['/docs'] },
    response: job({ state: 'scanning' }),
  },
  { channel: IPC.memoryImportJob, args: { id: 'J1' }, response: { job: SCANNED, files: [] } },
  { channel: IPC.memoryImportStart },
  { channel: IPC.memoryImportJob, args: { id: 'J2' }, response: PAUSED_DETAIL },
  { channel: IPC.memoryImportResume },
  { channel: IPC.memoryImportRetryFile },
];

test('contract: import: pick a folder, confirm the estimate, start', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: withMemory([...BASE, { channel: IPC.memoryImportJobs, response: [SCANNED] }]),
  });
  await openMemory(page);

  await page.locator('[data-testid="memory-import"]').click();
  await page.locator('[data-testid="memory-import-folder"]').click();

  const dialog = page.locator('[data-testid="import-confirm-dialog"]');
  await expect(dialog.locator('[data-testid="import-estimate"]')).toContainText(
    `${est.files} files, ${est.chunks} chunks`,
  );
  await expect(dialog.locator('[data-testid="import-estimate"]')).toContainText(
    `${est.calls} Claude calls`,
  );

  await dialog.locator('[data-testid="import-confirm-start"]').click();
  await expect(dialog).toHaveCount(0);
  await expect(page.locator('[data-testid="import-job"]')).toBeVisible();
  expect(control.log().filter((e) => e.channel === IPC.memoryImportStart)).toHaveLength(1);
});

test('contract: import: a paused job shows its reason, per-file results, retry and resume', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: withMemory([...BASE, { channel: IPC.memoryImportJobs, response: [PAUSED] }]),
  });
  await openMemory(page);

  await expect(page.locator('[data-testid="memory-import-status"]')).toHaveAttribute(
    'data-state',
    'paused',
  );
  await page.locator('[data-testid="memory-import-open"]').click();

  await expect(page.locator('[data-testid="import-reason"]')).toContainText('usage limit');
  const files = page.locator('[data-testid="import-files"]');
  await expect(files.locator('[data-testid="import-file-billing.md"]')).toHaveAttribute(
    'data-state',
    'done',
  );
  await expect(files.locator('[data-testid="import-file-releases.md"]')).toHaveAttribute(
    'data-state',
    'failed',
  );
  await expect(files.locator('[data-testid="import-file-logo.txt"]')).toHaveAttribute(
    'data-state',
    'skipped',
  );

  await files.locator('[data-testid="import-file-releases.md"] button').first().click();
  await expect(page.locator('[data-testid="import-file-reason"]')).toContainText(
    String(failedFile.reason),
  );

  await files.locator('[data-testid="import-retry-file-releases.md"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.memoryImportRetryFile)?.args)
    .toMatchObject({ fileId: 'f2' });

  await page.locator('[data-testid="import-resume"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.memoryImportResume)?.args)
    .toMatchObject({ id: 'J2' });
});

// P274: provenance, doc write-back, parsed docs and the cross-check, from the Go flows' contracts.
const running = (d: ImportDetail) => ({ ...d.job, state: 'running' });

async function openImports(page: Page, detail: ImportDetail): Promise<void> {
  await openMemory(page);
  await page.locator('[data-testid="memory-import-open"]').click();
  await expect(page.locator('[data-testid="import-job"]')).toBeVisible();
  expect(detail.files.length).toBeGreaterThan(0);
}

function withDetail(detail: ImportDetail): ControlSnapshot[] {
  return [
    ...BASE,
    { channel: IPC.memoryImportJobs, response: [running(detail)] },
    { channel: IPC.memoryImportJob, args: { id: detail.job.id }, response: detail },
  ];
}

test('contract: import: the confirm dialog sends the chosen provenance and doc write-back', async ({
  relaunch,
}) => {
  const sent = contract<{
    origin: string;
    sourceTag: string;
    scope: string;
    project: string;
    writeDoc: boolean;
  }>('memory-import', 'args:MemoryImportService.Start#provenance');
  const { window: page, control } = await relaunch({
    control: withMemory([
      ...BASE,
      { channel: IPC.memoryImportJobs, response: [SCANNED] },
      { channel: IPC.memoryProjects, response: [sent.project] },
    ]),
  });
  await openMemory(page);
  await page.locator('[data-testid="memory-import"]').click();
  await page.locator('[data-testid="memory-import-folder"]').click();

  const dialog = page.locator('[data-testid="import-confirm-dialog"]');
  const start = dialog.locator('[data-testid="import-confirm-start"]');
  const tag = dialog.locator('[data-testid="import-source-tag"]');
  await expect(tag).toHaveValue('doc');
  await tag.fill('Not A Tag');
  await expect(dialog.locator('[data-testid="import-source-tag-error"]')).toBeVisible();
  await expect(start).toBeDisabled();

  await dialog.locator(`[data-testid="import-origin-${sent.origin}"]`).click();
  await expect(tag).toHaveCount(0);
  await dialog.locator(`[data-testid="import-scope-${sent.scope}"]`).click();
  await expect(start).toBeDisabled();
  await dialog.locator('[data-testid="import-project"]').click();
  await page.locator(`[data-testid="import-project-option-${sent.project}"]`).click();
  await expect(start).toBeEnabled();
  if (sent.writeDoc) await dialog.locator('[data-testid="import-write-doc"]').click();
  await start.click();

  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.memoryImportStart)?.args)
    .toMatchObject({
      origin: sent.origin,
      sourceTag: sent.sourceTag,
      scope: sent.scope,
      project: sent.project,
      writeDoc: sent.writeDoc,
    });
});

test('contract: import: a finished file with a written doc shows the doc path', async ({
  relaunch,
}) => {
  const written = contract<ImportDetail>('memory-import', 'MemoryImportService.Job#writeDoc');
  const { window: page } = await relaunch({ control: withMemory(withDetail(written)) });
  await openImports(page, written);

  await expect(page.locator('[data-testid="import-job-origin"]')).toHaveText('User');
  await expect(page.locator('[data-testid="import-job-scope"]')).toHaveText(
    String(written.job.project),
  );
  await page.locator('[data-testid="import-files"] button').first().click();
  await expect(page.locator('[data-testid="import-file-doc"]')).toBeVisible();
  await expect(page.locator('[data-testid="import-file-doc-error"]')).toHaveCount(0);
});

test('contract: import: a kira-memory doc shows as a parsed file with its counts', async ({
  relaunch,
}) => {
  const parsed = contract<ImportDetail>('memory-import', 'MemoryImportService.Job#memdoc');
  const doc = parsed.files[0] as { relPath: string; added: number; groupId: string };
  const { window: page } = await relaunch({ control: withMemory(withDetail(parsed)) });
  await openImports(page, parsed);

  const row = page.locator(`[data-testid="import-file-${doc.relPath}"]`);
  await expect(row.locator('[data-testid="import-file-kind"]')).toHaveText('Memory doc');
  await expect(row).toContainText(`+${doc.added}`);
  await row.locator('button').first().click();
  await expect(page.locator('[data-testid="import-file-group"]')).toContainText(doc.groupId);
});

test('contract: import: an agent job awaiting confirmation shows why and starts on the user', async ({
  relaunch,
}) => {
  const claims = contract<ImportDetail>('memory-import', 'MemoryImportService.Job#claims');
  const { window: page, control } = await relaunch({ control: withMemory(withDetail(claims)) });
  await openImports(page, claims);

  await expect(page.locator('[data-testid="import-reason"]')).toContainText(
    String(claims.job.reason),
  );
  await page.locator('[data-testid="import-start"]').click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.memoryImportStart).length)
    .toBe(1);
});

test('contract: import: cross-checked facts count and the unsupported one is dropped with why', async ({
  relaunch,
}) => {
  const checked = contract<ImportDetail>('memory-import', 'MemoryImportService.Job#verified');
  const f = checked.files[0] as {
    relPath: string;
    verified: number;
    dropped: { fact: string; why: string }[];
  };
  const { window: page } = await relaunch({ control: withMemory(withDetail(checked)) });
  await openImports(page, checked);

  const row = page.locator(`[data-testid="import-file-${f.relPath}"]`);
  await expect(row.locator('[data-testid="import-file-verified"]')).toContainText(
    String(f.verified),
  );
  await row.locator('button').first().click();
  const detail = page.locator('[data-testid="import-file-detail"]');
  for (const d of f.dropped) {
    await expect(detail).toContainText(d.fact);
    await expect(detail).toContainText(d.why);
  }
});
