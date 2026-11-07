import type { Page } from '@playwright/test';
import { defaultSettings } from '../../frontend/src/state/settingsDomain';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { withMemory } from './support/memoryNav';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// P221: memory setup (Claude Code registration, semantic model download) lives in
// Settings > Memory, against a mocked bridge.

function semantic(state: string, over: Record<string, unknown> = {}): ControlSnapshot {
  return {
    channel: IPC.memorySemanticStatus,
    response: { state, message: '', model: '', done: 0, total: 0, ...over },
  };
}

async function openMemorySettings(page: Page): Promise<void> {
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await page.locator('[data-testid="settings-section-Memory"]').click();
}

// Contract memory-settings. Backend half: memoryflow TestConnectClaudeCode, TestSemanticNotInstalled
// and TestInstallSemanticModelCancelled.
test('contract: Connect Claude Code shows the registration command', async ({ relaunch }) => {
  const status = contract<{ command: string }>('memory-settings', 'MemoryService.McpStatus');
  const installed = contract<{ outcome: string }>('memory-settings', 'MemoryService.InstallAgent');
  expect(installed.outcome).toBe('installed');
  const { window: page } = await relaunch({
    control: withMemory([
      { channel: IPC.memoryMcpStatus, response: status },
      { channel: IPC.memoryMcpInstall, response: installed },
    ]),
  });
  await openMemorySettings(page);
  await expect(page.locator('[data-testid="memory-mcp-command"]')).toContainText('kira-memory');
  await page.locator('[data-testid="memory-mcp-install"]').click();
  await expect(page.locator('[data-testid="memory-mcp-install-outcome"]')).toContainText(
    'Registered',
  );
});

// Contract memory-settings. Backend half: memoryflow TestCheckHook.
test('contract: check hook shows the install commands and copies each', async ({ relaunch }) => {
  const check = contract<{ installCommand: string; hookLine: string }>(
    'memory-settings',
    'MemoryService.CheckHook',
  );
  expect(check.installCommand).toContain('memory-check --install-hook');
  expect(check.hookLine).toContain('memory-check --staged');
  const { window: page } = await relaunch({
    control: withMemory([{ channel: IPC.memoryCheckHook, response: check }]),
  });
  await openMemorySettings(page);
  await expect(page.locator('[data-testid="memory-check-section"]')).toBeVisible();
  await expect(page.locator('[data-testid="memory-check-install"]')).toHaveText(
    check.installCommand,
  );
  await expect(page.locator('[data-testid="memory-check-hook"]')).toHaveText(check.hookLine);
  await page.evaluate(() => {
    const w = window as unknown as { __clipboard: string[] };
    w.__clipboard = [];
    navigator.clipboard.writeText = async (text: string) => {
      w.__clipboard.push(text);
    };
  });
  const copied = () =>
    page.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard);
  await page.locator('[data-testid="memory-check-copy-install"]').click();
  await expect.poll(copied).toEqual([check.installCommand]);
  await page.locator('[data-testid="memory-check-copy-hook"]').click();
  await expect.poll(copied).toEqual([check.installCommand, check.hookLine]);
});

for (const key of ['not-installed', 'cancelled'] as const) {
  test(`contract: semantic ${key}: download logs the install`, async ({ relaunch }) => {
    const status = contract<{ state: string }>(
      'memory-settings',
      `MemoryService.SemanticStatus#${key}`,
    );
    expect(status.state).toBe('notInstalled');
    const { window: page, control } = await relaunch({
      control: withMemory([
        { channel: IPC.memorySemanticStatus, response: status },
        { channel: IPC.memorySemanticInstall, response: undefined },
      ]),
    });
    await openMemorySettings(page);
    const download = page.locator('[data-testid="memory-semantic-download"]');
    await expect(download).toContainText('35.0 MB');
    await download.click();
    await expect
      .poll(() => control.log().some((e) => e.channel === IPC.memorySemanticInstall))
      .toBe(true);
  });
}

