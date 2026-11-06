import { expect, test } from './fixtures';

// Ported from tests/e2e/smoke.spec.ts. The app-name assertion (`app.evaluate(({app}) =>
// app.getName())`) has no analogue here — there is no Electron `app` object, and the bundle's
// display name is a packaged-build property this tier's static server never sets up;
// it stays a manual macOS check.
test('workbench launches with all chrome regions and a healthy engine', async ({
  kira,
  consoleErrors,
}) => {
  // Toggling the operations panel below debounce-persists the new layout (state/layout.ts) — one
  // `layoutSet` call this test doesn't assert on, answered by mockRuntime.ts's own
  // `WILDCARD_DEFAULTS` (no fixture needs to spell it out).
  const { window } = kira;

  // 'toolbar' is intentionally not a fixed shell region any more:
  // each view now renders its own toolbar, so with no connections/tabs (this fixture's empty
  // boot) there is none — the tab strip stays mounted but empty, and MainView's own FirstRun
  // screen is what carries the "what do I do now" messaging.
  const alwaysPresent = ['project-panel', 'tab-strip', 'main-view', 'status-bar'];
  for (const testid of alwaysPresent) {
    await expect(window.locator(`[data-testid="${testid}"]`)).toBeVisible();
  }

  // The cell editor panel is driven by cell selection, not a fixed shell region — with no
  // connections/tabs open yet, nothing is selected, so it starts hidden.
  await expect(window.locator('[data-testid="cell-editor"]')).toHaveCount(0);

  // Operations panel starts hidden (defaultLayout.panel.operations.visible === false).
  await expect(window.locator('[data-testid="operations-panel"]')).toHaveCount(0);
  await window.click('[data-testid="toggle-operations-panel"]');
  await expect(window.locator('[data-testid="operations-panel"]')).toBeVisible();

  await window.screenshot({ path: 'test-results/screenshots/workbench.png' });

  expect(consoleErrors).toEqual([]);
});
