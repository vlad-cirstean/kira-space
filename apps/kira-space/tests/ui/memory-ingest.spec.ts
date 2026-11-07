import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { openMemory, withMemory } from './support/memoryNav';

// P274: an agent's ingest job in the imports view. Backend half: memoryflow TestMcpIngestExternal.

interface IngestFile {
  relPath: string;
  kind: string;
  groupId: string;
  added: number;
}

test('contract: ingest: an agent job shows its channel, provenance and files', async ({
  relaunch,
}) => {
  const jobs = contract<Record<string, unknown>[]>('memory-import', 'MemoryImportService.Jobs#mcp');
  const recorded = contract<{ job: { id: string }; files: IngestFile[] }>(
    'memory-import',
    'MemoryImportService.Job#mcp',
  );
  // Group ids are generated per run and masked in the contract; give each file one to show.
  const detail = {
    ...recorded,
    files: recorded.files.map((f, i) => ({ ...f, groupId: `kg_flowgroup0${i}` })),
  };
  const { window: page } = await relaunch({
    control: withMemory([
      { channel: IPC.memoryImportJobs, response: jobs.map((j) => ({ ...j, state: 'running' })) },
      { channel: IPC.memoryImportJob, args: { id: detail.job.id }, response: detail },
    ]),
  });
  await openMemory(page);
  await page.locator('[data-testid="memory-import-open"]').click();

  await expect(page.locator('[data-testid="import-job-channel"]')).toHaveText('Agent');
  await expect(page.locator('[data-testid="import-job-origin"]')).toHaveText('External · slack');
  await expect(page.locator('[data-testid="import-job-scope"]')).toHaveText('Company');

  const kinds: Record<string, string> = { json: 'JSON', markdown: 'Markdown' };
  expect(detail.files).toHaveLength(2);
  for (const f of detail.files) {
    const row = page.locator(`[data-testid="import-file-${f.relPath}"]`);
    await expect(row.locator('[data-testid="import-file-kind"]')).toHaveText(kinds[f.kind] ?? '');
    await expect(row).toContainText(`+${f.added}`);
    await row.locator('button').first().click();
    await expect(page.locator('[data-testid="import-file-group"]')).toContainText(f.groupId);
  }
});
