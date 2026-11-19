import { TIMING } from '@workbench/behaviour/timing';
import { expect, test } from './fixtures';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// CopyIconButton on a real host (the op log row's Copy command): the check glyph shows after a copy
// and reverts after TIMING.copiedFeedback; a rejected clipboard write turns the button error tone.
// Renderer-only: no IPC.

const RECORD = {
  id: 'op-copy-btn',
  startedAt: '2026-01-01T00:00:00.000Z',
  durationMs: 12,
  kind: 'branchCreate',
  status: 'ok',
  command: 'git branch feat main',
  error: null,
  category: 'git',
  module: 'git',
  trigger: 'user',
  subject: '',
  repoRoot: '/work/alpha',
  repoName: 'alpha',
  source: 'Kira Space',
  taskId: '',
  parentId: '',
  background: false,
  repeat: 1,
  steps: [],
  stepsDropped: 0,
  cancellable: false,
};

async function openRow(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  writeText: string,
) {
  const control: ControlSnapshot[] = [{ channel: IPC.opsRecent, response: [RECORD] }];
  const { window } = await relaunch({ control });
  await window.click('[data-testid="toggle-operations-panel"]');
  await window.evaluate((impl) => {
    navigator.clipboard.writeText = new Function(
      `return (${impl})`,
    )() as typeof navigator.clipboard.writeText;
  }, writeText);
  await window.clock.install();
  const row = window.locator('[data-testid="op-row"]').first();
  await row.hover();
  return row.locator('[data-testid="op-copy"]');
}

test('a copy shows the check glyph, then reverts after the feedback window', async ({
  relaunch,
}) => {
  const button = await openRow(relaunch, 'async () => {}');
  await expect(button.locator('.codicon-copy')).toBeVisible();
  await button.click();
  await expect(button.locator('.codicon-check')).toBeVisible();
  await expect(button).not.toHaveAttribute('data-tone', 'error');
  await button.page().clock.runFor(TIMING.copiedFeedback + 100);
  await expect(button.locator('.codicon-copy')).toBeVisible();
});

test('a rejected clipboard write puts the button in error tone', async ({ relaunch }) => {
  const button = await openRow(relaunch, "async () => { throw new Error('denied'); }");
  await button.click();
  await expect(button).toHaveAttribute('data-tone', 'error');
  await expect(button.locator('.codicon-check')).toHaveCount(0);
});
