import { codeEditorText, replaceCodeEditorText } from '@workbench/testing/ui/codeEditor';
import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// The Agent config tab: Context and Settings sub-tabs share one unsaved-edits guard.

const t = (id: string) => `[data-testid="${id}"]`;

const context = {
  folder: 'agent',
  file: 'context.md',
  path: '~/.kira-space/config/agent/context.md',
  exists: true,
  text: 'Use tabs.\n',
  hash: 'h1',
  editable: true,
  problem: null,
  lastGood: false,
  maxBytes: 32768,
  keepsOnResume: [],
};

test('unsaved edits ask before switching sub-tab or leaving Agent config', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.agentContextGet, response: context },
    { channel: IPC.agentSettingsGet, response: contract('agent-settings', 'view') },
  ]);
  const dialog = page.locator(t('confirm-dialog'));

  await page.locator(t('ade-tab-agents')).click();
  await expect(page.locator(t('ade-context'))).toBeVisible();

  await page.locator(t('ade-agents-tab-settings')).click();
  await expect(page.locator(t('ade-settings'))).toBeVisible();
  await expect(dialog).toHaveCount(0);
  await page.locator(t('ade-agents-tab-context')).click();

  await replaceCodeEditorText(page, page.locator(t('ade-context-text')), 'Use spaces.\n');
  await page.locator(t('ade-agents-tab-settings')).click();
  await expect(page.locator(t('confirm-dialog-message'))).toContainText(
    'unsaved changes to the agent context',
  );
  await page.locator(t('confirm-dialog-cancel')).click();
  await expect
    .poll(() => codeEditorText(page.locator(t('ade-context-text'))))
    .toBe('Use spaces.\n');
  await expect(page.locator(t('ade-settings'))).toHaveCount(0);

  await page.locator(t('ade-agents-tab-settings')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(page.locator(t('ade-settings'))).toBeVisible();

  await replaceCodeEditorText(page, page.locator(t('ade-settings-text')), 'env:\n  A: b\n');
  await page.locator(t('ade-tab-plan')).click();
  await expect(page.locator(t('confirm-dialog-message'))).toContainText(
    'unsaved changes to the agent settings',
  );
  await page.locator(t('confirm-dialog-cancel')).click();
  await expect(page.locator(t('ade-settings'))).toBeVisible();
  await page.locator(t('ade-tab-plan')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(page.locator(t('ade-agents'))).toHaveCount(0);
});
