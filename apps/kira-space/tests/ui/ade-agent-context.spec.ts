import { codeEditorText, replaceCodeEditorText } from '@workbench/testing/ui/codeEditor';
import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// The Context tab: one shared agent context file, edited as plain text.

const t = (id: string) => `[data-testid="${id}"]`;

const view = (over: Record<string, unknown> = {}) => ({
  folder: 'agent',
  file: 'context.md',
  path: '~/.kira-space/agent/context.md',
  exists: true,
  text: 'Use tabs.\n',
  hash: 'h1',
  editable: true,
  problem: null,
  lastGood: false,
  maxBytes: 32768,
  keepsOnResume: [],
  ...over,
});

async function openContext(
  relaunch: Parameters<typeof openPlan>[0],
  over: Record<string, unknown> = {},
  extra: Parameters<typeof openPlan>[1] = [],
) {
  const app = await openPlan(relaunch, [
    { channel: IPC.agentContextGet, response: view(over) },
    ...extra,
  ]);
  await app.window.locator(t('ade-tab-agents')).click();
  await app.window.locator(t('ade-agents-tab-context')).click();
  await app.window.locator(t('ade-context')).waitFor();
  await app.window.locator(t('ade-context-note')).waitFor();
  return app;
}

test('Save sends the text with the loaded hash', async ({ relaunch }) => {
  const { window: page, control } = await openContext(relaunch, {}, [
    { channel: IPC.agentContextSave, response: view({ text: 'Use spaces.\n', hash: 'h2' }) },
  ]);
  await expect.poll(() => codeEditorText(page.locator(t('ade-context-text')))).toBe('Use tabs.\n');
  await expect(page.locator(t('ade-context-text'))).toHaveAttribute('data-language', 'markdown');
  await expect(page.locator(t('ade-context-save'))).toBeDisabled();
  await replaceCodeEditorText(page, page.locator(t('ade-context-text')), 'Use spaces.\n');
  await page.locator(t('ade-context-save')).click();
  await expect.poll(() => calls(control, IPC.agentContextSave)).toHaveLength(1);
  expect(calls(control, IPC.agentContextSave)[0]?.args).toEqual({
    text: 'Use spaces.\n',
    baseHash: 'h1',
  });
  await expect(page.locator(t('ade-context-save'))).toBeDisabled();
});

test('a reload after a save shows the saved text', async ({ relaunch }) => {
  const saved = view({ text: 'Use spaces.\n', hash: 'h2' });
  const { window: page } = await openContext(relaunch, {}, [
    { channel: IPC.agentContextSave, response: saved },
    // The second answer to Get is what the file holds after the save.
    { channel: IPC.agentContextGet, response: saved },
  ]);
  await replaceCodeEditorText(page, page.locator(t('ade-context-text')), 'Use spaces.\n');
  await page.locator(t('ade-context-save')).click();
  await expect(page.locator(t('ade-context-save'))).toBeDisabled();
  await page.reload();
  await page.locator(t('ade-tab-agents')).click();
  await page.locator(t('ade-agents-tab-context')).click();
  await expect
    .poll(() => codeEditorText(page.locator(t('ade-context-text'))))
    .toBe('Use spaces.\n');
});

test('a broken file shows its line and the last-good note', async ({ relaunch }) => {
  const { window: page } = await openContext(relaunch, {
    problem: { line: 2, message: 'not valid UTF-8' },
    lastGood: true,
    editable: false,
  });
  await expect(page.locator(t('config-file-status'))).toContainText('2');
  await expect(page.locator(t('ade-context-last-good'))).toBeVisible();
  await expect(page.locator(t('ade-context-read-only'))).toBeVisible();
});

test('a missing file shows the none hint', async ({ relaunch }) => {
  const { window: page } = await openContext(relaunch, { exists: false, text: '', hash: '' });
  await expect(page.locator(t('ade-context-none'))).toContainText('context.md');
});

test('the resume note names providers that keep the recorded context', async ({ relaunch }) => {
  const { window: page } = await openContext(relaunch, { keepsOnResume: ['Claude Code'] });
  await expect(page.locator(t('ade-context-resume'))).toContainText('Claude Code');
});

test('Open names the agent folder and file', async ({ relaunch }) => {
  const { window: page, control } = await openContext(relaunch);
  await page.locator(t('config-file-open')).click();
  await expect.poll(() => calls(control, IPC.configFilesOpen)).toHaveLength(1);
  expect(calls(control, IPC.configFilesOpen)[0]?.args).toEqual({
    folder: 'agent',
    file: 'context.md',
  });
});

test('a save refused for a changed file offers Reload and Overwrite', async ({ relaunch }) => {
  const { window: page, control } = await openContext(relaunch, {}, [
    {
      channel: IPC.agentContextSave,
      error: { code: 'E_CONFLICT', message: 'The agent context changed. Refresh and retry.' },
    },
  ]);
  await replaceCodeEditorText(page, page.locator(t('ade-context-text')), 'Other.\n');
  await page.locator(t('ade-context-save')).click();
  await expect(page.locator(t('ade-context-save-error'))).toContainText('changed');
  await page.locator(t('ade-context-overwrite')).click();
  await expect.poll(() => calls(control, IPC.agentContextSave)).toHaveLength(2);
  await page.locator(t('ade-context-reload')).click();
  await expect(page.locator(t('ade-context-save-error'))).toHaveCount(0);
});
