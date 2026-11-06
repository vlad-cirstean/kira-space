import { expect } from '@playwright/test';
import type { QuitStatus } from '@shared/domain/prompts';
import { emitPrompts, routed } from '../prompts';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<'lifecycleCancelQuit' | 'lifecycleConfirmQuit' | 'lifecycleQuitStatus'>;

// P279: quitting while agents run asks first. Backend half: TestQuitConfirm (appflow) stores the
// status against real runs.
export function registerQuitScenarios(env: Env): void {
  const { test, contract, ipc: IPC } = env;

  const status = () => contract<QuitStatus>('quit-confirm', 'LifecycleService.QuitStatus#asking');
  const entry = routed('quit', 'app');

  async function open(relaunch: Parameters<Parameters<typeof test>[1]>[0]['relaunch']) {
    const { window: page, control } = await relaunch({
      control: [
        { channel: IPC.lifecycleQuitStatus, response: status() },
        { channel: IPC.lifecycleConfirmQuit },
        { channel: IPC.lifecycleCancelQuit },
      ],
    });
    await emitPrompts(page, [entry]);
    const dialog = page.locator('[data-testid="confirm-dialog"]');
    await expect(dialog).toBeVisible();
    return { page, control, dialog };
  }

  test('contract: the quit dialog lists the live work and Quit anyway confirms', async ({
    relaunch,
  }) => {
    const { control, dialog } = await open(relaunch);
    const s = status();
    const message = dialog.locator('[data-testid="confirm-dialog-message"]');
    await expect(message).toContainText(`Running now: ${s.agents + s.runs + s.scripts}`);
    if (s.agents) await expect(message).toContainText(`Claude sessions: ${s.agents}`);
    if (s.runs) await expect(message).toContainText(`Task runs: ${s.runs}`);
    if (s.scripts) await expect(message).toContainText(`Script runs: ${s.scripts}`);
    await expect(dialog.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText(
      'Quit anyway',
    );

    await dialog.locator('[data-testid="confirm-dialog-confirm"]').click();
    await expect
      .poll(() => control.log().some((e) => e.channel === IPC.lifecycleConfirmQuit))
      .toBe(true);
    expect(control.log().some((e) => e.channel === IPC.lifecycleCancelQuit)).toBe(false);
  });

  for (const how of ['Cancel', 'Escape'] as const) {
    test(`${how} cancels the quit`, async ({ relaunch }) => {
      const { page, control, dialog } = await open(relaunch);
      if (how === 'Cancel') await dialog.locator('[data-testid="confirm-dialog-cancel"]').click();
      else await page.keyboard.press('Escape');
      await expect
        .poll(() => control.log().filter((e) => e.channel === IPC.lifecycleCancelQuit).length)
        .toBe(1);
      expect(control.log().some((e) => e.channel === IPC.lifecycleConfirmQuit)).toBe(false);
    });
  }
}
