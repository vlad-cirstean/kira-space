import { customScriptSchema, scriptsSnapshotSchema } from '@shared/domain/scripts';
import { registerFileScenarios } from '@workbench/testing/ui/scenarios/automationsFile';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P472: a run is a YAML file; the panel lists them and a dialog edits the text. Backend half:
// termflow TestRunsConfigFile stores the runs-file contract.

registerFileScenarios(
  { test, contract, ipc: IPC },
  {
    extraBase: [{ channel: IPC.windowsEnsure, response: { mode: 'git' } }],
    modeTab: (page) => page.locator('[data-testid="mode-tab"][data-mode="automations"]'),
  },
);

test('contract: a script whose fixed folder is gone still lists', async ({ relaunch }) => {
  const fixed = contract('script-folders', 'CustomScriptsService.Create#fixed', {
    schema: customScriptSchema,
  });
  expect(fixed.dirMode).toBe('fixed');
  const valid = contract('runs-file', 'snapshot#valid', { schema: scriptsSnapshotSchema });
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.windowsEnsure, response: { mode: 'git' } },
      { channel: IPC.customScriptsList, response: { ...valid, scripts: [fixed] } },
    ],
  });
  await page.locator('[data-testid="mode-tab"][data-mode="automations"]').click();
  await expect(page.locator(`[data-testid="script-${fixed.id}"]`)).toBeVisible();
});
