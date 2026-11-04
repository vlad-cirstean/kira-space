import type { Locator } from '@playwright/test';
import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Agents > Settings: Import from and Export to a provider's own user-level config. The fixtures are the
// Go flow test's own answers (tests/contract/agent-port-import.json and agent-port-export.json).

const t = (id: string) => `[data-testid="${id}"]`;

const settings = contract<Record<string, unknown>>('agent-settings', 'view');
const importPreview = contract<{
  token: string;
  items: { id: string; state: string }[];
}>('agent-port-import', 'preview');
const exportPreview = contract<Record<string, unknown> & { files: { role: string }[] }>(
  'agent-port-export',
  'preview',
);
const providers = [
  { id: 'claude', label: 'Claude', files: [] },
  { id: 'opencode', label: 'OpenCode', files: [] },
];
const applied = {
  written: ['/h/settings.yaml'],
  backups: ['/h/.claude/settings.json.kira-backup-20260101T000000Z'],
  servers: [{ name: 'gh', outcome: 'replaced', detail: '' }],
  secrets: ['gh-authorization'],
  notes: [],
};

async function openSettings(
  relaunch: Parameters<typeof openPlan>[0],
  extra: Parameters<typeof openPlan>[1] = [],
) {
  const app = await openPlan(relaunch, [
    { channel: IPC.agentSettingsGet, response: settings },
    { channel: IPC.agentPortList, response: providers },
    ...extra,
  ]);
  await app.window.locator(t('ade-tab-agents')).click();
  await app.window.locator(t('ade-agents-tab-settings')).click();
  await app.window.locator(t('ade-settings-port')).waitFor();
  return app;
}

// The item list is virtual: scroll in steps until the row is rendered.
async function reveal(dialog: Locator, id: string) {
  const list = dialog.locator(t('ade-port-items'));
  const row = dialog.locator(t(`ade-port-item-${id}`));
  await list.evaluate((el) => {
    el.scrollTop = 0;
  });
  // The window re-renders a frame after the scroll; a row left from the last window is stale.
  await dialog.page().waitForTimeout(150);
  for (let i = 0; i < 40 && (await row.count()) === 0; i++) {
    await list.evaluate((el) => {
      el.scrollTop += 150;
    });
    await dialog.page().waitForTimeout(80);
  }
  await expect(row).toBeVisible();
}

test('Claude and OpenCode show Import and Export, a provider the app cannot port shows none', async ({
  relaunch,
}) => {
  const { window: page } = await openSettings(relaunch);
  await expect(page.locator(t('ade-port-import'))).toContainText('Import from Claude');
  await expect(page.locator(t('ade-port-export'))).toContainText('Export to Claude');
  await page.locator(t('ade-settings-result')).getByText('OpenCode', { exact: true }).click();
  await expect(page.locator(t('ade-port-import'))).toContainText('Import from OpenCode');
  await page.locator(t('ade-settings-result')).getByText('Fake agent', { exact: true }).click();
  await expect(page.locator(t('ade-settings-port'))).toHaveCount(0);
});

test('Import lists the items with states, secrets as dots, and sends the picked ones once', async ({
  relaunch,
}) => {
  const { window: page, control } = await openSettings(relaunch, [
    { channel: IPC.agentPortImportPreview, response: importPreview },
    { channel: IPC.agentPortImportApply, response: applied },
  ]);
  await page.locator(t('ade-port-import')).click();
  const dialog = page.locator(t('ade-port-import-dialog'));
  await dialog.locator(t('ade-port-items')).waitFor();
  await expect(dialog.locator(t('ade-port-sources'))).toContainText('~/.claude.json');
  expect(calls(control, IPC.agentPortImportPreview)[0]?.args).toEqual({ provider: 'claude' });

  const row = (id: string) => dialog.locator(t(`ade-port-item-${id}`));
  const check = (id: string) => dialog.locator(t(`ade-port-check-${id}`));
  await expect(row('env/JIRA_TOKEN').locator(t('ade-port-secret'))).toContainText('••••');
  await expect(dialog).not.toContainText('sk-p454-');
  await reveal(dialog, 'mcp/notes');
  await expect(row('mcp/notes')).toHaveAttribute('data-state', 'new');
  await expect(check('mcp/notes')).toBeChecked();
  await expect(row('mcp/gh')).toHaveAttribute('data-state', 'conflict');
  await expect(check('mcp/gh')).not.toBeChecked();
  await reveal(dialog, 'mcp/old');
  await expect(row('mcp/old')).toHaveAttribute('data-state', 'skipped');
  await expect(check('mcp/old')).toBeDisabled();
  await reveal(dialog, 'mcp/notes');
  await expect(row('mcp/notes').locator(t('ade-port-secret'))).toContainText('••••');
  await reveal(dialog, 'mcp/gh');
  await check('mcp/gh').click();
  await reveal(dialog, 'context');
  await check('context').click();
  await dialog.locator(t('ade-port-mode-context')).selectOption('append');
  await dialog.locator(t('ade-port-apply')).click();
  await dialog.locator(t('ade-port-result')).waitFor();
  const sent = calls(control, IPC.agentPortImportApply);
  expect(sent).toHaveLength(1);
  const args = sent[0]?.args as { token: string; items: { id: string; mode: string }[] };
  expect(args.token).toBe(importPreview.token);
  expect(args.items.find((i) => i.id === 'mcp/gh')).toEqual({ id: 'mcp/gh', mode: '' });
  expect(args.items.find((i) => i.id === 'context')).toEqual({ id: 'context', mode: 'append' });
  expect(args.items.some((i) => i.id === 'mcp/old')).toBe(false);
  await expect(dialog.locator(t('ade-port-written'))).toContainText('settings.yaml');
});

