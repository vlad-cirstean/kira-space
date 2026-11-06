import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { openHttpMode } from './support/apiMode';
import { collectionRow } from './support/apiRows';

// Contract api-files. Backend half: apiflow.TestApiConfigFiles writes the same fixture. One
// scenario, both halves: a collection file is read into the tree; a bad edit surfaces as a
// "Line N" problem while the rows keep the last good text; Open and Reveal go to the config files
// service for the api folder; a secret the file references is set from the Secrets section.

interface ApiFile {
  file: string;
  path: string;
  problem: { line: number; message: string } | null;
  lastGood: boolean;
}
interface ApiConfigView {
  files: ApiFile[];
  secrets: { name: string; set: boolean; referenced: boolean }[];
}

const tree = contract<{
  collections: { id: string; file: string }[];
  items: { id: string; name: string }[];
}>('api-files', 'CollectionsService.List');
const broken = contract<ApiConfigView>('api-files', 'ApiConfigService.View');
const valid: ApiConfigView = {
  ...broken,
  files: broken.files.map((f) => ({ ...f, problem: null, lastGood: false })),
};
const afterSet: ApiConfigView = {
  ...broken,
  secrets: broken.secrets.map((s) => ({ ...s, set: true })),
};

const collection = tree.collections[0];
const file = collection.file;
const problem = broken.files[0].problem;
if (!problem) throw new Error('api-files contract has no problem');

const VARIABLES = [
  {
    id: 'var-base',
    scope: 'collection',
    ownerId: collection.id,
    name: 'base_url',
    value: 'http://flow.test',
    isSecret: false,
    sortOrder: 0,
    description: '',
  },
  {
    id: 'var-token',
    scope: 'collection',
    ownerId: collection.id,
    name: 'token',
    value: '',
    isSecret: true,
    sortOrder: 1,
    description: '',
  },
];

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.collectionsList, response: tree },
  { channel: IPC.variablesListEnvironments, response: [] },
  // Two answers in order: the first load (valid), then the reload the `config` push triggers.
  { channel: IPC.apiConfigView, response: valid },
  { channel: IPC.apiConfigView, response: broken },
  {
    channel: IPC.variablesList,
    args: { scope: 'collection', ownerId: collection.id },
    response: VARIABLES,
  },
  { channel: IPC.configFilesOpen, response: undefined },
  { channel: IPC.configFilesReveal, response: undefined },
  { channel: IPC.apiConfigSetSecret, response: afterSet },
];

test('api config files — the tree shows the file, a bad edit shows its line, Open and Reveal use the api folder, a secret is set', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: CONTROL });
  await openHttpMode(page);

  // The file's collection and request are in the tree.
  await expect(collectionRow(page, collection.id)).toBeVisible();
  await collectionRow(page, collection.id).locator('[data-testid="tree-twisty"]').click();
  await expect(collectionRow(page, tree.items[0].id)).toContainText(tree.items[0].name);
  await expect(page.locator('[data-testid="api-config-problems"]')).toHaveCount(0);

  // A bad edit reaches the app as a `config` push: the strip names the line.
  await emitWailsEvent(page, IPC.apiDataChanged, { changes: [{ kind: 'config' }] });
  const strip = page.locator('[data-testid="api-config-problems"]');
  await expect(strip.locator('[data-testid="config-file-status"]')).toHaveText(
    `✕ Line ${problem.line}: ${problem.message}`,
  );
  await expect(strip.locator('[data-testid="api-config-last-good"]')).toBeVisible();
  await expect(collectionRow(page, tree.items[0].id)).toContainText(tree.items[0].name);

  // Open and Reveal name the api folder and the file.
  await strip.locator('[data-testid="config-file-open"]').click();
  await strip.locator('[data-testid="config-file-reveal"]').click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.configFilesOpen))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.configFilesOpen)?.args).toMatchObject({
    folder: 'api',
    file,
  });
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.configFilesReveal))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.configFilesReveal)?.args).toMatchObject({
    folder: 'api',
    file,
  });

  // The variable-set tab follows the last good rows, read-only, with the same problem on top.
  await collectionRow(page, collection.id).click({ button: 'right' });
  await page.click('[data-testid="menu-item-variables"]');
  const view = page.locator('[data-testid="variables-dialog"][data-scope="collection"]');
  await expect(view).toBeVisible();
  await expect(view.locator('[data-testid="config-file-status"]')).toHaveText(
    `✕ Line ${problem.line}: ${problem.message}`,
  );
  await expect(view.locator('[data-testid="variable-row"]')).toHaveCount(2);
  await expect(
    view.locator('[data-testid="variable-row"][data-id="var-base"] [data-testid="variable-value"]'),
  ).toHaveText('http://flow.test');
  await expect(
    view.locator(
      '[data-testid="variable-row"][data-id="var-token"] [data-testid="variable-value-masked"]',
    ),
  ).toBeVisible();
  await expect(view.locator('input')).toHaveCount(1); // the name filter only: no cell is editable

  // The Secrets section lists the name the file references; Set writes it without echoing it back.
  await page.click('[data-testid="api-environments"]');
  const name = broken.secrets[0].name;
  const secret = page.locator(`[data-testid="api-secret-${name}"]`);
  await expect(secret.locator('[data-testid="api-secret-state"]')).toHaveText('No value');
  await secret.locator('[data-testid="api-secret-set"]').click();
  await page.fill('[data-testid="api-secret-value"]', 'tok-123');
  await page.click('[data-testid="api-secret-save"]');
  await expect(secret.locator('[data-testid="api-secret-state"]')).toHaveText('Value set');
  const set = control.log().filter((e) => e.channel === IPC.apiConfigSetSecret);
  expect(set).toHaveLength(1);
  expect(set[0].args).toMatchObject({ name, value: 'tok-123' });
  await expect(page.locator('[data-testid="api-secret-dialog"]')).toHaveCount(0);
  expect(
    JSON.stringify(control.log().filter((e) => e.channel !== IPC.apiConfigSetSecret)),
  ).not.toContain('tok-123');
});
