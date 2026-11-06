import { expect, test } from './fixtures';
import { pushCacheStats } from './support/mockStream';
import { openSettings } from './support/settings';

// P236: the cache readout moved from the status bar to Settings > Cache. The engine pushes
// cache:stats on change; the backend half is adapterhost's TestAttachStream_PushesCacheStatsOnChange.

const MIB = 1024 * 1024;

test('the status bar has no cache readout; Settings > Cache shows usage and hit rate', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch();
  await pushCacheStats(page, {
    l2Bytes: MIB,
    l2BudgetBytes: 64 * MIB,
    l2Entries: 2,
    l2Hits: 3,
    l2Misses: 1,
    l3Entries: 0,
  });
  await expect(page.locator('[data-testid="cache-size"]')).toHaveCount(0);

  await openSettings(page);
  await page.click('[data-testid="settings-section-Cache"]');
  const inputs = page.locator('[data-testid="settings-dialog"] input[disabled]');
  await expect(inputs.nth(0)).toHaveValue('1.0 MB / 64.0 MB');
  await expect(inputs.nth(1)).toHaveValue('75% (3/4)');
});

test('Clear caches sends cache:clear', async ({ relaunch }) => {
  const { window: page, stream } = await relaunch({
    stream: [{ op: 'cache:clear', payload: null }],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Cache"]');
  await page.click('[data-testid="settings-clear-caches"]');
  await expect
    .poll(async () => (await stream.ops()).filter((o) => o.op === 'cache:clear').length)
    .toBe(1);
});
