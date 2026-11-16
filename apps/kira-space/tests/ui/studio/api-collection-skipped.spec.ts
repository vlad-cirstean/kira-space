import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openHttpMode } from './support/apiMode';
import { collectionRow } from './support/apiRows';

// Contract api-collection-skipped. Backend half: apiflow.TestHandEditedCollectionWithBadEntry
// writes the same fixture. A hand-edited collection with one bad item loads its valid items; the
// panel lists the skipped item with its line and reason, with no whole-file error.

interface ApiConfigView {
  files: {
    file: string;
    problem: unknown;
    lastGood: boolean;
    skipped: { line: number; index: number; id: string; name: string; message: string }[];
  }[];
}

const tree = contract<{
  collections: { id: string; file: string }[];
  items: { id: string; name: string }[];
}>('api-collection-skipped', 'CollectionsService.List');
const view = contract<ApiConfigView>('api-collection-skipped', 'ApiConfigService.View');

const collection = tree.collections[0];
const skipped = view.files[0].skipped[0];

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.collectionsList, response: tree },
  { channel: IPC.variablesListEnvironments, response: [] },
  { channel: IPC.apiConfigView, response: view },
];

test('api collection skipped — valid items load, the skipped item is listed with its line, Edit opens the variables dialog', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await openHttpMode(page);

  await expect(collectionRow(page, collection.id)).toBeVisible();
  await collectionRow(page, collection.id).locator('[data-testid="tree-twisty"]').click();
  await expect(collectionRow(page, tree.items[0].id)).toContainText('Good one');
  await expect(collectionRow(page, tree.items[1].id)).toContainText('Good two');
  await expect(page.getByText('No protocol', { exact: true })).toHaveCount(0);

  const alert = page.locator('[data-testid="api-config-skipped"]');
  await expect(alert).toBeVisible();
  await expect(alert).toContainText('Hand');
  const rows = alert.locator('[data-testid="api-config-skipped-item"]');
  await expect(rows).toHaveCount(1);
  await expect(rows).toContainText(`Line ${skipped.line}`);
  await expect(rows).toContainText(skipped.name);
  await expect(rows).toContainText(skipped.message);
  await expect(page.locator('[data-testid="api-config-last-good"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="config-file-problem"]')).toHaveCount(0);

  await alert.locator('[data-testid="api-config-skipped-edit"]').click();
  await expect(page.locator('[data-testid="api-variables-dialog"]')).toBeVisible();
});
