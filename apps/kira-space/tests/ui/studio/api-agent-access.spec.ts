import { codeEditorText, replaceCodeEditorText } from '@workbench/testing/ui/codeEditor';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { openHttpMode } from './support/apiMode';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// The Environments view lists what agents may use; the variables dialog's Agent access tab edits it
// over the same draft as the YAML tab. Backend half: apiflow.TestApiAgentAccess writes the
// api-agent-access contract.

const KEY = 'api-agent-access';
const ENV_FILE = 'staging.environment.yaml';

interface FileFx {
  file: string;
  hash: string;
  text: string;
}
interface FormFx {
  text: string;
  problem: { line: number; message: string } | null;
}
interface AccessFx {
  id: string;
  name: string;
  variables: { name: string; hosts?: string[] }[];
}

const file = (key: string) => contract<FileFx>(KEY, `file#${key}`);
const form = (name: string) => contract<FormFx>(KEY, `form#${name}`);
const access = () => contract<AccessFx[]>(KEY, 'AgentAccess');
const agentFormCall = (name: string): ControlSnapshot => ({
  channel: IPC.apiConfigForm,
  args: contract(KEY, `args:ApiConfigService.Form#${name}`),
  response: form(name),
});

const ENVIRONMENTS = [
  {
    id: access()[0]?.id,
    name: 'Staging',
    sortOrder: 0,
    isActive: false,
    color: 'none',
    description: '',
    file: ENV_FILE,
  },
];

const dlg = '[data-testid="api-variables-dialog"]';

async function openAgentTab(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  extra: ControlSnapshot[] = [],
) {
  const app = await relaunch({
    control: [
      {
        channel: IPC.collectionsList,
        response: { collections: [], items: [] },
      },
      { channel: IPC.variablesListEnvironments, response: ENVIRONMENTS },
      { channel: IPC.apiConfigAgentAccess, response: access() },
      { channel: IPC.apiConfigFile, args: { file: ENV_FILE }, response: file('read') },
      agentFormCall('read'),
      ...extra,
    ],
  });
  await openHttpMode(app.window);
  await app.window.click('[data-testid="api-environments"]');
  await app.window.click('[data-testid="api-agent-access-edit"]');
  await expect(app.window.locator(dlg)).toBeVisible();
  await expect(app.window.locator('[data-testid="api-section-agent"]')).toBeVisible();
  return app;
}

test('contract: the overview lists shared variables and hosts above Secrets', async ({
  relaunch,
}) => {
  const app = await relaunch({
    control: [
      { channel: IPC.collectionsList, response: { collections: [], items: [] } },
      { channel: IPC.variablesListEnvironments, response: ENVIRONMENTS },
      { channel: IPC.apiConfigAgentAccess, response: access() },
    ],
  });
  const page = app.window;
  await openHttpMode(page);
  await page.click('[data-testid="api-environments"]');
  const overview = page.locator('[data-testid="api-agent-access"]');
  const token = overview.locator('[data-testid="api-agent-access-var-token"]');
  await expect(token).toContainText('staging.example.com');
  await expect(token).toContainText('Secret');
  const above = await overview.boundingBox();
  const below = await page.locator('[data-testid="api-secrets"]').boundingBox();
  expect(above && below && above.y < below.y).toBe(true);
});

test('contract: Edit opens the dialog on Agent access', async ({ relaunch }) => {
  const { window: page } = await openAgentTab(relaunch);
  await expect(page.locator('[data-testid="api-agent-available-token"]')).toHaveAttribute(
    'data-state',
    'unchecked',
  );
});

test('contract: sharing a secret prefills its host and the YAML tab follows', async ({
  relaunch,
}) => {
  const { window: page, control } = await openAgentTab(relaunch, [
    agentFormCall('share'),
    { channel: IPC.apiConfigSaveFile, response: file('saved') },
  ]);
  await page.locator('[data-testid="api-agent-available-token"]').click();
  await expect(page.locator('[data-testid="api-agent-hosts-token"]')).toContainText(
    'staging.example.com',
  );
  await page.locator('[data-testid="api-tab-yaml"]').click();
  await expect
    .poll(() => codeEditorText(page.locator('[data-testid="api-variables-text"]')))
    .toBe(form('share').text);
  await page.locator('[data-testid="api-variables-save"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.apiConfigSaveFile)?.args)
    .toEqual(contract(KEY, 'args:ApiConfigService.SaveFile'));
});

test('contract: removing the last host shows the required error', async ({ relaunch }) => {
  const { window: page } = await openAgentTab(relaunch, [
    agentFormCall('share'),
    agentFormCall('no-hosts'),
  ]);
  await page.locator('[data-testid="api-agent-available-token"]').click();
  await page
    .locator('[data-testid="api-agent-hosts-token"] [data-slot="tags-input-item-delete"]')
    .click();
  await expect(page.locator('[data-testid="api-agent-hosts-error-token"]')).toBeVisible();
});

test('contract: auto-approving reads edits the environment key', async ({ relaunch }) => {
  const { window: page } = await openAgentTab(relaunch, [agentFormCall('reads')]);
  await page.locator('[data-testid="api-agent-auto-reads"]').click();
  await page.locator('[data-testid="api-tab-yaml"]').click();
  await expect
    .poll(() => codeEditorText(page.locator('[data-testid="api-variables-text"]')))
    .toBe(form('reads').text);
});

test('contract: a YAML edit sharing a variable shows in the form', async ({ relaunch }) => {
  const { window: page } = await openAgentTab(relaunch, [agentFormCall('yaml')]);
  await page.locator('[data-testid="api-tab-yaml"]').click();
  await replaceCodeEditorText(
    page,
    page.locator('[data-testid="api-variables-text"]'),
    form('yaml').text,
  );
  await page.locator('[data-testid="api-tab-agent"]').click();
  await expect(page.locator('[data-testid="api-agent-available-base_url"]')).toHaveAttribute(
    'data-state',
    'checked',
  );
});
