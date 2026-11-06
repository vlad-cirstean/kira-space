import { expect, studioTest as test } from '../../ui/fixtures';
import { openSettings } from '../../ui/studio/support/settings';

// One baseline per settings pane. Pre-approved re-record: the run-state/error/dialog
// colour and spinner-speed disclosures never touch these panes. The fieldVariants()
// -> Field/FieldLabel conversion is measured pixel-identical, so these baselines must pass
// unchanged.
const sections = ['Appearance', 'Data', 'Cache', 'Api', 'Database MCP', 'Advanced'] as const;

for (const section of sections) {
  test(`settings dialog: ${section} pane (P110 I2-1b)`, async ({ kira }) => {
    const { window } = kira;
    await openSettings(window);
    if (section !== 'Appearance') {
      await window.click(`[data-testid="settings-section-${section}"]`);
    }
    await expect(window.locator('[data-testid="settings-dialog"]')).toBeVisible();
    await expect(window.locator('[data-testid="settings-dialog"]')).toHaveScreenshot(
      `settings-${section.toLowerCase().replace(/\s+/g, '-')}.png`,
    );
  });
}
