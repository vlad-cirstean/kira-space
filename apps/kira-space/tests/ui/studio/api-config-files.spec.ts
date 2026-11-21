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
// service for the api folder; the Variables tab shows the last good rows read-only.

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
    secretSet: false,
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
    secretSet: false,
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
  // The dialog Show in YAML opens: the file as the loader reports it, and a clean form decode.
  {
    channel: IPC.apiConfigFile,
    response: {
      ...contract<Record<string, unknown>>('api-variables-form', 'file#collection'),
      file,
      problem,
      lastGood: true,
    },
  },
  { channel: IPC.apiConfigForm, response: contract('api-variables-form', 'form#collection') },
];

test('api config files — the tree shows the file, a bad edit shows its line, Edit opens the variables dialog, the Variables tab is read-only', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await openHttpMode(page);

  // The file's collection and request are in the tree.
  await expect(collectionRow(page, collection.id)).toBeVisible();
  await collectionRow(page, collection.id).locator('[data-testid="tree-twisty"]').click();
  await expect(collectionRow(page, tree.items[0].id)).toContainText(tree.items[0].name);
  await expect(page.locator('[data-testid="api-config-problems"]')).toHaveCount(0);

  // A bad edit reaches the app as a `config` push: the strip says the file can't be read, with no line.
  await emitWailsEvent(page, IPC.apiDataChanged, { changes: [{ kind: 'config' }] });
  const strip = page.locator('[data-testid="api-config-problems"]');
  await expect(strip.locator('[data-testid="config-file-problem"]')).toContainText('Can’t be read');
  await expect(strip.locator('[data-testid="api-config-last-good"]')).toBeVisible();
  await expect(collectionRow(page, tree.items[0].id)).toContainText(tree.items[0].name);

  // Edit opens the variables dialog on the broken text.
  await strip.locator('[data-testid="api-config-problem-edit"]').click();
  await expect(page.locator('[data-testid="api-variables-dialog"]')).toBeVisible();
  await page.keyboard.press('Escape');

  // The Variables tab follows the last good rows, read-only, with the file's column marked.
  await page.click('[data-testid="api-environments"]');
  await expect(page.locator(`[data-testid="matrix-col-${collection.id}"]`)).toHaveAttribute(
    'data-broken',
    'true',
  );
  await expect(page.locator('[data-testid="matrix-row-base_url"]')).toBeVisible();
  await expect(page.locator('[data-testid="matrix-row-token"]')).toBeVisible();
  const base = page.locator(`[data-testid="matrix-cell-base_url-${collection.id}"]`);
  await expect(base).toContainText('http://flow.test');
  await base.click();
  await expect(page.locator('[data-testid="matrix-cell-input"]')).toHaveCount(0);
  await expect(page.locator(`[data-testid="matrix-cell-token-${collection.id}"]`)).toHaveAttribute(
    'data-kind',
    'secret',
  );

  // The column menu's YAML opens the variables dialog on its YAML tab, with the same notice.
  await page.click(`[data-testid="matrix-col-menu-${collection.id}"]`);
  await page.click('[data-testid="menu-item-yaml"]');
  const dialog = page.locator('[data-testid="api-variables-dialog"]');
  await expect(dialog).toBeVisible();
  await expect(dialog.locator('[data-testid="api-tab-yaml"]')).toHaveAttribute('data-state', 'on');
  await expect(dialog.locator('[data-testid="config-file-problem"]')).toContainText(
    'Can’t be read',
  );
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
});
