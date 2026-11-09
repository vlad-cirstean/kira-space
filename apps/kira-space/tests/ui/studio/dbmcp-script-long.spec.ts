import { scriptRunLogPageSchema, scriptRunSchema } from '@shared/domain/scriptRuns';
import { scriptsSnapshotSchema } from '@shared/domain/scripts';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openSettings } from './support/settings';

// Contract dbmcp-script-long. Backend half: dbmcpflow TestDbMcpScriptLongRunning (an agent starts a
// script with mcp.mode long, reads its output while it runs, then its exit state) and termflow
// TestRunsForm (the mcp.mode edit).

const snapshot = () =>
  contract('dbmcp-script-long', 'CustomScriptsService.List#long', {
    schema: scriptsSnapshotSchema,
  });
const running = () => contract<unknown[]>('dbmcp-script-long', 'ScriptRunsService.List#running');
const log = () =>
  contract('dbmcp-script-long', 'ScriptRunsService.ReadLog#running', {
    schema: scriptRunLogPageSchema,
  });
const ended = () =>
  contract('dbmcp-script-long', 'ScriptRunsService.Get#ended', { schema: scriptRunSchema });

async function openRun(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  runs: unknown[],
) {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.customScriptsList, response: snapshot() },
      { channel: IPC.scriptRunsList, response: runs },
      { channel: IPC.scriptRunsReadLog, response: log() },
    ],
  });
  await page.locator('[data-testid="toggle-operations-panel"]').click();
  const op = page.locator(`[data-op-id="run:${(runs[0] as { id: string }).id}"]`);
  await expect(op).toContainText('Agent run');
  await op.click({ button: 'right' });
  await page.locator('[data-testid="menu-item-open-run"]').click();
  return page;
}

test('contract: the settings row marks a long-running script', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.customScriptsList, response: snapshot() }],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  await expect(page.locator('[data-testid="db-mcp-script-long-serve"]')).toHaveText('Long running');
  await expect(page.locator('[data-testid="db-mcp-script-long-quick"]')).toHaveCount(0);
});

test('contract: a long script run shows its output while it runs', async ({ relaunch }) => {
  const page = await openRun(relaunch, running());
  await expect(
    page.locator('[data-testid="script-run-view"] [data-testid="run-log"]'),
  ).toContainText('started');
  await expect(page.locator('[data-testid="run-stop"]')).toBeVisible();
  await expect(page.locator('[data-testid="script-run-result"]')).toHaveCount(0);
});

test('contract: after the process exits the run shows its failed exit state', async ({
  relaunch,
}) => {
  const run = ended();
  const page = await openRun(relaunch, [{ ...run, id: 'run-long' }]);
  await expect(page.locator('[data-testid="run-outcome-exit"]')).toContainText('4');
  await expect(page.locator('[data-testid="run-stop"]')).toHaveCount(0);
});
