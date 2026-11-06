import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { openSettings } from './studio/support/settings';
import { openAdvanced, openWorkflows, SCRIPTS, saved } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// The Kira database tools option of a workflow step. Backend half: adeflow TestDbMcpStep (contract
// ade-db-mcp-step) saves the same option and reads it back.

const t = (id: string) => `[data-testid="${id}"]`;

interface SavedStep {
  id: string;
  session: boolean;
  smartScript: string;
  dbMcp: boolean;
}
interface SavedWorkflow {
  workflow: { steps: SavedStep[] };
}
const stepOf = (out: SavedWorkflow, id: string): SavedStep | undefined =>
  out.workflow.steps.find((x) => x.id === id);
const node = (page: Page, step: string) =>
  page.locator(`${t('ade-wf-node')}[data-step-id="${step}"]`);
const dbMcp = (page: Page) => page.locator(t('ade-wf-step-db-mcp'));

test('contract: a saved step shows the option on, and ticking it on another step saves dbMcp', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch, [
    {
      channel: IPC.adeTaskWorkflows,
      response: contract('ade-db-mcp-step', 'AdeTaskService.Workflows'),
    },
  ]);
  await node(page, 'withdb').click();
  await openAdvanced(page);
  await expect(dbMcp(page)).toBeChecked();
  await node(page, 'plain').click();
  await openAdvanced(page);
  await expect(dbMcp(page)).not.toBeChecked();
  await dbMcp(page).click();
  const out = await saved<SavedWorkflow>(page, control, 1);
  expect(stepOf(out, 'plain')?.dbMcp).toBe(true);
  expect(stepOf(out, 'withdb')?.dbMcp).toBe(true);
  expect(calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(1);
});

test('a smart script step has no option and switching to one clears it', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch, [SCRIPTS]);
  await node(page, 'plan').click();
  await openAdvanced(page);
  await dbMcp(page).click();
  await expect(dbMcp(page)).toBeChecked();
  await page.locator(t('ade-wf-step-mode-smart')).click();
  await expect(dbMcp(page)).toHaveCount(0);
  const out = await saved<SavedWorkflow>(page, control, 1);
  expect(stepOf(out, 'plan')).toMatchObject({ dbMcp: false });
  expect(stepOf(out, 'plan')?.smartScript).not.toBe('');
});

test('a manual step shows the option only with a session and clears it without one', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await node(page, 'spec').click();
  await openAdvanced(page);
  await dbMcp(page).click();
  await expect(dbMcp(page)).toBeChecked();
  await page.locator(t('ade-wf-step-session')).click();
  await expect(dbMcp(page)).toHaveCount(0);
  await page.locator(t('ade-wf-step-session')).click();
  await openAdvanced(page);
  await expect(dbMcp(page)).not.toBeChecked();
  const out = await saved<SavedWorkflow>(page, control, 1);
  expect(stepOf(out, 'spec')).toMatchObject({ session: true, dbMcp: false });
});

test('the Database MCP settings say step sessions get their own access', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  await expect(page.locator(t('dbmcp-sessions-hint'))).toHaveText(
    'Workflow steps with Kira database tools on get their own access, even with this off.',
  );
});
