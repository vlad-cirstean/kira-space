import { expect, test } from './fixtures';
import { editorState, gridCell } from './support/grid';

// Excerpt of tests/ui/mutations.spec.ts `editCell` and interaction.spec.ts's menu Edit/Escape
// step, plus the veto cases onBeforeEditCell enforces.
test('edit commits, Escape cancels, vetoed cells refuse', async ({ grid }) => {
  const page = await grid();

  const name = gridCell(page, 3, 'name');
  await name.fill('Renamed');
  expect(await name.text()).toBe('Renamed');
  expect((await name.state()).staged).toBe(true);

  await gridCell(page, 4, 'name').rightClick();
  await page.click('[data-testid="menu-item-edit"]');
  await expect.poll(async () => (await editorState(page)).open).toBe(true);
  await page.keyboard.press('Control+a');
  await page.keyboard.type('Dropped');
  await page.keyboard.press('Escape');
  await expect.poll(async () => (await editorState(page)).open).toBe(false);
  expect(await gridCell(page, 4, 'name').text()).toBe('Customer number 5 Ltd');
  expect((await gridCell(page, 4, 'name').state()).staged).toBe(false);

  await gridCell(page, 2, 'email').dblclick(); // masked
  expect((await editorState(page)).open).toBe(false);
  expect((await editorState(page)).vetoReason).toBe('masked');

  await gridCell(page, 2, 'updated_at').dblclick(); // generated
  expect((await editorState(page)).open).toBe(false);
  expect((await editorState(page)).vetoReason).toBe('generated');
});
