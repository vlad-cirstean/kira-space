// J-COLL-012. UI half of apiflow.TestTreeRowURLStaysATemplate (contract api-copy-url): Copy URL on a
// tree row writes the row's template text. A variable and a secret in the URL stay as placeholders
// on the clipboard, and nothing is fetched to resolve them.
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openHttpMode } from './support/apiMode';
import { collectionRow } from './support/apiRows';
import { installClipboardSpy, lastClipboardWrite } from './support/clipboard';

const tree = contract<{
  collections: { id: string }[];
  items: { id: string; name: string; url: string }[];
}>('api-copy-url', 'CollectionsService.List');
const COLLECTION_ID = 'col-copy';
const ITEM_ID = 'item-copy';
const TREE = {
  collections: tree.collections.map((c) => ({ ...c, id: COLLECTION_ID })),
  items: tree.items.map((i) => ({ ...i, id: ITEM_ID, collectionId: COLLECTION_ID })),
};

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.collectionsList, response: TREE },
  { channel: IPC.variablesListEnvironments, response: [] },
];

test('Copy URL on a tree row writes the template text and resolves no variable', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: CONTROL });
  await openHttpMode(page);
  await collectionRow(page, COLLECTION_ID).locator('[data-testid="tree-twisty"]').click();
  await installClipboardSpy(page);

  await collectionRow(page, ITEM_ID).click({ button: 'right' });
  await page.click('[data-testid="menu-item-copy-url"]');

  await expect.poll(() => lastClipboardWrite(page)).toBe(tree.items[0].url);
  expect(tree.items[0].url).toBe('{{base}}/x?key={{key}}');
  expect(control.log().filter((e) => e.channel === IPC.variablesReveal)).toHaveLength(0);
});
