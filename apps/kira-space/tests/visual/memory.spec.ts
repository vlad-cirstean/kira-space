import { expect, test } from '../ui/fixtures';
import { contract } from '../ui/support/contract';
import { IPC } from '../ui/support/ipcChannels';
import { openMemory, withMemory } from '../ui/support/memoryNav';

// P285 Part 4: the distill view after a run, every line picked. Backend values from contract
// memory-distill.
test('memory: distill result (P285)', async ({ relaunch }) => {
  const { window } = await relaunch({
    control: withMemory([
      {
        channel: IPC.memoryDistillLimits,
        response: contract('memory-distill', 'MemoryDistillService.Limits'),
      },
      {
        channel: IPC.memoryDistillRun,
        response: contract('memory-distill', 'MemoryDistillService.Distill#done'),
      },
    ]),
  });
  await openMemory(window);
  await window.locator('[data-testid="memory-distill"]').click();
  await window
    .locator('[data-testid="distill-input"]')
    .fill('The worker keeps port 8000 open and the queue never retries.');
  await window.locator('[data-testid="distill-run"]').click();
  await expect(window.locator('[data-testid="distill-result"]')).toBeVisible();
  await expect(window.locator('[data-testid="distill-view"]')).toHaveScreenshot(
    'memory-distill-result.png',
  );
});
