import { expect, test } from '../ui/fixtures';

// P110 I2-1b: one baseline per settings pane, mirroring Kira Studio's own tests/visual/settings.spec.ts.
// Space has no shared openSettings() helper (tests/ui/repo-workspace.spec.ts inlines the same two
// lines), so this spec does too rather than adding one just for itself.
const sections = ['Appearance', 'Git', 'Advanced'] as const;

for (const section of sections) {
  test(`settings dialog: ${section} pane (P110 I2-1b)`, async ({ relaunch }) => {
    const { window } = await relaunch();
    await window.click('[data-testid="open-settings"]');
    await expect(window.locator('[data-testid="settings-dialog"]')).toBeVisible();
    if (section !== 'Appearance') {
      await window.click(`[data-testid="settings-section-${section}"]`);
    }
    await expect(window.locator('[data-testid="settings-dialog"]')).toHaveScreenshot(
      `settings-${section.toLowerCase().replace(/\s+/g, '-')}.png`,
    );
  });
}
