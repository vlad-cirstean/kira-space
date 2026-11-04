import { expect, test } from './fixtures';
import { openPortGraph, PORT_REPO } from './support/gitUiPortFixtures';
import {
  buildGraphStreamChunk,
  buildMultiBranchChunk,
  buildMultiBranchRefsList,
} from './support/graphStreamFixture';

// Every graph toolbar divider is the shared ToolbarSeparator: 1x14, centred in the bar, transparent spacer.
// Studio's DataToolbar asserts the same numbers (data-view.spec.ts).

test('graph toolbar dividers are 1x14, centred, transparent', async ({ relaunch }) => {
  const page = await openPortGraph(relaunch, {
    chunks: [buildGraphStreamChunk(PORT_REPO.repoId, 0, buildMultiBranchChunk())],
    results: { 'refs.list': buildMultiBranchRefsList() },
  });
  const toolbar = page.locator('[role="toolbar"]').first();
  await expect(toolbar).toBeVisible();
  const separators = toolbar.locator('[data-testid="toolbar-separator"]');
  await expect(separators.first()).toBeVisible();

  const expected = await page.evaluate(() => {
    const probe = document.createElement('div');
    probe.className = 'bg-transparent';
    document.body.appendChild(probe);
    const color = getComputedStyle(probe).backgroundColor;
    probe.remove();
    return color;
  });
  const bar = await toolbar.boundingBox();
  if (!bar) throw new Error('toolbar has no bounding box');

  const count = await separators.count();
  expect(count).toBeGreaterThanOrEqual(2);
  for (let i = 0; i < count; i++) {
    const sep = separators.nth(i);
    if (!(await sep.isVisible())) continue;
    const box = await sep.boundingBox();
    if (!box) throw new Error('separator has no bounding box');
    expect(box.width).toBe(1);
    expect(box.height).toBe(14);
    expect(Math.abs(box.y + box.height / 2 - (bar.y + bar.height / 2))).toBeLessThanOrEqual(0.5);
    await expect(sep).toHaveCSS('background-color', expected);
  }
});
