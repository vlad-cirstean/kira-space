import { expect } from '@playwright/test';
import {
  calls,
  captureControl,
  changeRequestItem,
  emptyItem,
  issueItem,
  laterResult,
  noBranchResult,
  t,
  captureTest as test,
  textItem,
} from './support/capture';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// UI half of captureflow/capture_test.go and adeflow/capturereview_test.go: the Go flows write
// tests/contract/capture.json (one item per kind, the confirm results); this spec renders them in
// the popup and checks the keyboard model.

const CONFIRM_LATER = { channel: IPC.captureConfirm, response: laterResult };
const DISMISS = { channel: IPC.captureDismiss, response: null };

const today = () => {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
};

test('no radios or checkboxes; the hints show Later, Today and Close', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: captureControl(textItem) });
  await expect(page.locator(t('capture-title'))).toBeVisible();
  await expect(page.locator('[role="radio"]')).toHaveCount(0);
  await expect(page.locator('[role="checkbox"]')).toHaveCount(0);
  await expect(page.locator(t('capture-actions'))).toHaveCount(0);
  await expect(page.locator(t('capture-hint-later'))).toContainText('Later');
  await expect(page.locator(t('capture-hint-today'))).toContainText('⌘⏎');
  await expect(page.locator(t('capture-hint-today'))).toContainText('Today');
});

test('Enter adds to Later with the edited title', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: captureControl(textItem, [CONFIRM_LATER]),
  });
  await expect(page.locator(t('capture-label'))).toHaveCount(0);
  const title = page.locator(t('capture-title'));
  await expect(title).toHaveValue(textItem.title);
  await title.fill('Edited title');
  await page.keyboard.press('Enter');
  await expect.poll(() => calls(control, IPC.captureConfirm)).toHaveLength(1);
  expect(calls(control, IPC.captureConfirm)[0]?.args).toMatchObject({
    itemId: textItem.id,
    title: 'Edited title',
    day: null,
  });
});

test('CmdOrCtrl+Enter adds to Today', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: captureControl(textItem, [CONFIRM_LATER]),
  });
  await page.keyboard.press('Meta+Enter');
  await expect.poll(() => calls(control, IPC.captureConfirm)).toHaveLength(1);
  expect(calls(control, IPC.captureConfirm)[0]?.args).toMatchObject({ day: today() });
});

test('Shift+Enter and Alt+Enter add nothing', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: captureControl(textItem, [CONFIRM_LATER]),
  });
  await expect(page.locator(t('capture-title'))).toBeVisible();
  await page.keyboard.press('Shift+Enter');
  await page.keyboard.press('Alt+Enter');
  await expect(page.locator(t('capture-window'))).toBeVisible();
  expect(calls(control, IPC.captureConfirm)).toHaveLength(0);
});

test('a PR shows its label and the review note, and adds to Today', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: captureControl(changeRequestItem, [CONFIRM_LATER]),
  });
  await expect(page.locator(t('capture-label'))).toHaveText('PR #7 acme/api');
  await expect(page.locator(t('capture-text'))).toHaveText(changeRequestItem.text);
  await expect(page.locator(t('capture-review'))).toBeVisible();
  await page.keyboard.press('Meta+Enter');
  await expect.poll(() => calls(control, IPC.captureConfirm)).toHaveLength(1);
  expect(calls(control, IPC.captureConfirm)[0]?.args).toMatchObject({
    itemId: changeRequestItem.id,
    day: today(),
  });
});

test('an issue shows its label without the review note', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: captureControl(issueItem) });
  await expect(page.locator(t('capture-label'))).toHaveText(issueItem.label);
  await expect(page.locator(t('capture-review'))).toHaveCount(0);
});

test('a branch that could not be added shows the notice; Enter closes without a second add', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: captureControl(changeRequestItem, [
      { channel: IPC.captureConfirm, response: noBranchResult },
      DISMISS,
    ]),
  });
  await page.keyboard.press('Enter');
  await expect(page.locator(t('capture-notice'))).toContainText('without its branch');
  await expect(page.locator(t('capture-review'))).toHaveCount(0);
  await page.keyboard.press('Enter');
  await expect.poll(() => calls(control, IPC.captureDismiss)).toHaveLength(1);
  expect(calls(control, IPC.captureConfirm)).toHaveLength(1);
});

test('Escape dismisses', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: captureControl(textItem, [DISMISS]),
  });
  await page.keyboard.press('Escape');
  await expect.poll(() => calls(control, IPC.captureDismiss)).toHaveLength(1);
});

test('an empty clipboard shows the empty state and Enter dismisses', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: captureControl(emptyItem, [DISMISS]),
  });
  await expect(page.locator(t('capture-empty'))).toBeVisible();
  await expect(page.locator(t('capture-title'))).toHaveCount(0);
  await page.keyboard.press('Enter');
  await expect.poll(() => calls(control, IPC.captureDismiss)).toHaveLength(1);
  expect(calls(control, IPC.captureConfirm)).toHaveLength(0);
});

test('a pushed item replaces the content', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: captureControl(textItem) });
  await emitWailsEvent(page, IPC.captureItem, changeRequestItem);
  await expect(page.locator(t('capture-label'))).toHaveText('PR #7 acme/api');
});

test('a failed confirm keeps the popup open and shows the error', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: captureControl(textItem, [
      {
        channel: IPC.captureConfirm,
        error: {
          code: 'E_INTERNAL',
          message: 'The task could not be added.',
        },
      },
    ]),
  });
  await page.keyboard.press('Enter');
  await expect(page.locator(t('capture-error'))).toContainText('could not be added');
  await expect(page.locator(t('capture-window'))).toBeVisible();
});
