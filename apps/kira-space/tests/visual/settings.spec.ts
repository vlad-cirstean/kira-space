import { expect, test } from '../ui/fixtures';
import { IPC } from '../ui/support/ipcChannels';
import { withMemory } from '../ui/support/memoryNav';

// P110 I2-1b: one baseline per settings pane, mirroring Kira Studio's own tests/visual/settings.spec.ts.
// Space has no shared openSettings() helper (tests/ui/repo-workspace.spec.ts inlines the same two
// lines), so this spec does too rather than adding one just for itself.
const sections = ['Appearance', 'Git', 'Memory', 'Advanced', 'Experimental'] as const;

for (const section of sections) {
  test(`settings dialog: ${section} pane (P110 I2-1b)`, async ({ relaunch }) => {
    const { window } = await relaunch({
      control: withMemory([
        {
          channel: IPC.memoryMcpStatus,
          response: {
            command: "claude mcp add-json --scope user 'kira-memory' '{}'",
            executable: '/Applications/Kira Space',
            agentAvailable: true,
            probed: [],
          },
        },
        {
          channel: IPC.memoryCheckHook,
          response: {
            executable: '/Applications/Kira Space',
            installCommand: "'/Applications/Kira Space' memory-check --install-hook",
            hookLine:
              "if [ -x '/Applications/Kira Space' ]; then '/Applications/Kira Space' memory-check --staged; fi",
          },
        },
      ]),
    });
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