// Contract memory-settings. Backend half: memoryflow TestInstallSemanticModel.
test('contract: semantic install shows the download progress, then the installed state', async ({
  relaunch,
}) => {
  const notInstalled = contract<{ state: string }>(
    'memory-settings',
    'MemoryService.SemanticStatus#not-installed',
  );
  const downloading = contract<{ state: string }>(
    'memory-settings',
    'MemoryService.SemanticStatus#downloading',
  );
  const installed = contract<{ state: string }>(
    'memory-settings',
    'MemoryService.SemanticStatus#installed',
  );
  expect(downloading.state).toBe('downloading');
  const { window: page, control } = await relaunch({
    control: withMemory([
      { channel: IPC.memorySemanticStatus, response: notInstalled },
      { channel: IPC.memorySemanticStatus, response: downloading },
      { channel: IPC.memorySemanticStatus, response: installed },
      { channel: IPC.memorySemanticInstall, response: undefined, hold: true },
    ]),
  });
  await openMemorySettings(page);
  const section = page.locator('[data-testid="memory-semantic"]');
  await expect(section).toHaveAttribute('data-state', 'notInstalled');
  await page.locator('[data-testid="memory-semantic-download"]').click();
  await emitWailsEvent(page, IPC.memorySemantic, undefined);
  await expect(section).toHaveAttribute('data-state', 'downloading');
  await expect(page.locator('[data-testid="memory-semantic-cancel"]')).toBeVisible();
  control.release(IPC.memorySemanticInstall);
  await expect(section).toHaveAttribute('data-state', installed.state);
});

// Contract memory-settings. Backend half: memoryflow TestInstallSemanticModelChecksum.
test('contract: semantic install checksum failure shows the error', async ({ relaunch }) => {
  const failure = contract<{ code: string; message: string }>(
    'memory-settings',
    'MemoryService.InstallSemanticModel#checksum',
  );
  const status = contract<{ state: string }>(
    'memory-settings',
    'MemoryService.SemanticStatus#not-installed',
  );
  const { window: page } = await relaunch({
    control: withMemory([
      { channel: IPC.memorySemanticStatus, response: status },
      { channel: IPC.memorySemanticInstall, error: failure },
    ]),
  });
  await openMemorySettings(page);
  await page.locator('[data-testid="memory-semantic-download"]').click();
  await expect(page.locator('[data-testid="memory-semantic-error"]')).toContainText(
    failure.message,
  );
});

test('semantic unavailable: shows the reason and a retry', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: withMemory([
      semantic('unavailable', { message: 'model corrupt' }),
      { channel: IPC.memorySemanticRetry, response: undefined },
    ]),
  });
  await openMemorySettings(page);
  await expect(page.locator('[data-testid="memory-semantic-error"]')).toContainText(
    'model corrupt',
  );
  await page.locator('[data-testid="memory-semantic-retry"]').click();
  await expect
    .poll(() => control.log().some((e) => e.channel === IPC.memorySemanticRetry))
    .toBe(true);
});

// Contract memory-settings. Backend half: memoryflow TestMemoryModels.
test('contract: memory model saves memory.models with the dialog', async ({ relaunch }) => {
  const memory = contract<{ models: Record<string, string> }>(
    'memory-settings',
    'SettingsService.GetAll#memory',
  );
  const patch = contract<{ memory: { models: Record<string, string> } }>(
    'memory-settings',
    'SettingsService.Set#memory-model',
  );
  const { window: page, control } = await relaunch({
    control: withMemory([
      { channel: IPC.settingsGetAll, response: { ...defaultSettings, memory } },
    ]),
  });
  await openMemorySettings(page);
  const select = page.locator('[data-testid="settings-memory-agent-model"]');
  const reset = page.locator('[data-testid="settings-reset-memory-agentModel"]');
  await expect(select).toHaveValue('sonnet');
  await expect(reset).toBeDisabled();

  await select.selectOption(patch.memory.models.claude ?? '');
  await expect(reset).toBeEnabled();
  expect(control.log().some((e) => e.channel === IPC.settingsSet)).toBe(false);

  await page.locator('[data-testid="settings-save"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual({ patch });
});
