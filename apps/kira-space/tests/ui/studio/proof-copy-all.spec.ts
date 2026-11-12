import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openProofWindow, proofControl, shotFolder, threeSteps } from './support/proof';

// P621. The Go half (internal/flows/proofflow TestProofShotCopyAll) proves a batch of tab PNGs
// reaches the clipboard as separate items; this half proves the window sends one image per tab,
// in tab order, commits once and says so. Without the capability, save stays the primary action.

// A capture clones thousands of styled nodes; headless WebKit takes seconds per tab.
const CAPTURE_MS = 60_000;
const PNG_PREFIX = 'iVBORw0KGgo';

// biome-ignore lint/suspicious/noExplicitAny: logged bridge args
type Json = Record<string, any>;

const multiImage = (
  commit: Json = { response: contract('proof', 'ProofService.CommitShotClip') },
) => [
  ...threeSteps(),
  {
    channel: IPC.proofShotClipboard,
    response: contract('proof', 'ProofService.ShotClipboard'),
  },
  {
    channel: IPC.proofOpenShotClip,
    response: { ...(contract('proof', 'ProofService.OpenShotClip') as Json), id: 'clip_test' },
  },
  { channel: IPC.proofCommitShotClip, ...commit },
];

const stepsReady = (page: import('@playwright/test').Page) =>
  expect(page.locator('[data-testid="proof-step"][data-ready="true"]').first()).toBeVisible({
    timeout: 20_000,
  });

const calls = (log: { channel: string; args: unknown }[], channel: string) =>
  log.filter((c) => c.channel === channel).map((c) => c.args as Json);

test('copy screenshots puts every tab on the clipboard', async ({ relaunch, uiServer }) => {
  test.setTimeout(180_000);
  const { window: page, control } = await relaunch({ control: proofControl(multiImage()) });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  const primary = page.locator('[data-testid="proof-shot"]');
  await expect(primary).toHaveAttribute('data-mode', 'copyAll');
  await expect(primary).toContainText('Copy screenshots');
  await primary.click();
  await expect
    .poll(() => calls(control.log(), IPC.proofCommitShotClip).length, { timeout: CAPTURE_MS })
    .toBe(1);

  const adds = calls(control.log(), IPC.proofAddShotClip);
  expect(adds.map((a) => a.filename)).toEqual([
    '01-Orders-before.png',
    '02-Doc-hit.png',
    '03-Stats.png',
  ]);
  for (const a of adds) {
    expect(a.clipId).toBe('clip_test');
    expect(a.pngBase64.startsWith(PNG_PREFIX)).toBe(true);
  }
  expect(calls(control.log(), IPC.proofCommitShotClip)[0]).toMatchObject({ clipId: 'clip_test' });
  await expect.poll(() => calls(control.log(), IPC.proofCloseShotClip).length).toBe(1);
  expect(calls(control.log(), IPC.proofOpenShotClip)).toHaveLength(1);
  expect(calls(control.log(), IPC.proofOpenShotFolder)).toHaveLength(0);

  const status = page.locator('[data-testid="proof-shot-status"]');
  await expect(status).toHaveAttribute('data-tone', 'copied');
  await expect(status).toContainText('Copied 3 screenshots');
  await expect(
    page.locator('[data-testid="proof-tabs"] [data-testid="tab"][data-active="true"]'),
  ).toContainText('#1');
});

test('the menu saves when copy is the default', async ({ relaunch, uiServer }) => {
  test.setTimeout(180_000);
  const { window: page, control } = await relaunch({
    control: proofControl([
      ...multiImage(),
      { channel: IPC.proofOpenShotFolder, response: shotFolder() },
    ]),
  });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  await page.locator('[data-testid="proof-shot-more"]').click();
  await page.locator('[data-testid="proof-shot-save-all"]').click();
  await expect
    .poll(() => calls(control.log(), IPC.proofSaveJpegTo).length, { timeout: CAPTURE_MS })
    .toBe(3);
  expect(calls(control.log(), IPC.proofOpenShotClip)).toHaveLength(0);
});

test('a failed copy shows the error and leaves nothing open', async ({ relaunch, uiServer }) => {
  test.setTimeout(180_000);
  const message = 'This system can’t copy several images to the clipboard.';
  const { window: page, control } = await relaunch({
    control: proofControl([...multiImage({ error: { code: 'E_UNAVAILABLE', message } })]),
  });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  await page.locator('[data-testid="proof-shot"]').click();
  const status = page.locator('[data-testid="proof-shot-status"]');
  await expect(status).toContainText(message, { timeout: CAPTURE_MS });
  await expect(status).toHaveAttribute('data-tone', 'error');
  await expect.poll(() => calls(control.log(), IPC.proofCloseShotClip).length).toBe(1);
});

test('without a multi-image clipboard save stays the default', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({ control: proofControl(threeSteps()) });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  const primary = page.locator('[data-testid="proof-shot"]');
  await expect(primary).toHaveAttribute('data-mode', 'saveAll');
  await expect(primary).toContainText('Save screenshots');
  await page.locator('[data-testid="proof-shot-more"]').click();
  await expect(page.locator('[data-testid="proof-shot-copy"]')).toBeVisible();
  await expect(page.locator('[data-testid="proof-shot-copy-all"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="proof-shot-save-all"]')).toHaveCount(0);
});