test('a preview that went stale offers Preview again', async ({ relaunch }) => {
  const { window: page, control } = await openSettings(relaunch, [
    { channel: IPC.agentPortImportPreview, response: importPreview },
    {
      channel: IPC.agentPortImportApply,
      error: { code: 'E_CONFLICT', message: 'A file changed since the preview. Preview again.' },
    },
  ]);
  await page.locator(t('ade-port-import')).click();
  const dialog = page.locator(t('ade-port-import-dialog'));
  await dialog.locator(t('ade-port-apply')).click();
  await expect(dialog.locator(t('ade-port-error'))).toContainText('changed since the preview');
  await dialog.locator(t('ade-port-retry')).click();
  await expect.poll(() => calls(control, IPC.agentPortImportPreview)).toHaveLength(2);
  await expect(dialog.locator(t('ade-port-error'))).toHaveCount(0);
});

test('Export shows the diffs, server actions and env names, and sends the unchecked set', async ({
  relaunch,
}) => {
  const { window: page, control } = await openSettings(relaunch, [
    { channel: IPC.agentPortExportPreview, response: exportPreview },
    { channel: IPC.agentPortExportApply, response: applied },
  ]);
  await page.locator(t('ade-port-export')).click();
  const dialog = page.locator(t('ade-port-export-dialog'));
  await dialog.locator(t('ade-port-files')).waitFor();
  await expect(dialog.locator(t('ade-port-server-gh'))).toContainText('Replace');
  await expect(dialog.locator(t('ade-port-server-tool'))).toContainText('Add');
  await expect(dialog.locator(t('ade-port-env-lines'))).toHaveText(
    'export HOME=\nexport KIRA_SECRET_GH_TOKEN=',
  );
  await expect(dialog.locator(t('ade-port-skipped'))).toContainText('env.P454_E');
  await expect(dialog.locator(t('ade-port-cli-missing'))).toHaveCount(0);

  await dialog.locator(t('ade-port-file-settings')).getByRole('button').first().click();
  const diff = dialog.locator('[data-kira-diff-original-text]').first();
  await expect(diff).toHaveAttribute('data-kira-diff-original-text', /••••/);
  await expect(diff).not.toHaveAttribute('data-kira-diff-original-text', /sk-p454-/);

  await dialog.locator(t('ade-port-file-check-context')).click();
  await dialog.locator(t('ade-port-apply')).click();
  await dialog.locator(t('ade-port-result')).waitFor();
  expect(calls(control, IPC.agentPortExportApply)[0]?.args).toEqual({
    token: exportPreview.token,
    files: ['settings'],
    servers: true,
  });
  await expect(dialog.locator(t('ade-port-backups'))).toContainText(
    'settings.json.kira-backup-20260101T000000Z',
  );
});

test('a missing CLI shows the alert and leaves servers out', async ({ relaunch }) => {
  const { window: page } = await openSettings(relaunch, [
    { channel: IPC.agentPortExportPreview, response: { ...exportPreview, cliMissing: true } },
  ]);
  await page.locator(t('ade-port-export')).click();
  const dialog = page.locator(t('ade-port-export-dialog'));
  await expect(dialog.locator(t('ade-port-cli-missing'))).toBeVisible();
  await expect(dialog.locator(t('ade-port-servers-check'))).toBeDisabled();
});
