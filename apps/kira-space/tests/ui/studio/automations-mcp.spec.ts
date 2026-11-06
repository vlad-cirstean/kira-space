import { scriptRunSchema } from '@shared/domain/scriptRuns';
import {
  customScriptSchema,
  runFormResultSchema,
  scriptFileViewSchema,
  scriptsSnapshotSchema,
} from '@shared/domain/scripts';
import { codeEditorText } from '@workbench/testing/ui/codeEditor';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { modeTab } from './support/apiMode';
import { openSettings } from './support/settings';

// Contracts runs-form and automations. Backend half: termflow TestRunsFormStudio (the mcp edits)
// and dbmcpflow TestDbMcpScriptRunAndLogs (a run started by an agent has trigger mcp).

const file = (key: string) =>
  contract('runs-form', `file#${key}`, { schema: scriptFileViewSchema });
const form = (key: string) => contract('runs-form', `form#${key}`, { schema: runFormResultSchema });

function list() {
  const valid = contract('runs-file', 'snapshot#valid', { schema: scriptsSnapshotSchema });
  return {
    ...valid,
    scripts: valid.scripts.map((s) => ({ ...s, id: 'ship', name: 'Ship', collection: 'Ops' })),
  };
}

function base(
  formResponse: unknown = form('read'),
  extra: readonly { channel: string; response?: unknown }[] = [],
) {
  return [
    { channel: IPC.customScriptsList, response: list() },
    { channel: IPC.customScriptsFile, response: file('read') },
    { channel: IPC.customScriptsForm, response: formResponse },
    ...extra,
  ];
}

const dialog = (page: import('@playwright/test').Page) =>
  page.locator('[data-testid="run-file-dialog"]');

async function openDialog(page: import('@playwright/test').Page): Promise<void> {
  await modeTab(page, 'automations').click();
  await page.locator('[data-testid="script-ship"]').dblclick();
  await expect(dialog(page)).toBeVisible();
}

test('contract: ticking the box sends an mcp.enabled edit and the YAML tab shows the block', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: base() });
  await openDialog(page);
  await dialog(page).locator('[data-testid="run-file-tab-mcp"]').click();
  control.respond(IPC.customScriptsForm, form('mcp'));
  await expect(dialog(page).locator('[data-testid="script-mcp-description"]')).toHaveCount(0);
  await dialog(page).locator('[data-testid="script-mcp-enabled"]').click();
  await expect
    .poll(() =>
      control
        .log()
        .filter((e) => e.channel === IPC.customScriptsForm)
        .flatMap((e) => (e.args as { edits: { path: string[]; value?: unknown }[] }).edits),
    )
    .toContainEqual(expect.objectContaining({ path: ['mcp', 'enabled'], value: true }));
  await dialog(page).locator('[data-testid="run-file-tab-yaml"]').click();
  await expect
    .poll(() => codeEditorText(dialog(page).locator('[data-testid="run-file-text"]')))
    .toContain('mcp:');
});

test('a smart script has no MCP tab', async ({ relaunch }) => {
  const read = form('read');
  const { window: page } = await relaunch({
    control: base({ ...read, value: { ...read.value, kind: 'smart', prompt: 'Check the build' } }),
  });
  await openDialog(page);
  await expect(dialog(page).locator('[data-testid="run-file-tab-script"]')).toBeVisible();
  await expect(dialog(page).locator('[data-testid="run-file-tab-mcp"]')).toHaveCount(0);
});

test('contract: a problem on mcp.run badges the MCP tab and shows on the field', async ({
  relaunch,
}) => {
  const invalid = form('mcpInvalid');
  expect(invalid.problem?.path).toBe('/mcp/run');
  const { window: page, control } = await relaunch({ control: base() });
  await openDialog(page);
  await dialog(page).locator('[data-testid="run-file-tab-mcp"]').click();
  control.respond(IPC.customScriptsForm, invalid);
  await dialog(page).locator('[data-testid="script-mcp-enabled"]').click();
  await expect(dialog(page).locator('[data-testid="run-file-tab-mcp-errors"]')).toBeVisible();
  await expect(dialog(page).locator('[data-testid="run-file-save"]')).toBeDisabled();
});

test('contract: the settings switch reads the file, applies the edit and saves on the read hash', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: base(form('mcp'), [{ channel: IPC.customScriptsSaveFile, response: file('saved') }]),
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  const toggle = page.locator('[data-testid="db-mcp-script-toggle-ship"]');
  await expect(toggle).not.toBeChecked();
  await toggle.click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.customScriptsSaveFile)?.args)
    .toEqual({ file: 'ship.yaml', text: form('mcp').text, baseHash: file('read').hash });
  expect(control.log().find((e) => e.channel === IPC.customScriptsForm)?.args).toEqual({
    file: 'ship.yaml',
    text: file('read').text,
    edits: [{ path: ['mcp', 'enabled'], value: true, unset: false }],
  });
});

test('contract: a run started by an agent reads Agent run in Operations', async ({ relaunch }) => {
  const script = contract('automations', 'CustomScriptsService.Create#failed', {
    schema: customScriptSchema,
  });
  const failed = contract('automations', 'ScriptRunsService.Get#failed', {
    schema: scriptRunSchema,
  });
  const running = {
    ...failed,
    id: 'run-mcp',
    trigger: 'mcp',
    state: 'running',
    terminalId: '',
    finishedAt: null,
    outcome: null,
    createdAt: 1_000,
    startedAt: 1_000,
  };
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [script] } },
      { channel: IPC.scriptRunsList, response: [running] },
    ],
  });
  await page.locator('[data-testid="toggle-operations-panel"]').click();
  await expect(
    page.locator('[data-testid="operations-panel"] [data-op-id="run:run-mcp"]'),
  ).toContainText('Agent run');
});
