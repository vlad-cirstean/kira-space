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
const journeyImport = contract<{ token: string; items: { id: string; state: string }[] }>(
  'agent-port-journey',
  'import',
);
const journeyExport = contract<Record<string, unknown> & { files: { role: string }[] }>(
  'agent-port-journey',
  'export',
);
const journeyBackups = contract<{ file: string; role: string; restorable: boolean }[]>(
  'agent-port-journey',
  'backups',
);
const journeyRestore = contract<{ written: string[]; backups: string[] }>(
  'agent-port-journey',
  'restore',
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
  await expect(dialog.locator(t('ade-port-written'))).toHaveCount(0);
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

// J-HAR-002, frontend half: the flow test TestAgentPortClaudeJourney runs the same scenario on the
// backend and asserts the same results (masked previews, backups, restore) from the same contracts.
test('journey: import masks secrets, export shows statusline and skips, backups list, restore', async ({
  relaunch,
}) => {
  const { window: page, control } = await openSettings(relaunch, [
    { channel: IPC.agentPortImportPreview, response: journeyImport },
    { channel: IPC.agentPortExportPreview, response: journeyExport },
    { channel: IPC.agentPortBackups, response: journeyBackups },
    { channel: IPC.agentPortRestore, response: journeyRestore },
  ]);

  await page.locator(t('ade-port-import')).click();
  const imp = page.locator(t('ade-port-import-dialog'));
  await imp.locator(t('ade-port-items')).waitFor();
  await reveal(imp, 'mcp/gh');
  const slots = imp.locator(t('ade-port-secret'));
  await expect(slots.first()).toBeVisible();
  for (const text of await slots.allTextContents()) expect(text).toContain('••••');
  await expect(imp).not.toContainText('p635-');
  await reveal(imp, 'claude/status_line');
  await expect(imp.locator(t('ade-port-item-claude/status_line'))).toHaveAttribute(
    'data-state',
    'new',
  );
  await imp.locator(t('ade-port-close')).click();

  await page.locator(t('ade-port-export')).click();
  const exp = page.locator(t('ade-port-export-dialog'));
  await exp.locator(t('ade-port-files')).waitFor();
  await exp.locator(t('ade-port-file-settings')).getByRole('button').first().click();
  const diff = exp.locator('[data-kira-diff-original-text]').first();
  await expect(diff).toHaveAttribute('data-kira-diff-modified-text', /refreshInterval/);
  await expect(diff).toHaveAttribute('data-kira-diff-original-text', /••••/);
  await expect(diff).not.toHaveAttribute('data-kira-diff-original-text', /p635-/);
  await expect(exp.locator(t('ade-port-skipped'))).toContainText('env.JIRA_TOKEN');
  await exp.locator(t('ade-port-close')).click();

  await page.locator(t('ade-port-backups-open')).click();
  const dialog = page.locator(t('ade-port-backups-dialog'));
  await dialog.locator(t('ade-port-backup-list')).waitFor();
  const ids = await dialog
    .locator('[data-role]')
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-testid')));
  expect(ids).toEqual(journeyBackups.map((b) => `ade-port-backup-${b.file}`));
  const mcp = journeyBackups.find((b) => b.role === 'mcp');
  if (!mcp) throw new Error('no mcp backup in the contract');
  expect(mcp.restorable).toBe(false);
  await expect(dialog.locator(t(`ade-port-restore-${mcp.file}`))).toBeDisabled();
  await expect(dialog.locator(t(`ade-port-backup-reason-${mcp.file}`))).toBeVisible();

  const oldest = journeyBackups.filter((b) => b.role === 'settings').at(-1);
  if (!oldest) throw new Error('no settings backup in the contract');
  await dialog.locator(t(`ade-port-restore-${oldest.file}`)).click();
  await expect(page.locator(t('confirm-dialog'))).toBeVisible();
  await expect(page.locator(t('confirm-dialog-message'))).toContainText(oldest.file);
  await page.locator(t('confirm-dialog-confirm')).click();
  await dialog.locator(t('ade-port-result')).waitFor();
  expect(calls(control, IPC.agentPortRestore)).toHaveLength(1);
  expect(calls(control, IPC.agentPortRestore)[0]?.args).toEqual({
    provider: 'claude',
    file: oldest.file,
  });
  await expect(dialog.locator(t('ade-port-written'))).toContainText(
    journeyRestore.written[0]?.split('/').pop() ?? '',
  );
  await expect(dialog.locator(t('ade-port-backups'))).toContainText(
    journeyRestore.backups[0]?.split('/').pop() ?? '',
  );
  await expect.poll(() => calls(control, IPC.agentPortBackups)).toHaveLength(2);
});

test('restore cancelled at the confirm sends nothing', async ({ relaunch }) => {
  const { window: page, control } = await openSettings(relaunch, [
    { channel: IPC.agentPortBackups, response: journeyBackups },
    { channel: IPC.agentPortRestore, response: journeyRestore },
  ]);
  await page.locator(t('ade-port-backups-open')).click();
  const dialog = page.locator(t('ade-port-backups-dialog'));
  const first = journeyBackups.find((b) => b.restorable);
  if (!first) throw new Error('no restorable backup in the contract');
  await dialog.locator(t(`ade-port-restore-${first.file}`)).click();
  await page.locator(t('confirm-dialog-close')).click();
  await expect(page.locator(t('confirm-dialog'))).toHaveCount(0);
  expect(calls(control, IPC.agentPortRestore)).toHaveLength(0);
  await expect(dialog.locator(t('ade-port-result'))).toHaveCount(0);
});

test('export apply reloads the backups list', async ({ relaunch }) => {
  const { window: page, control } = await openSettings(relaunch, [
    { channel: IPC.agentPortExportPreview, response: exportPreview },
    { channel: IPC.agentPortExportApply, response: applied },
    { channel: IPC.agentPortBackups, response: journeyBackups },
  ]);
  await page.locator(t('ade-port-backups-open')).click();
  const dialog = page.locator(t('ade-port-backups-dialog'));
  await dialog.locator(t('ade-port-backup-list')).waitFor();
  await dialog.locator(t('ade-port-close')).click();
  expect(calls(control, IPC.agentPortBackups)).toHaveLength(1);

  await page.locator(t('ade-port-export')).click();
  const exp = page.locator(t('ade-port-export-dialog'));
  await exp.locator(t('ade-port-files')).waitFor();
  await exp.locator(t('ade-port-apply')).click();
  await exp.locator(t('ade-port-result')).waitFor();
  await exp.locator(t('ade-port-close')).click();
  await page.locator(t('ade-port-backups-open')).click();
  await expect.poll(() => calls(control, IPC.agentPortBackups)).toHaveLength(2);
});
