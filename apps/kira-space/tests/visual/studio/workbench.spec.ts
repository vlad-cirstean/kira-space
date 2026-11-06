import { expect, test } from '../../ui/studio/fixtures';

// The full workbench shell at rest, no
// connections/tabs open — the exact empty-boot state smoke.spec.ts already proves stable,
// and the same class of failure (a visually-collapsed region whose DOM/aria state still reports
// healthy) found in the webview tier's own layout tests, which this tier has no equivalent
// guard for at all.
test('workbench shell at rest (P6)', async ({ kira }) => {
  const { window } = kira;

  await expect(window.locator('[data-testid="project-panel"]')).toBeVisible();
  await expect(window.locator('[data-testid="tab-strip"]')).toBeVisible();
  await expect(window.locator('[data-testid="main-view"]')).toBeVisible();
  await expect(window.locator('[data-testid="status-bar"]')).toBeVisible();

  await expect(window).toHaveScreenshot('workbench-shell.png');
});
