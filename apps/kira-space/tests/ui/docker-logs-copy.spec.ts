import { expect, studioTest as test } from './fixtures';
import { installClipboardSpy, lastClipboardWrite } from './studio/support/clipboard';
import { openContainer, openDocker, setupDocker } from './studio/support/dockerFixture';
import { emitWailsEvent } from './support/mockRuntime';

const TOTAL = 20_050;
const CAP = 20_000;
const BATCH = 5_000;

// J-DKR-015: the viewer keeps the newest 20,000 lines of a live follow, and a copy of a selection
// carries exactly the text of the selected rows, none of the dropped ones.
test('docker logs: a live follow keeps the newest 20000 lines and a selection copies exactly', async ({
  relaunch,
}) => {
  const { page, docker } = await setupDocker(relaunch);
  await openDocker(page);
  await openContainer(page, 'solo', 'logs');
  await expect.poll(() => docker.calls('LogsOpen').length).toBe(1);
  const { streamId } = docker.calls('LogsOpen')[0].args as { streamId: string };

  for (let from = 1; from <= TOTAL; from += BATCH) {
    const lines = [];
    for (let n = from; n < from + BATCH && n <= TOTAL; n++) {
      lines.push({ stream: 'stdout', text: `line ${n}` });
    }
    await emitWailsEvent(page, 'kira:docker:logs', { streamId, lines, ended: false });
  }

  const rows = page.locator('[data-testid="docker-log-line"]');
  const scroller = page.locator('[data-testid="docker-log"]');
  // Following: the newest line is at the bottom.
  await expect(rows.last()).toHaveText(`line ${TOTAL}`);

  // Scrolled to the top: the oldest kept line is 51, so the first 50 were dropped.
  await scroller.evaluate((el) => {
    el.scrollTop = 0;
  });
  await expect(rows.first()).toHaveAttribute('data-index', '0');
  await expect(rows.first()).toHaveText(`line ${TOTAL - CAP + 1}`);
  const total = await scroller
    .locator('div.relative')
    .first()
    .evaluate((el) => el.clientHeight);
  expect(total).toBe(CAP * 18);

  // Select the first five rows and copy them.
  const want = Array.from({ length: 5 }, (_, i) => `line ${TOTAL - CAP + 1 + i}`);
  await expect(rows.nth(4)).toHaveText(want[4] ?? '');
  await page.evaluate(() => {
    const els = document.querySelectorAll('[data-testid="docker-log-line"]');
    const range = document.createRange();
    range.setStartBefore(els[0] as Element);
    range.setEndAfter(els[4] as Element);
    const sel = window.getSelection();
    sel?.removeAllRanges();
    sel?.addRange(range);
  });
  // The native copy writes the selection; WebKit refuses an ungestured clipboard read, so record
  // the text the copy handler stores instead.
  await page.evaluate(() => {
    const w = window as unknown as { __copied: string[] };
    w.__copied = [];
    const orig = DataTransfer.prototype.setData;
    DataTransfer.prototype.setData = function (format: string, data: string) {
      if (format === 'text/plain') w.__copied.push(data);
      return orig.call(this, format, data);
    };
  });
  await page.keyboard.press('ControlOrMeta+C');
  await expect
    .poll(() => page.evaluate(() => (window as unknown as { __copied: string[] }).__copied.length))
    .toBe(1);
  const copied = await page.evaluate(
    () => (window as unknown as { __copied: string[] }).__copied[0] ?? '',
  );
  expect(copied.trim().split(/\r?\n/)).toEqual(want);
});

// Copy log with a filter set writes the filtered lines the pane shows, not the whole log.
test('docker logs: Copy log with a filter writes only the matching lines', async ({ relaunch }) => {
  const { page, docker } = await setupDocker(relaunch);
  await openDocker(page);
  await openContainer(page, 'solo', 'logs');
  await expect.poll(() => docker.calls('LogsOpen').length).toBe(1);
  const { streamId } = docker.calls('LogsOpen')[0].args as { streamId: string };

  const texts = ['boom one', 'quiet', 'boom two'];
  await emitWailsEvent(page, 'kira:docker:logs', {
    streamId,
    lines: texts.map((text) => ({ stream: 'stdout', text })),
    ended: false,
  });
  const rows = page.locator('[data-testid="docker-log-line"]');
  await expect(rows).toHaveCount(3);
  await page.locator('[data-testid="docker-logs-filter"]').fill('boom');
  await expect(rows).toHaveCount(2);
  await installClipboardSpy(page);
  await page.locator('[data-testid="docker-logs-copy"]').click();
  await expect.poll(() => lastClipboardWrite(page)).toBe('boom one\nboom two');
});
